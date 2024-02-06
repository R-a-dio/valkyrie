package admin

import (
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"time"

	radio "github.com/R-a-dio/valkyrie"
	"github.com/R-a-dio/valkyrie/errors"
	"github.com/R-a-dio/valkyrie/website/middleware"
	"github.com/R-a-dio/valkyrie/website/public"
	"github.com/rs/zerolog/hlog"
)

type PendingInput struct {
	SharedInput
	Submissions []PendingForm
}

func NewPendingInput(r *http.Request) PendingInput {
	return PendingInput{
		SharedInput: NewSharedInput(r),
	}
}

func (PendingInput) TemplateBundle() string {
	return "admin-pending"
}

type PendingForm struct {
	radio.PendingSong

	Errors map[string]string
}

func (PendingForm) TemplateBundle() string {
	return "admin-pending"
}

func (PendingForm) TemplateName() string {
	return "form_admin_pending"
}

func (pi *PendingInput) Prepare(s radio.SubmissionStorage) error {
	const op errors.Op = "website/admin.pendingInput.Prepare"

	subms, err := s.All()
	if err != nil {
		return errors.E(op, err)
	}

	pi.Submissions = make([]PendingForm, len(subms))
	for i, v := range subms {
		pi.Submissions[i].PendingSong = v
	}
	return nil
}

func (s *State) GetPending(w http.ResponseWriter, r *http.Request) {
	var input = NewPendingInput(r)

	if err := input.Prepare(s.Storage.Submissions(r.Context())); err != nil {
		hlog.FromRequest(r).Error().Err(err).Msg("database failure")
		return
	}

	if err := s.TemplateExecutor.Execute(w, r, input); err != nil {
		hlog.FromRequest(r).Error().Err(err).Msg("template failure")
		return
	}
}

func (s *State) PostPending(w http.ResponseWriter, r *http.Request) {
	var input = NewPendingInput(r)

	if input.User == nil || !input.User.UserPermissions.Has(radio.PermPendingEdit) {
		s.GetPending(w, r)
		return
	}

	form, err := s.postPending(w, r)
	if err == nil {
		// success handle the response back to the client
		if public.IsHTMX(r) {
			// htmx, send an empty response so that the entry gets removed
			return
		}
		// no htmx, send a full page back
		s.GetPending(w, r)
		return
	}

	hlog.FromRequest(r).Error().Err(err).Msg("failed post pending")

	// failed, handle the input and see if we can get info back to the user
	if public.IsHTMX(r) {
		// htmx, send just the form back
		if err := s.TemplateExecutor.Execute(w, r, form); err != nil {
			hlog.FromRequest(r).Error().Err(err).Msg("template failure")
		}
		return
	}

	// no htmx, send a full page back, but we have to hydrate the full list and swap out
	// the element that was posted with the posted values
	if err := input.Prepare(s.Storage.Submissions(r.Context())); err != nil {
		hlog.FromRequest(r).Error().Err(err).Msg("database failure")
		return
	}

	i := slices.IndexFunc(input.Submissions, func(p PendingForm) bool {
		return p.ID == form.ID
	})

	if i != -1 { // if our ID doesn't exist some third-party might've removed it from pending
		input.Submissions[i] = form
	}

	if err := s.TemplateExecutor.Execute(w, r, input); err != nil {
		hlog.FromRequest(r).Error().Err(err).Msg("template failure")
		return
	}
}

func (s *State) postPending(w http.ResponseWriter, r *http.Request) (PendingForm, error) {
	const op errors.Op = "website/admin.postPending"

	// grab the pending id
	id, err := strconv.Atoi(r.PostFormValue("id"))
	if err != nil {
		return PendingForm{}, errors.E(op, err, errors.InvalidForm)
	}

	// grab the pending data from the database
	song, err := s.Storage.Submissions(r.Context()).GetSubmission(radio.SubmissionID(id))
	if err != nil {
		return PendingForm{}, errors.E(op, err, errors.InternalServer)
	}

	// then update it with the submitted form data
	form := NewPendingForm(*song, r.PostForm)
	if !form.Validate() {
		return form, errors.E(op, err, errors.InvalidForm)
	}

	// continue somewhere else depending on the status
	switch form.Status {
	case radio.SubmissionAccepted:
		return s.postPendingDoAccept(w, r, form)
	case radio.SubmissionDeclined:
		return s.postPendingDoDecline(w, r, form)
	case radio.SubmissionReplacement:
		return s.postPendingDoReplace(w, r, form)
	}

	return form, errors.E(op, errors.InvalidArgument)
}

func (s *State) postPendingDoReplace(w http.ResponseWriter, r *http.Request, form PendingForm) (PendingForm, error) {
	const op errors.Op = "website/admin.postPendingDoReplace"
	var ctx = r.Context()

	// transaction start
	ss, tx, err := s.Storage.SubmissionsTx(ctx, nil)
	if err != nil {
		return form, errors.E(op, err, errors.InternalServer)
	}
	defer tx.Rollback() // rollback if we fail anywhere

	ts, _, err := s.Storage.TrackTx(ctx, tx)
	if err != nil {
		return form, errors.E(op, err, errors.InternalServer)
	}

	// create a database song from the form
	track := form.ToSong(*middleware.UserFromContext(ctx))
	form.AcceptedSong = &track

	// update tracks data
	err = ts.UpdateMetadata(track)
	if err != nil {
		return form, errors.E(op, err, errors.InternalServer)
	}

	// insert into post-pending
	err = ss.InsertPostPending(form.PendingSong)
	if err != nil {
		return form, errors.E(op, err, errors.InternalServer)
	}

	// remove from submissions
	err = ss.RemoveSubmission(form.ID)
	if err != nil {
		return form, errors.E(op, err, errors.InternalServer)
	}

	// TODO: file move handling

	// commit
	if err = tx.Commit(); err != nil {
		return form, errors.E(op, err, errors.InternalServer)
	}

	return PendingForm{}, nil
}

func (s *State) postPendingDoDecline(w http.ResponseWriter, r *http.Request, form PendingForm) (PendingForm, error) {
	const op errors.Op = "website/admin.postPendingDoDecline"
	var ctx = r.Context()

	// transaction start
	ss, tx, err := s.Storage.SubmissionsTx(ctx, nil)
	if err != nil {
		return form, errors.E(op, err, errors.InternalServer)
	}
	defer tx.Rollback() // rollback if we fail anywhere

	// insert into post-pending
	err = ss.InsertPostPending(form.PendingSong)
	if err != nil {
		return form, errors.E(op, err, errors.InternalServer)
	}

	// remove from submissions
	err = ss.RemoveSubmission(form.ID)
	if err != nil {
		return form, errors.E(op, err, errors.InternalServer)
	}

	// TODO: file deletion handling

	// commit
	if err = tx.Commit(); err != nil {
		return form, errors.E(op, err, errors.InternalServer)
	}

	return form, nil
}

func (s *State) postPendingDoAccept(w http.ResponseWriter, r *http.Request, form PendingForm) (PendingForm, error) {
	const op errors.Op = "website/admin.postPendingDoAccept"
	var ctx = r.Context()

	// transaction start
	ss, tx, err := s.Storage.SubmissionsTx(ctx, nil)
	if err != nil {
		return form, errors.E(op, err, errors.InternalServer)
	}
	defer tx.Rollback() // rollback if we fail anywhere

	// create a database song from the form
	track := form.ToSong(*middleware.UserFromContext(ctx))

	ts, _, err := s.Storage.TrackTx(ctx, tx)
	if err != nil {
		return form, errors.E(op, err, errors.InternalServer)
	}

	// insert the song into the database
	track.TrackID, err = ts.Insert(track)
	if err != nil {
		return form, errors.E(op, err, errors.InternalServer)
	}
	form.AcceptedSong = &track

	// insert the song into the post-pending info
	err = ss.InsertPostPending(form.PendingSong)
	if err != nil {
		return form, errors.E(op, err, errors.InternalServer)
	}

	// remove the submission entry
	err = ss.RemoveSubmission(form.ID)
	if err != nil {
		return form, errors.E(op, err, errors.InternalServer)
	}

	// TODO: file moving/renaming handling

	// commit
	if err = tx.Commit(); err != nil {
		return form, errors.E(op, err, errors.InternalServer)
	}

	return form, nil
}

// NewPendingForm creates a PendingForm with song as a base and updating those
// values from the form values given.
func NewPendingForm(song radio.PendingSong, form url.Values) PendingForm {
	pf := PendingForm{PendingSong: song}
	pf.Update(form)
	return pf
}

func (pf *PendingForm) Update(form url.Values) {
	switch form.Get("action") {
	case "replace":
		pf.Status = radio.SubmissionReplacement
	case "decline":
		pf.Status = radio.SubmissionDeclined
	case "accept":
		pf.Status = radio.SubmissionAccepted
	default:
		pf.Status = radio.SubmissionInvalid
	}

	pf.Artist = form.Get("artist")
	pf.Title = form.Get("title")
	pf.Album = form.Get("album")
	pf.Tags = form.Get("tags")
	if id, err := strconv.Atoi(form.Get("replacement")); err == nil {
		pf.ReplacementID = radio.TrackID(id)
	}
	pf.Reason = form.Get("reason")
	pf.ReviewedAt = time.Now()
	pf.GoodUpload = form.Get("good") != ""
}

func (pf *PendingForm) Validate() bool {
	pf.Errors = make(map[string]string)
	if pf.Status == radio.SubmissionInvalid {
		pf.Errors["action"] = "invalid status"
	}
	if len(pf.Artist) > 500 {
		pf.Errors["artist"] = "artist name too long"
	}
	if len(pf.Title) > 200 {
		pf.Errors["title"] = "title name too long"
	}
	if len(pf.Album) > 200 {
		pf.Errors["album"] = "album name too long"
	}
	if len(pf.Reason) > 120 {
		pf.Errors["reason"] = "reason too long"
	}

	return len(pf.Errors) == 0
}

func (pf *PendingForm) ToSong(user radio.User) radio.Song {
	var song radio.Song

	song.DatabaseTrack = new(radio.DatabaseTrack)
	if pf.Status == radio.SubmissionAccepted {
		song.Artist = pf.Artist
		song.Title = pf.Title
		song.Album = pf.Album
		song.Hydrate()
		song.Tags = pf.Tags
		song.FilePath = pf.FilePath
		if pf.ReplacementID != 0 {
			song.TrackID = pf.ReplacementID
			song.NeedReplacement = false
		}
		song.Length = pf.Length
		song.Usable = true
		song.Acceptor = user.Username
		song.LastEditor = user.Username
	}

	return song
}
