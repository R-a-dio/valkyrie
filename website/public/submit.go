package public

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"io"
	"log"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	radio "github.com/R-a-dio/valkyrie"
	"github.com/R-a-dio/valkyrie/errors"
	"github.com/R-a-dio/valkyrie/streamer/audio"
	"github.com/R-a-dio/valkyrie/website/middleware"
)

const (
	formMaxCommentLength     = 512
	formMaxDaypassLength     = 64
	formMaxReplacementLength = 16
	formMaxFileLength        = (1 << 20) * 100               // 100MiB
	formMaxSize              = formMaxFileLength + (1<<20)*3 // 3MiB on-top of file limit
	formMaxMIMEParts         = 6

	daypassHeader = "X-Daypass"
)

var Daypass = DaypassImpl{}

type DaypassImpl struct {
	mu      sync.Mutex
	update  time.Time
	daypass string
}

type DaypassInfo struct {
	// ValidUntil is the time this daypass will expire
	ValidUntil time.Time
	// Value is the current daypass
	Value string
}

// Info returns info about the daypass
func (di *DaypassImpl) Info() DaypassInfo {
	var info DaypassInfo
	info.Value = di.get()
	di.mu.Lock()
	info.ValidUntil = di.update.Add(time.Hour * 24)
	di.mu.Unlock()
	return info
}

// Is checks if the daypass given is equal to the current daypass
func (di *DaypassImpl) Is(daypass string) bool {
	return di.get() == daypass
}

// get returns the current daypass and optionally generates a new one
// if it has expired
func (di *DaypassImpl) get() string {
	di.mu.Lock()
	defer di.mu.Unlock()

	if time.Since(di.update) >= time.Hour*24 {
		di.update = time.Now()
		di.daypass = di.generate()
	}

	return di.daypass
}

// generate a new random daypass, this is a random sequence of
// bytes, sha256'd and base64 encoded before trimming it down to 16 characters
func (di *DaypassImpl) generate() string {
	var b [32]byte
	if _, err := rand.Read(b[:]); err != nil {
		log.Println("failed to update daypass:", err)
		// keep using the old daypass if we error
		return di.daypass
	}

	b = sha256.Sum256(b[:])
	return base64.RawURLEncoding.EncodeToString(b[:])[:16]
}

// getIdentifier either returns the username of a logged in user, or the RemoteAddr of
// the request
func (s State) getIdentifier(r *http.Request) (string, bool) {
	if user := middleware.UserFromContext(r.Context()); user != nil {
		return user.Username, true
	}
	return r.RemoteAddr, false
}

func (s State) canSubmitSong(r *http.Request) (time.Duration, error) {
	const op errors.Op = "website.canSubmitSong"

	identifier, isUser := s.getIdentifier(r)
	if isUser { // logged in users can always submit songs
		return 0, nil
	}

	last, err := s.Storage.Submissions(r.Context()).LastSubmissionTime(identifier)
	if err != nil {
		return 0, errors.E(op, err, errors.InternalServer)
	}

	since := time.Since(last)
	if since > time.Hour { // cooldown has passed so can submit song
		return 0, nil
	}

	daypass := r.Header.Get(daypassHeader)
	if Daypass.Is(daypass) { // daypass was used so can submit song
		return 0, nil
	}

	return since, errors.E(op, errors.UserCooldown)
}

func (s State) GetSubmit(w http.ResponseWriter, r *http.Request) {
	err := s.getSubmit(w, r, SubmissionForm{})
	if err != nil {
		log.Println(err)
		return
	}
}

func (s State) getSubmit(w http.ResponseWriter, r *http.Request, form SubmissionForm) error {
	const op errors.Op = "website.getSubmit"
	ctx := r.Context()

	submitInput := struct {
		shared
		Form  SubmissionForm
		Stats radio.SubmissionStats
	}{
		shared: s.shared(r),
		Form:   form,
	}

	identifier, _ := s.getIdentifier(r)
	stats, err := s.Storage.Submissions(ctx).SubmissionStats(identifier)
	if err != nil {
		return errors.E(op, err)
	}
	submitInput.Stats = stats

	return s.TemplateExecutor.ExecuteFull(theme, "submit", w, submitInput)
}

func (s State) PostSubmit(w http.ResponseWriter, r *http.Request) {
	// setup response function that differs between htmx/non-htmx request
	responseFn := func(form SubmissionForm) error {
		return s.getSubmit(w, r, form)
	}
	if IsHTMX(r) {
		responseFn = func(form SubmissionForm) error {
			return s.TemplateExecutor.ExecuteTemplate(theme, "submit", "form_submit", w, form)
		}
	}
	defer http.NewResponseController(w).Flush()

	// parse and validate the form
	form, err := s.postSubmit(w, r)
	if err != nil {
		// TODO: debug this
		// for unknown reason if we send a response without reading the body the connection is
		// hard-reset instead and our response goes missing, so discard the body up to our
		// allowed max size and then cut off if required
		io.CopyN(io.Discard, r.Body, formMaxSize)

		log.Println(err)
		if err := responseFn(form); err != nil {
			log.Println(err)
		}
		return
	}

	// success, update the submission time for the identifier
	identifier, _ := s.getIdentifier(r)
	if err = s.Storage.Submissions(r.Context()).UpdateSubmissionTime(identifier); err != nil {
		log.Println(err)
		if err = responseFn(form); err != nil {
			log.Println(err)
		}
		return
	}

	// send a new empty form back to the user
	back := SubmissionForm{Success: true}
	if form.IsDaypass {
		// if the submission was with a daypass, prefill the daypass for them again
		back.Daypass = form.Daypass
		back.IsDaypass = true
	}
	if err := responseFn(back); err != nil {
		log.Println(err)
	}
}

func (s State) postSubmit(w http.ResponseWriter, r *http.Request) (SubmissionForm, error) {
	const op errors.Op = "website.PostSubmit"

	// find out if the client is allowed to upload
	cooldown, err := s.canSubmitSong(r)
	if err != nil {
		return SubmissionForm{
			Errors: map[string]string{"cooldown": strconv.FormatInt(int64(cooldown/time.Second), 10)},
		}, errors.E(op, err)
	}

	mr, err := r.MultipartReader()
	if err != nil {
		return SubmissionForm{}, errors.E(op, err, errors.InternalServer)
	}

	var form SubmissionForm
	err = form.ParseForm(filepath.Join(s.Conf().MusicPath, "pending"), mr)
	if err != nil {
		return SubmissionForm{}, errors.E(op, err, errors.InternalServer)
	}

	// ParseForm just saved a temporary file that we want to delete if any other error
	// occurs after this, so prep a defer for it that we can no-op later
	var tmpFilename = form.File
	defer func() {
		if tmpFilename != "" {
			os.Remove(tmpFilename)
		}
	}()

	if !form.Validate() {
		return form, errors.E(op, errors.InvalidForm)
	}

	song, err := PendingFromProbe(form.File)
	if err != nil {
		form.Errors["track"] = "File invalid; probably not an audio file."
		return form, errors.E(op, err, errors.InternalServer)
	}
	// fill in extra info we don't get from the probe
	song.Comment = form.Comment
	song.Filename = form.OriginalFilename
	song.UserIdentifier = r.RemoteAddr
	if form.Replacement != nil {
		song.ReplacementID = *form.Replacement
	}
	song.SubmittedAt = time.Now()
	form.Song = song

	err = s.Storage.Submissions(r.Context()).InsertSubmission(*song)
	if err != nil {
		form.Errors["postprocessing"] = "Internal error, yell at someone in IRC"
		return form, errors.E(op, err, errors.InternalServer)
	}

	// clear the tmpFilename so that it doesn't get deleted after we return
	tmpFilename = ""
	return form, nil
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
	Success bool
	Token   string // csrf token?

	File             string
	OriginalFilename string

	Comment string
	Daypass string

	Replacement *radio.TrackID
	IsDaypass   bool

	Song *radio.PendingSong

	Errors map[string]string
}

func (sf *SubmissionForm) ParseForm(tempdir string, mr *multipart.Reader) error {
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
				// clean the extension from the user
				ext := filepath.Ext(part.FileName())
				ext = strings.ReplaceAll(ext, "*", "")

				f, err := os.CreateTemp(tempdir, "pending-*"+ext)
				if err != nil {
					return err
				}
				defer f.Close()

				n, err := io.CopyN(f, part, formMaxFileLength)
				if err != nil && !errors.IsE(err, io.EOF) {
					os.Remove(f.Name())
					return err
				}
				if n >= formMaxFileLength {
					os.Remove(f.Name())
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
			sf.IsDaypass = Daypass.Is(s)
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

func (sf *SubmissionForm) Validate() bool {
	sf.Errors = make(map[string]string)
	if sf.File == "" {
		sf.Errors["track"] = "no temporary file"
	}
	if sf.OriginalFilename == "" {
		sf.Errors["track"] = "no file selected"
	}
	if sf.Comment == "" {
		sf.Errors["comment"] = "no comment supplied"
	}
	if sf.Daypass != "" && !Daypass.Is(sf.Daypass) {
		sf.Errors["daypass"] = "daypass invalid"
	}

	return len(sf.Errors) == 0
}
