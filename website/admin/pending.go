package admin

import (
	"log"
	"net/http"
	"net/url"
	"strconv"

	radio "github.com/R-a-dio/valkyrie"
	"github.com/R-a-dio/valkyrie/errors"
)

type pendingInput struct {
	shared
	Submissions []radio.PendingSong
}

func (i *pendingInput) Prepare(s radio.SubmissionStorage) error {
	const op errors.Op = "website/admin.pendingInput.Prepare"

	subms, err := s.All()
	if err != nil {
		return errors.E(op, err)
	}
	i.Submissions = subms
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
	err := s.postPending(w, r)
	if err != nil {
		log.Println(err)
		return
	}
}

func (s *State) postPending(w http.ResponseWriter, r *http.Request) error {
	const op errors.Op = "website/admin.postPending"

	var input = pendingInput{
		shared: s.shared(r),
	}

	switch r.PostFormValue("action") {
	case "replace":
		return s.postPendingDoReplace(w, r)
	case "decline":
		return s.postPendingDoDecline(w, r)
	case "accept":
		return s.postPendingDoAccept(w, r)
	default:
		log.Println("someone submit a form without action")
		return errors.E(op, errors.InternalServer)
	}

	return s.TemplateExecutor.ExecuteFull("default", "form_admin_pending", w, input)
}

func (s *State) postPendingDoReplace(w http.ResponseWriter, r *http.Request) error {
	const op errors.Op = "website/admin.postPendingDoReplace"

	return nil
}

func (s *State) postPendingDoDecline(w http.ResponseWriter, r *http.Request) error {
	const op errors.Op = "website/admin.postPendingDoDecline"

	id, err := strconv.Atoi(r.PostForm.Get("id"))
	if err != nil {
		return errors.E(op, err, errors.InvalidForm)
	}

	song, err := s.Storage.Submissions(r.Context()).GetSubmission(radio.SubmissionID(id))
	if err != nil {
		return errors.E(op, err, errors.InternalServer)
	}

	pendingSongFromForm(song, r.PostForm)
	song.Status = radio.SubmissionDeclined
	return nil
}

func (s *State) postPendingDoAccept(w http.ResponseWriter, r *http.Request) error {
	const op errors.Op = "website/admin.postPendingDoAccept"

	id, err := strconv.Atoi(r.PostForm.Get("id"))
	if err != nil {
		return errors.E(op, err, errors.InvalidForm)
	}

	song, err := s.Storage.Submissions(r.Context()).GetSubmission(radio.SubmissionID(id))
	if err != nil {
		return errors.E(op, err, errors.InternalServer)
	}

	pendingSongFromForm(song, r.PostForm)
	song.Status = radio.SubmissionAccepted

	log.Println(song)
	return nil
}

func pendingSongFromForm(song *radio.PendingSong, form url.Values) {
	song.Artist = form.Get("artist")
	song.Title = form.Get("title")
	song.Album = form.Get("album")
	song.Tags = form.Get("tags")
	if id, err := strconv.Atoi(form.Get("replacement")); err == nil {
		song.ReplacementID = radio.TrackID(id)
	}
	song.Reason = form.Get("reason")
}
