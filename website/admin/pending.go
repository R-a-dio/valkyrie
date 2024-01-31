package admin

import (
	"log"
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"time"

	radio "github.com/R-a-dio/valkyrie"
	"github.com/R-a-dio/valkyrie/errors"
	"github.com/R-a-dio/valkyrie/website/public"
)

type pendingInput struct {
	shared
	Submissions []PendingForm
}

type PendingForm struct {
	radio.PendingSong

	Errors map[string]string
}

func (pi *pendingInput) Prepare(s radio.SubmissionStorage) error {
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
	var input = pendingInput{
		shared: s.shared(r),
	}
	if err := input.Prepare(s.Storage.Submissions(r.Context())); err != nil {
		log.Println(err)
		return
	}

	if err := s.TemplateExecutor.ExecuteFull("default", "admin-pending", w, input); err != nil {
		log.Println(err)
		return
	}
}

func (s *State) PostPending(w http.ResponseWriter, r *http.Request) {
	var input = pendingInput{
		shared: s.shared(r),
	}
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

	// failed, handle the input and see if we can get info back to the user
	if public.IsHTMX(r) {
		// htmx, send just the form back
		if err := s.TemplateExecutor.ExecuteTemplate("default", "admin-pending", "form_admin_pending", w, form); err != nil {
			log.Println(err)
		}
		return
	}

	// no htmx, send a full page back, but we have to hydrate the full list and swap out
	// the element that was posted with the posted values
	if err := input.Prepare(s.Storage.Submissions(r.Context())); err != nil {
		log.Println(err)
		return
	}

	i := slices.IndexFunc(input.Submissions, func(p PendingForm) bool {
		return p.ID == form.ID
	})

	if i != -1 { // if our ID doesn't exist some third-party might've removed it from pending
		input.Submissions[i] = form
	}

	if err := s.TemplateExecutor.ExecuteFull("default", "admin-pending", w, input); err != nil {
		log.Println(err)
		return
	}
}

func (s *State) postPending(w http.ResponseWriter, r *http.Request) (PendingForm, error) {
	const op errors.Op = "website/admin.postPending"

	switch r.PostFormValue("action") {
	case "replace":
		return s.postPendingDoReplace(w, r)
	case "decline":
		return s.postPendingDoDecline(w, r)
	case "accept":
		return s.postPendingDoAccept(w, r)
	default:
		log.Println("someone submit a form without action")
		return PendingForm{}, errors.E(op, errors.InternalServer)
	}
}

func (s *State) postPendingDoReplace(w http.ResponseWriter, r *http.Request) (PendingForm, error) {
	const op errors.Op = "website/admin.postPendingDoReplace"

	return PendingForm{}, nil
}

func (s *State) postPendingDoDecline(w http.ResponseWriter, r *http.Request) (PendingForm, error) {
	const op errors.Op = "website/admin.postPendingDoDecline"

	id, err := strconv.Atoi(r.PostForm.Get("id"))
	if err != nil {
		return PendingForm{}, errors.E(op, err, errors.InvalidForm)
	}

	song, err := s.Storage.Submissions(r.Context()).GetSubmission(radio.SubmissionID(id))
	if err != nil {
		return PendingForm{}, errors.E(op, err, errors.InternalServer)
	}

	form := NewPendingForm(*song, r.PostForm)
	if !form.Validate() {
		return form, errors.E(op, err, errors.InvalidForm)
	}
	form.Status = radio.SubmissionDeclined

	log.Println(form)
	return form, nil
}

func (s *State) postPendingDoAccept(w http.ResponseWriter, r *http.Request) (PendingForm, error) {
	const op errors.Op = "website/admin.postPendingDoAccept"

	id, err := strconv.Atoi(r.PostForm.Get("id"))
	if err != nil {
		return PendingForm{}, errors.E(op, err, errors.InvalidForm)
	}

	song, err := s.Storage.Submissions(r.Context()).GetSubmission(radio.SubmissionID(id))
	if err != nil {
		return PendingForm{}, errors.E(op, err, errors.InternalServer)
	}

	form := NewPendingForm(*song, r.PostForm)
	if !form.Validate() {
		return form, errors.E(op, err, errors.InvalidForm)
	}
	form.Status = radio.SubmissionAccepted

	log.Println(form)
	return form, nil
}

func NewPendingForm(song radio.PendingSong, form url.Values) PendingForm {
	pf := PendingForm{PendingSong: song}
	pf.Update(form)
	return pf
}

func (pf *PendingForm) Update(form url.Values) {
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
