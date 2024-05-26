package admin

import (
	"fmt"
	"html/template"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"time"

	radio "github.com/R-a-dio/valkyrie"
	"github.com/R-a-dio/valkyrie/config"
	"github.com/R-a-dio/valkyrie/errors"
	"github.com/R-a-dio/valkyrie/streamer/audio"
	"github.com/R-a-dio/valkyrie/util"
	"github.com/R-a-dio/valkyrie/website/middleware"
	"github.com/go-chi/chi/v5"
	"github.com/gorilla/csrf"
	"github.com/rs/xid"
	"github.com/rs/zerolog/hlog"
)

type PendingInput struct {
	middleware.Input
	Submissions []PendingForm
}

func NewPendingInput(r *http.Request) PendingInput {
	return PendingInput{
		Input: middleware.InputFromRequest(r),
	}
}

func (PendingInput) TemplateBundle() string {
	return "pending"
}

type PendingForm struct {
	radio.PendingSong

	CSRFTokenInput template.HTML
	Errors         map[string]string
}

func (PendingForm) TemplateBundle() string {
	return "pending"
}

func (PendingForm) TemplateName() string {
	return "form_admin_pending"
}

// Hydrate hydrates the PendingInput with information from the SubmissionStorage
func (pi *PendingInput) Hydrate(s radio.SubmissionStorage, r *http.Request) error {
	const op errors.Op = "website/admin.pendingInput.Hydrate"

	subms, err := s.All()
	if err != nil {
		return errors.E(op, err)
	}

	csrfInput := csrf.TemplateField(r)
	pi.Submissions = make([]PendingForm, len(subms))
	for i, v := range subms {
		pi.Submissions[i].PendingSong = v
		pi.Submissions[i].CSRFTokenInput = csrfInput
	}
	return nil
}

func (s *State) GetPendingSong(w http.ResponseWriter, r *http.Request) {
	textID := chi.URLParam(r, "SubmissionID")
	id, err := radio.ParseSubmissionID(textID)
	if err != nil {
		// panic because chi should be the one verifying we are getting a numeric parameter
		panic("non-number found: " + textID)
	}

	song, err := s.Storage.Submissions(r.Context()).GetSubmission(id)
	if err != nil {
		hlog.FromRequest(r).Error().Err(err).Msg("database failure")
		return
	}

	// grab the path of the song and make it absolute
	path := util.AbsolutePath(s.Conf().MusicPath, song.FilePath)

	// if we want the audio file, send that back
	if r.FormValue("spectrum") == "" {
		f, err := s.FS.Open(path)
		if err != nil {
			hlog.FromRequest(r).Error().Err(err).Msg("fs failure")
			return
		}
		defer f.Close()

		util.AddContentDispositionSong(w, song.Metadata(), song.FilePath)
		http.ServeContent(w, r, "", time.Now(), f)
		return
	}

	// otherwise prep the spectrum image
	specPath, err := audio.Spectrum(r.Context(), s.FS, path)
	if err != nil {
		hlog.FromRequest(r).Error().Err(err).Msg("ffmpeg failure")
		return
	}
	defer os.Remove(specPath)

	// TODO: use in-memory file
	http.ServeFile(w, r, specPath)
}

func (s *State) GetPending(w http.ResponseWriter, r *http.Request) {
	var input = NewPendingInput(r)

	if err := input.Hydrate(s.Storage.Submissions(r.Context()), r); err != nil {
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

	if !input.User.IsValid() || !input.User.UserPermissions.Has(radio.PermPendingEdit) {
		hlog.FromRequest(r).Warn().Any("user", input.User).Msg("failed permission check")
		s.GetPending(w, r)
		return
	}

	form, err := s.postPending(w, r)
	if err == nil {
		// success handle the response back to the client
		if util.IsHTMX(r) {
			// htmx, send an empty response so that the entry gets removed
			return
		}
		// no htmx, send a full page back
		s.GetPending(w, r)
		return
	}

	hlog.FromRequest(r).Error().Err(err).Msg("failed post pending")

	// failed, handle the input and see if we can get info back to the user
	if util.IsHTMX(r) {
		// htmx, send just the form back
		if err := s.TemplateExecutor.Execute(w, r, form); err != nil {
			hlog.FromRequest(r).Error().Err(err).Msg("template failure")
		}
		return
	}

	// no htmx, send a full page back, but we have to hydrate the full list and swap out
	// the element that was posted with the posted values
	if err := input.Hydrate(s.Storage.Submissions(r.Context()), r); err != nil {
		hlog.FromRequest(r).Error().Err(err).Msg("database failure")
		return
	}

	// find our original form in the new submission page
	i := slices.IndexFunc(input.Submissions, func(p PendingForm) bool {
		return p.ID == form.ID
	})

	if i != -1 { // if our ID doesn't exist some third-party might've removed it from pending
		// and swap it out with the submitted form
		input.Submissions[i] = form
	}

	if err := s.TemplateExecutor.Execute(w, r, input); err != nil {
		hlog.FromRequest(r).Error().Err(err).Msg("template failure")
		return
	}
}

func (s *State) postPending(w http.ResponseWriter, r *http.Request) (PendingForm, error) {
	const op errors.Op = "website/admin.postPending"

	if err := r.ParseForm(); err != nil {
		return newPendingForm(r), errors.E(op, err, errors.InvalidForm)
	}
	// grab the pending id
	id, err := radio.ParseSubmissionID(r.PostFormValue("id"))
	if err != nil {
		return newPendingForm(r), errors.E(op, err, errors.InvalidForm)
	}

	// grab the pending data from the database
	song, err := s.Storage.Submissions(r.Context()).GetSubmission(id)
	if err != nil {
		return newPendingForm(r), errors.E(op, err, errors.InternalServer)
	}

	// then update it with the submitted form data
	form, err := NewPendingForm(r, *song)
	if err != nil {
		return form, errors.E(op, errors.InvalidForm)
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

	// create a song from the form
	track := form.ToSong(*middleware.UserFromContext(ctx))

	// grab our existing song data
	existing, err := ts.Get(track.TrackID)
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

	// grab the path of the existing entry
	existingPath := util.AbsolutePath(s.Conf().MusicPath, existing.FilePath)

	// then the path of the new entry
	pendingPath := util.AbsolutePath(pendingPath(s.Config), form.FilePath)

	// and move the new to the old path
	err = s.FS.Rename(pendingPath, existingPath)
	if err != nil {
		return form, errors.E(op, err, errors.InternalServer)
	}
	track.FilePath = existing.FilePath

	// update tracks data
	err = ts.UpdateMetadata(track)
	if err != nil {
		return form, errors.E(op, err, errors.InternalServer)
	}

	// commit
	if err = tx.Commit(); err != nil {
		return form, errors.E(op, err, errors.InternalServer)
	}

	return newPendingForm(r), nil
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

	// make path absolute if it isn't
	filePath := util.AbsolutePath(pendingPath(s.Config), form.FilePath)

	// remove the file
	err = s.FS.Remove(filePath)
	if err != nil {
		return form, errors.E(op, err, errors.InternalServer)
	}

	// commit
	if err = tx.Commit(); err != nil {
		return form, errors.E(op, err, errors.InternalServer)
	}

	return form, nil
}

func pendingPath(cfg config.Config) string {
	return filepath.Join(cfg.Conf().MusicPath, "pending")
}

func (s *State) postPendingDoAccept(w http.ResponseWriter, r *http.Request, form PendingForm) (PendingForm, error) {
	const op errors.Op = "website/admin.postPendingDoAccept"
	var ctx = r.Context()
	var new = form // make a copy so we can return the retrieved form when something goes wrong

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
	new.AcceptedSong = &track

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

	// generate a new filename for this song
	newFilename, err := GenerateMusicFilename(track)
	if err != nil {
		return form, errors.E(op, err, errors.InternalServer)
	}
	newFilePath := filepath.Join(s.Conf().MusicPath, newFilename)

	// make path absolute if it isn't
	track.FilePath = util.AbsolutePath(pendingPath(s.Config), track.FilePath)

	// rename the file to the actual music directory
	err = s.FS.Rename(track.FilePath, newFilePath)
	if err != nil {
		return form, errors.E(op, err, errors.InternalServer)
	}

	// now we need to change the filename in the database
	track.FilePath = newFilename
	err = ts.UpdateMetadata(track)
	if err != nil {
		return form, errors.E(op, err, errors.InternalServer)
	}

	// commit
	if err = tx.Commit(); err != nil {
		return form, errors.E(op, err, errors.InternalServer)
	}

	return new, nil
}

// NewPendingForm creates a PendingForm with song as a base and updating those
// values from the form values given.
func NewPendingForm(r *http.Request, song radio.PendingSong) (PendingForm, error) {
	const op errors.Op = "website/admin.NewPendingForm"

	pf := newPendingForm(r)
	pf.PendingSong = song
	pf.Update(r.PostForm)
	if !pf.Validate() {
		return pf, errors.E(op, errors.InvalidForm)
	}
	return pf, nil
}

func newPendingForm(r *http.Request) PendingForm {
	return PendingForm{
		CSRFTokenInput: csrf.TemplateField(r),
	}
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

	if id, err := radio.ParseSubmissionID(form.Get("id")); err == nil {
		pf.ID = id
	}
	pf.Artist = form.Get("artist")
	pf.Title = form.Get("title")
	pf.Album = form.Get("album")
	pf.Tags = form.Get("tags")
	if id, err := radio.ParseTrackID(form.Get("replacement")); err == nil {
		pf.ReplacementID = id
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
	if len(pf.Artist) > radio.LimitArtistLength {
		pf.Errors["artist"] = "artist name too long"
	}
	if len(pf.Title) > radio.LimitTitleLength {
		pf.Errors["title"] = "title too long"
	}
	if len(pf.Album) > radio.LimitAlbumLength {
		pf.Errors["album"] = "album name too long"
	}
	if len(pf.Reason) > radio.LimitReasonLength {
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

func (pf *PendingForm) ToValues() url.Values {
	var v = make(url.Values)

	v.Add("id", pf.ID.String())
	switch pf.Status {
	case radio.SubmissionAccepted:
		v.Add("action", "accept")
	case radio.SubmissionDeclined:
		v.Add("action", "decline")
	case radio.SubmissionReplacement:
		v.Add("action", "replace")
	}

	v.Add("artist", pf.Artist)
	v.Add("title", pf.Title)
	v.Add("album", pf.Album)
	v.Add("tags", pf.Tags)
	if pf.ReplacementID > 0 {
		v.Add("replacement", pf.ReplacementID.String())
	}
	v.Add("reason", pf.Reason)
	if pf.GoodUpload {
		v.Add("good", "checked")
	}
	return v
}

// GenerateMusicFilename generates a filename that can be used to store
// the song.
func GenerateMusicFilename(song radio.Song) (string, error) {
	const op errors.Op = "website/admin.GenerateMusicFilename"

	uid := xid.New()
	ext := filepath.Ext(song.FilePath)
	if ext == "" {
		return "", errors.E(op, "empty extension on FilePath", song)
	}
	if song.TrackID == 0 {
		return "", errors.E(op, "zero TrackID", song)
	}
	filename := fmt.Sprintf("%d_%s%s", song.TrackID, uid.String(), ext)
	return filename, nil
}
