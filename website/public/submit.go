package public

import (
	"context"
	"io"
	"log"
	"net/http"
	"os"
	"strconv"
	"time"

	radio "github.com/R-a-dio/valkyrie"
	"github.com/R-a-dio/valkyrie/errors"
	"github.com/R-a-dio/valkyrie/streamer/audio"
)

const (
	formMaxCommentLength     = 512
	formMaxDaypassLength     = 64
	formMaxReplacementLength = 16
	formMaxFileLength        = (1 << 20) * 100 // 100MiB
	formMaxMIMEParts         = 6
)

func (s State) GetSubmit(w http.ResponseWriter, r *http.Request) {
	submitInput := struct {
		shared
	}{
		shared: s.shared(r),
	}

	err := s.TemplateExecutor.ExecuteFull(theme, "submit", w, submitInput)
	if err != nil {
		log.Println(err)
		return
	}
}

func (s State) PostSubmit(w http.ResponseWriter, r *http.Request) {
	err := s.postSubmit(w, r)
	if err != nil {
		log.Println(err)
		return
	}
	return
}

func (s State) postSubmit(w http.ResponseWriter, r *http.Request) error {
	const op errors.Op = "website.PostSubmit"

	mr, err := r.MultipartReader()
	if err != nil {
		return errors.E(op, err, errors.InternalServer)
	}

	var form SubmissionForm

	for i := 0; i < formMaxMIMEParts; i++ {
		part, err := mr.NextPart()
		if err != nil {
			if errors.IsE(err, io.EOF) { // done parsing our form
				break
			}
			return errors.E(op, err, errors.InternalServer)
		}

		switch part.FormName() {
		case "track":
			err = func() error {
				f, err := os.CreateTemp("", "pending")
				defer f.Close()
				if err != nil {
					return errors.E(op, err, errors.InternalServer)
				}

				n, err := io.CopyN(f, part, formMaxFileLength)
				if err != nil && !errors.IsE(err, io.EOF) {
					return errors.E(op, err, errors.InternalServer)
				}
				if n >= formMaxCommentLength {
					// file too large
					return errors.E(op, err, errors.InternalServer)
				}
				form.File = f.Name()
				return nil
			}()
			if err != nil {
				return err
			}
		case "comment":
			s, err := readString(part, formMaxCommentLength)
			if err != nil {
				return errors.E(op, err, errors.InternalServer)
			}
			form.Comment = s
		case "daypass":
			s, err := readString(part, formMaxDaypassLength)
			if err != nil {
				return errors.E(op, err, errors.InternalServer)
			}
			// TODO: implement daypass
			form.IsDaypass = s != s
		case "replacement":
			s, err := readString(part, formMaxReplacementLength)
			if err != nil {
				return errors.E(op, err, errors.InternalServer)
			}
			id, err := strconv.Atoi(s)
			if err != nil {
				return errors.E(op, err, errors.InternalServer)
			}
			tid := radio.TrackID(id)
			form.Replacement = &tid
		default:
			// unknown form field, bail early and tell the client it's bad
			return errors.E(op, errors.InvalidForm)
		}
	}

	return nil
}

func readString(r io.Reader, maxSize int64) (string, error) {
	r = io.LimitReader(r, maxSize)
	if b, err := io.ReadAll(r); err != nil {
		return "", err
	} else {
		return string(b), nil
	}
}

func PendingFromProbe(filename string) (*radio.PendingSong, error) {
	const op errors.Op = "website/api.PendingFromProbe"

	info, err := audio.Probe(context.Background(), filename)
	if err != nil {
		return nil, errors.E(op, err)
	}

	s := radio.PendingSong{
		Status:      radio.SubmissionAwaitingReview,
		FilePath:    filename,
		SubmittedAt: time.Now(),
		Format:      info.Format.FormatName,
	}

	if info.Format != nil {
		s.Artist = info.Format.Tags.Artist
		s.Title = info.Format.Tags.Track
		s.Album = info.Format.Tags.Album
	}

	return &s, nil
}

type SubmissionForm struct {
	Token string // csrf token?

	File    string
	Comment string

	Replacement *radio.TrackID
	IsDaypass   bool

	Track *radio.PendingSong
}
