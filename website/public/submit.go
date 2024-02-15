package public

import (
	"context"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	radio "github.com/R-a-dio/valkyrie"
	"github.com/R-a-dio/valkyrie/errors"
	"github.com/R-a-dio/valkyrie/streamer/audio"
	"github.com/R-a-dio/valkyrie/util"
	"github.com/R-a-dio/valkyrie/util/daypass"
	"github.com/R-a-dio/valkyrie/website/middleware"
	"github.com/rs/zerolog/hlog"
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

type SubmitInput struct {
	middleware.Input
	Form  SubmissionForm
	Stats radio.SubmissionStats
}

func NewSubmitInput(storage radio.SubmissionStorageService, r *http.Request, form *SubmissionForm) (*SubmitInput, error) {
	const op errors.Op = "website.NewSubmitInput"

	if form == nil {
		form = new(SubmissionForm)
	}

	identifier, _ := getIdentifier(r)
	stats, err := storage.Submissions(r.Context()).SubmissionStats(identifier)
	if err != nil {
		return nil, errors.E(op, err)
	}

	return &SubmitInput{
		Input: middleware.InputFromRequest(r),
		Form:  *form,
		Stats: stats,
	}, nil
}

func (SubmitInput) TemplateBundle() string {
	return "submit"
}

// getIdentifier either returns the username of a logged in user, or the RemoteAddr of
// the request
func getIdentifier(r *http.Request) (string, bool) {
	if user := middleware.UserFromContext(r.Context()); user != nil {
		return user.Username, true
	}
	return r.RemoteAddr, false
}

func (s State) canSubmitSong(r *http.Request) (time.Duration, error) {
	const op errors.Op = "website.canSubmitSong"

	identifier, isUser := getIdentifier(r)
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
	if s.Daypass.Is(daypass) { // daypass was used so can submit song
		return 0, nil
	}

	return since, errors.E(op, errors.UserCooldown)
}

func (s State) GetSubmit(w http.ResponseWriter, r *http.Request) {
	err := s.getSubmit(w, r, nil)
	if err != nil {
		hlog.FromRequest(r).Error().Err(err).Msg("")
		return
	}
}

func (s State) getSubmit(w http.ResponseWriter, r *http.Request, form *SubmissionForm) error {
	const op errors.Op = "website.getSubmit"

	input, err := NewSubmitInput(s.Storage, r, form)
	if err != nil {
		return errors.E(op, err)
	}

	return s.Templates.Execute(w, r, input)
}

func (s State) PostSubmit(w http.ResponseWriter, r *http.Request) {
	// setup response function that differs between htmx/non-htmx request
	responseFn := func(form SubmissionForm) {
		var err error
		if util.IsHTMX(r) {
			err = s.Templates.Execute(w, r, form)
		} else {
			err = s.getSubmit(w, r, &form)
		}

		if err != nil {
			s.errorHandler(w, r, err)
		}
	}

	// parse and validate the form
	form, err := s.postSubmit(w, r)
	if err != nil {
		// for unknown reason if we send a response without reading the body the connection is
		// hard-reset instead and our response goes missing, so discard the body up to our
		// allowed max size and then cut off if required
		io.CopyN(io.Discard, r.Body, formMaxSize) // TODO: add a possible timeout

		hlog.FromRequest(r).Error().Err(err).Msg("")
		responseFn(form)
		return
	}

	// success, update the submission time for the identifier
	identifier, _ := getIdentifier(r)
	if err = s.Storage.Submissions(r.Context()).UpdateSubmissionTime(identifier); err != nil {
		hlog.FromRequest(r).Error().Err(err).Msg("")
		responseFn(form)
		return
	}

	// send a new empty form back to the user
	back := SubmissionForm{Success: true}
	if form.IsDaypass {
		// if the submission was with a daypass, prefill the daypass for them again
		back.Daypass = form.Daypass
		back.IsDaypass = true
	}

	responseFn(back)
	return
}

func (s State) postSubmit(w http.ResponseWriter, r *http.Request) (SubmissionForm, error) {
	const op errors.Op = "website.PostSubmit"

	// find out if the client is allowed to upload
	cooldown, err := s.canSubmitSong(r)
	if err != nil {
		return SubmissionForm{
			Errors: map[string]string{
				"cooldown": strconv.FormatInt(int64(cooldown/time.Second), 10),
			},
		}, errors.E(op, err)
	}

	// start parsing the form, it's multipart encoded due to file upload and we manually
	// handle some details due to reasons described in ParseForm
	mr, err := r.MultipartReader()
	if err != nil {
		return SubmissionForm{}, errors.E(op, err, errors.InternalServer)
	}

	form, err := NewSubmissionForm(filepath.Join(s.Conf().MusicPath, "pending"), mr)
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

	// Run a sanity check on the form input
	if !form.Validate(s.Storage.Track(r.Context()), s.Daypass) {
		return *form, errors.E(op, errors.InvalidForm)
	}

	// Probe the uploaded file for more information
	song, err := PendingFromProbe(form.File)
	if err != nil {
		form.Errors["track"] = "File invalid; probably not an audio file."
		return *form, errors.E(op, err, errors.InternalServer)
	}

	// Copy information over from the form and request to the PendingSong
	song.Comment = form.Comment
	song.Filename = form.OriginalFilename
	song.UserIdentifier, _ = getIdentifier(r)
	if form.Replacement != nil {
		song.ReplacementID = *form.Replacement
	}
	song.SubmittedAt = time.Now()
	form.Song = song

	// Add the pending entry to the database
	err = s.Storage.Submissions(r.Context()).InsertSubmission(*song)
	if err != nil {
		form.Errors["postprocessing"] = "Internal error, yell at someone in IRC"
		return *form, errors.E(op, err, errors.InternalServer)
	}

	// clear the tmpFilename so that it doesn't get deleted after we return
	tmpFilename = ""
	return *form, nil
}

// readString reads a string from r no longer than maxSize
func readString(r io.Reader, maxSize int64) (string, error) {
	r = io.LimitReader(r, maxSize)
	if b, err := io.ReadAll(r); err != nil {
		return "", err
	} else {
		return string(b), nil
	}
}

// PendingFromProbe runs ffprobe on the given filename and constructs
// a PendingSong with the information found
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

// SubmissionForm is the form struct passed to the submit page templates as .Form
type SubmissionForm struct {
	Token string // csrf token?
	// Success indicates if the upload was a success
	Success bool
	// IsDaypass is true if Daypass was valid
	IsDaypass bool
	// Errors is populated when any errors were found with the uploaded form
	// this is populated with their form field names as indicated below in addition to
	// the following possible keys:
	//		"postprocessing": something failed after successful form parsing but before we saved all the data
	//		"cooldown": indicates the user was not permitted to upload yet, they need to wait longer
	Errors map[string]string
	// form fields
	OriginalFilename string         // name="track" The filename of the uploaded file
	Daypass          string         // name="daypass"
	Comment          string         // name="comment"
	Replacement      *radio.TrackID // name="replacement"

	// after processing fields

	// File is the on-disk filename for the uploaded file
	File string
	// Song we managed to populate by analyzing the uploaded file
	Song *radio.PendingSong
}

func (SubmissionForm) TemplateBundle() string {
	return "submit"
}

func (SubmissionForm) TemplateName() string {
	return "form_submit"
}

// ParseForm parses a multipart form into the SubmissionForm
//
// Go has standard library support to do this simpler, but it doesn't let you set limits on individual fields,
// so we're parsing each field ourselves so we can limit their length.
//
// Fields supported:
//
//	"track":		audio file being submitted
//	"comment":		comment to be shown on the pending admin panel
//	"daypass":		daypass to bypass upload limits
//	"replacement":	an ID (number) indicating what song to replace in the database with this
//
// Any other fields cause an error to be returned and all form parsing to stop.
func NewSubmissionForm(tempdir string, mr *multipart.Reader) (*SubmissionForm, error) {
	const op errors.Op = "public.NewSubmissionForm"

	var sf SubmissionForm

	for i := 0; i < formMaxMIMEParts; i++ {
		part, err := mr.NextPart()
		if errors.IsE(err, io.EOF) {
			// finished reading parts
			return &sf, nil
		}
		if err != nil {
			return nil, errors.E(op, err, errors.InternalServer)
		}

		switch part.FormName() {
		case "track": // audio file that is being submitted
			err = func() error {
				// clean the extension from the user
				path := filepath.Clean("/" + part.FileName())
				ext := filepath.Ext(path)
				if !AllowedExtension(ext) {
					return errors.E(op, errors.InvalidForm, "extension not allowed", errors.Info(ext))
				}
				// remove any * because CreateTemp uses them for the random replacement
				ext = strings.ReplaceAll(ext, "*", "")

				f, err := os.CreateTemp(tempdir, "pending-*"+ext)
				if err != nil {
					return errors.E(op, err, "failed to create temp file")
				}
				defer f.Close()

				n, err := io.CopyN(f, part, formMaxFileLength)
				if err != nil && !errors.IsE(err, io.EOF) {
					os.Remove(f.Name())
					return errors.E(op, err, "copying to temp file failed")
				}
				if n >= formMaxFileLength {
					os.Remove(f.Name())
					return errors.E(op, err, "form was too big")
				}
				sf.OriginalFilename = part.FileName()
				sf.File = f.Name()
				return nil
			}()
			if err != nil {
				return nil, errors.E(op, err)
			}
		case "comment": // comment to be shown on the pending admin panel
			s, err := readString(part, formMaxCommentLength)
			if err != nil {
				return nil, errors.E(op, err)
			}
			sf.Comment = s
		case "daypass": // a daypass
			s, err := readString(part, formMaxDaypassLength)
			if err != nil {
				return nil, errors.E(op, err)
			}
			sf.Daypass = s
		case "replacement": // replacement track identifier
			s, err := readString(part, formMaxReplacementLength)
			if err != nil {
				return nil, errors.E(op, err)
			}
			id, err := strconv.Atoi(s)
			if err != nil {
				return nil, errors.E(op, err)
			}
			tid := radio.TrackID(id)
			sf.Replacement = &tid
		default:
			// unknown form field, we just cancel everything and return
			return nil, errors.E(op, errors.InvalidForm)
		}
	}

	return &sf, nil
}

// Validate checks if required fields are filled in the SubmissionForm and
// if a daypass was supplied if it was a valid one. Populates sf.Errors with
// any errors that occur and what input field caused it.
func (sf *SubmissionForm) Validate(ts radio.TrackStorage, dp *daypass.Daypass) bool {
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
	if sf.Daypass != "" {
		sf.IsDaypass = dp.Is(sf.Daypass)
		if !sf.IsDaypass {
			sf.Errors["daypass"] = "daypass invalid"
		}
	}
	if sf.Replacement != nil && *sf.Replacement != 0 {
		song, err := ts.Get(*sf.Replacement)
		if err != nil {
			sf.Errors["replacement"] = "TrackID does not exist"
		}
		if !song.NeedReplacement {
			sf.Errors["replacement"] = "TrackID does not need replacement"
		}
	}

	return len(sf.Errors) == 0
}

// AllowedExtension returns if ext is an allowed extension for the uploaded audio files
func AllowedExtension(ext string) bool {
	ext = strings.ToLower(ext)
	switch ext {
	case ".mp3":
		return true
	case ".flac":
		return true
	case ".ogg":
		return true
	}
	return false
}
