package public

import (
	"context"
	"encoding"
	"io"
	"log"
	"mime/multipart"
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
	err := s.getSubmit(w, r, nil)
	if err != nil {
		log.Println(err)
		return
	}
}

func (s State) getSubmit(w http.ResponseWriter, r *http.Request, form *SubmissionForm) error {
	const op errors.Op = "website.getSubmit"
	ctx := r.Context()

	submitInput := struct {
		shared
		Form  *SubmissionForm
		Stats radio.SubmissionStats
	}{
		shared: s.shared(r),
		Form:   form,
	}

	stats, err := s.Storage.Submissions(ctx).SubmissionStats(r.RemoteAddr)
	if err != nil {
		return errors.E(op, err)
	}
	stats.LastSubmissionTime = time.Now()
	submitInput.Stats = stats

	return s.TemplateExecutor.ExecuteFull(theme, "submit", w, submitInput)
}

func (s State) PostSubmit(w http.ResponseWriter, r *http.Request) {
	form, err := s.postSubmit(w, r)

	if IsHTMX(r) {
		if err == nil {
			form = nil
		}
		if err != nil {
			log.Println(err)
		}

		err = s.TemplateExecutor.ExecuteTemplate(theme, "submit", "form_submit", w, form)
		if err != nil {
			log.Println(err)
		}
		return
	}

	if err != nil {
		log.Println(err)
		err = s.getSubmit(w, r, form)
		if err != nil {
			log.Println(err)
		}
		return
	}

	err = s.getSubmit(w, r, nil)
	if err != nil {
		log.Println(err)
	}
	return
}

func (s State) postSubmit(w http.ResponseWriter, r *http.Request) (*SubmissionForm, error) {
	const op errors.Op = "website.PostSubmit"

	mr, err := r.MultipartReader()
	if err != nil {
		return nil, errors.E(op, err, errors.InternalServer)
	}

	var form SubmissionForm
	err = form.ParseForm(mr)
	if err != nil {
		return nil, errors.E(op, err, errors.InternalServer)
	}

	song, err := PendingFromProbe(form.File)
	if err != nil {
		return &form, errors.E(op, err, errors.InternalServer)
	}
	form.Song = song

	if form.Validate() {
		return &form, nil
	}
	return &form, errors.E(op, errors.InvalidForm)
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

	info, err := audio.ProbeText(context.Background(), filename)
	if err != nil {
		return nil, errors.E(op, err)
	}

	s := radio.PendingSong{
		Status:      radio.SubmissionAwaitingReview,
		FilePath:    filename,
		Artist:      info.Artist,
		Title:       info.Title,
		Album:       info.Album,
		SubmittedAt: time.Now(),
		Format:      info.FormatName,
		Bitrate:     info.Bitrate,
		Length:      info.Duration,
	}

	return &s, nil
}

type SubmissionForm struct {
	Token string // csrf token?

	File             string
	OriginalFilename string

	Comment string
	Daypass string

	Replacement *radio.TrackID
	IsDaypass   bool

	Song *radio.PendingSong

	Errors map[string]string
}

func (sf *SubmissionForm) ParseForm(mr *multipart.Reader) error {
	const op errors.Op = "SubmissionForm.ParseForm"

	for i := 0; i < formMaxMIMEParts; i++ {
		part, err := mr.NextPart()
		if errors.IsE(err, io.EOF) {
			// finished reading parts
			return nil
		}
		if err != nil {
			return errors.E(op, err, errors.InternalServer)
		}

		switch part.FormName() {
		case "track": // audio file that is being submitted
			err = func() error {
				f, err := os.CreateTemp("", "pending")
				if err != nil {
					return err
				}
				defer f.Close()

				n, err := io.CopyN(f, part, formMaxFileLength)
				if err != nil && !errors.IsE(err, io.EOF) {
					return err
				}
				if n >= formMaxFileLength {
					return err
				}
				sf.OriginalFilename = part.FileName()
				sf.File = f.Name()
				return nil
			}()
			if err != nil {
				return errors.E(op, err)
			}
		case "comment":
			s, err := readString(part, formMaxCommentLength)
			if err != nil {
				return errors.E(op, err)
			}
			sf.Comment = s
		case "daypass":
			s, err := readString(part, formMaxDaypassLength)
			if err != nil {
				return errors.E(op, err)
			}
			sf.Daypass = s
			// TODO: implement daypass
			sf.IsDaypass = s != s
		case "replacement":
			s, err := readString(part, formMaxReplacementLength)
			if err != nil {
				return errors.E(op, err)
			}
			id, err := strconv.Atoi(s)
			if err != nil {
				return errors.E(op, err)
			}
			tid := radio.TrackID(id)
			sf.Replacement = &tid
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
