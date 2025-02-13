package public

import (
	"context"
	"html/template"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	radio "github.com/R-a-dio/valkyrie"
	"github.com/R-a-dio/valkyrie/errors"
	"github.com/R-a-dio/valkyrie/streamer/audio"
	"github.com/R-a-dio/valkyrie/templates/functions"
	"github.com/R-a-dio/valkyrie/util"
	"github.com/R-a-dio/valkyrie/util/secret"
	"github.com/R-a-dio/valkyrie/website/middleware"
	"github.com/gorilla/csrf"
	"github.com/rs/zerolog"
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

func NewSubmitInput(ts radio.TrackStorage, ss radio.SubmissionStorage, r *http.Request, form *SubmissionForm) (*SubmitInput, error) {
	const op errors.Op = "website.NewSubmitInput"

	if form == nil {
		tmp := newSubmissionForm(ts, r, nil)
		form = &tmp
	}

	identifier, _ := getIdentifier(r)
	stats, err := ss.SubmissionStats(identifier)
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

func (s *State) canSubmitSong(r *http.Request) (time.Duration, error) {
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
	if since > s.Config.UserUploadDelay() { // cooldown has passed so can submit song
		return 0, nil
	}

	daypass := r.Header.Get(daypassHeader)
	if s.Daypass.Equal(daypass, nil) { // daypass was used so can submit song
		return 0, nil
	}

	return since, nil
}

func (s *State) GetSubmit(w http.ResponseWriter, r *http.Request) {
	err := s.getSubmit(w, r, nil)
	if err != nil {
		hlog.FromRequest(r).Error().Ctx(r.Context()).Err(err).Msg("")
		return
	}
}

func (s *State) getSubmit(w http.ResponseWriter, r *http.Request, form *SubmissionForm) error {
	const op errors.Op = "website.getSubmit"

	ctx := r.Context()
	input, err := NewSubmitInput(
		s.Storage.Track(ctx),
		s.Storage.Submissions(ctx), r, form)
	if err != nil {
		return errors.E(op, err)
	}

	return s.Templates.Execute(w, r, input)
}

func (s *State) PostSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

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
	form, err := s.postSubmit(r)
	if err != nil {
		// for unknown reason if we send a response without reading the body the connection is
		// hard-reset instead and our response goes missing, so discard the body up to our
		// allowed max size and then cut off if required
		_ = http.NewResponseController(w).SetReadDeadline(time.Now().Add(time.Minute))
		_, _ = io.CopyN(io.Discard, r.Body, formMaxSize)

		s.errorHandler(w, r, err)
		return
	}
	// if any form errors occurred return here
	if len(form.Errors) > 0 {
		responseFn(form)
		return
	}

	// success, update the submission time for the identifier
	identifier, _ := getIdentifier(r)
	err = s.Storage.Submissions(ctx).UpdateSubmissionTime(identifier)
	if err != nil {
		hlog.FromRequest(r).Error().Ctx(ctx).Err(err).Msg("failed updating submission time")
		responseFn(form)
		return
	}

	// send a new empty form back to the user
	back := newSubmissionForm(s.Storage.Track(ctx), r, nil)
	back.Success = true
	if form.IsDaypass {
		// if the submission was with a daypass, prefill the daypass for them again
		back.Daypass = form.Daypass
		back.IsDaypass = true
	}

	responseFn(back)
}

func (s *State) postSubmit(r *http.Request) (SubmissionForm, error) {
	const op errors.Op = "website.PostSubmit"
	ctx := r.Context()

	// find out if the client is allowed to upload
	cooldown, err := s.canSubmitSong(r)
	if err != nil {
		return newSubmissionForm(s.Storage.Track(ctx), r, nil), errors.E(op, err)
	}
	if cooldown > 0 {
		// submitter has a cooldown to wait out
		return newSubmissionForm(s.Storage.Track(ctx), r, map[string]string{
			"cooldown": functions.PrettyDuration(cooldown),
		}), nil
	}

	// start parsing the form, it's multipart encoded due to file upload and we manually
	// handle some details due to reasons described in NewSubmissionForm
	form, err := NewSubmissionForm(s.Storage.Track(r.Context()), filepath.Join(s.Config.MusicPath(), "pending"), r)
	if err != nil {
		if form == nil {
			return newSubmissionForm(s.Storage.Track(ctx), r, nil), errors.E(op, err)
		}
		return *form, errors.E(op, err)
	}

	// ParseForm just saved a temporary file that we want to delete if any other error
	// occurs after this, so prep a defer for it that we can no-op later
	var tmpFilename = form.File
	defer func() {
		if tmpFilename != "" {
			os.Remove(tmpFilename)
		}
	}()

	// Run a sanity check on the form input, this should also catch any errors that
	// occured during the forms parsing above
	if !form.Validate(s.Storage.Track(r.Context()), s.Daypass) {
		return *form, nil
	}

	// Probe the uploaded file for more information
	song, err := PendingFromProbe(form.File)
	if err != nil {
		// the error here can either be that something is wrong with our ffprobe setup
		// (file missing or something) or that the file just failed to pass through. We
		// are going to assume it's the latter, but add a log of the actual error so we
		// can atleast tell if the former happened at a later point
		zerolog.Ctx(ctx).Error().Ctx(ctx).Err(err).Msg("failed to probe submitted file")
		form.Errors["track"] = "file is invalid."
		return *form, nil
	}

	// Copy information over from the form and request to the PendingSong
	song.Comment = form.Comment
	song.Filename = form.OriginalFilename
	song.UserIdentifier, _ = getIdentifier(r)
	if form.Replacement != nil {
		song.ReplacementID = form.Replacement
	}
	song.SubmittedAt = time.Now()
	form.Song = song

	// Add the pending entry to the database
	err = s.Storage.Submissions(r.Context()).InsertSubmission(*song)
	if err != nil {
		form.Errors["track"] = "internal error, yell at someone in IRC"
		return *form, errors.E(op, err, errors.InternalServer)
	}

	// clear the tmpFilename so that it doesn't get deleted after we return
	tmpFilename = ""
	return *form, nil
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
		Bitrate:     uint(info.Bitrate),
		Length:      info.Duration,
	}

	return &s, nil
}

// SubmissionForm is the form struct passed to the submit page templates as .Form
type SubmissionForm struct {
	CSRFTokenInput template.HTML
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
	OriginalFilename    string         // name="track" The filename of the uploaded file
	Daypass             string         // name="daypass"
	Comment             string         // name="comment"
	Replacement         *radio.TrackID // name="replacement"
	NeedReplacementList []radio.Song   // possible songs for Replacement field

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

func newSubmissionForm(ts radio.TrackStorage, r *http.Request, errs map[string]string) SubmissionForm {
	form := SubmissionForm{
		CSRFTokenInput: csrf.TemplateField(r),
		Errors:         errs,
	}

	needReplacement, err := ts.NeedReplacement()
	if err != nil {
		hlog.FromRequest(r).Error().Ctx(r.Context()).Err(err).Msg("")
		return form
	}

	form.NeedReplacementList = needReplacement
	return form
}

// NewSubmissionForm parses a multipart form into the SubmissionForm
//
// Go has standard library support to do this simpler, but it doesn't let you set limits on individual fields,
// so we're parsing each field ourselves so we can limit their length.
//
// Fields supported:
//
//	"track":		audio file being submitted
//	"comment":		comment to be shown on the pending admin panel
//	"daypass":		daypass to bypass upload limits
//	"replacement":	an TrackID (number) indicating what song to replace in the database with this
//
// Any other fields cause an error to be returned and all form parsing to stop.
func NewSubmissionForm(ts radio.TrackStorage, tempdir string, r *http.Request) (*SubmissionForm, error) {
	const op errors.Op = "public.NewSubmissionForm"

	sf := newSubmissionForm(ts, r, map[string]string{})

	err := r.ParseMultipartForm(formMaxSize)
	if err != nil {
		return nil, errors.E(op, err, errors.InvalidForm)
	}

	getValue := func(req *http.Request, name string) string {
		if req.MultipartForm == nil || req.MultipartForm.Value == nil {
			return ""
		}

		v := req.MultipartForm.Value[name]
		if len(v) > 0 {
			return v[0]
		}
		return ""
	}

	sf.Comment = getValue(r, "comment")
	sf.Daypass = getValue(r, "daypass")

	if replacement := getValue(r, "replacement"); replacement != "" {
		tid, err := radio.ParseTrackID(replacement)
		if err != nil {
			// invalid replacement form value, probably not a number
			sf.Errors["replacement"] = "not a number"
			return &sf, nil
		}
		if tid != 0 { // 0 is our no replacement indicator
			sf.Replacement = &tid
		}
	}

	// now handle the uploaded file
	tracks := r.MultipartForm.File["track"]
	if len(tracks) != 1 {
		if len(tracks) == 0 {
			// no files uploaded
			sf.Errors["track"] = "no file selected"
		} else {
			// too many files uploaded
			sf.Errors["track"] = "too many files selected"
		}

		return &sf, nil
	}
	track := tracks[0]

	// we want to use the extension given by the user as our extension
	// on the server, but we can't trust it yet. Run clean on it and
	// then check if we allow this extension
	path := filepath.Clean("/" + track.Filename)
	ext := filepath.Ext(path)
	if !AllowedExtension(ext) {
		// not an allowed extension
		sf.Errors["track"] = "file format not allowed"
		return &sf, nil
	}
	// remove any * because CreateTemp uses them for the random replacement
	ext = strings.ReplaceAll(ext, "*", "")

	// create the resting place for the uploaded file
	f, err := os.CreateTemp(tempdir, "pending-*"+ext)
	if err != nil {
		return &sf, errors.E(op, err, errors.InternalServer)
	}
	defer f.Close()

	// try to change its permissions, since CreateTemp gives us 0600
	if err = f.Chmod(0664); err != nil {
		// not a critical error, but we do log it
		hlog.FromRequest(r).Warn().Str("filename", f.Name()).Msg("failed to adjust file modes")
	}

	// open the uploaded file for copying
	uploaded, err := track.Open()
	if err != nil {
		return &sf, errors.E(op, err, errors.InternalServer)
	}
	defer uploaded.Close()

	// copy over the uploaded file to the resting place
	n, err := io.CopyN(f, uploaded, formMaxFileLength)
	if err != nil && !errors.IsE(err, io.EOF) {
		os.Remove(f.Name())
		return &sf, errors.E(op, err, errors.InternalServer)
	}
	if n >= formMaxFileLength {
		os.Remove(f.Name())
		sf.Errors["track"] = "file too large"
		return &sf, nil
	}

	// record the filename the user supplied
	sf.OriginalFilename = track.Filename
	// and the filename of the resting place
	sf.File = f.Name()
	return &sf, nil
}

// Validate checks if required fields are filled in the SubmissionForm and
// if a daypass was supplied if it was a valid one. Populates sf.Errors with
// any errors that occur and what input field caused it.
func (sf *SubmissionForm) Validate(ts radio.TrackStorage, dp secret.Secret) bool {
	if sf.Errors == nil {
		sf.Errors = make(map[string]string)
	}
	if sf.Errors["track"] == "" {
		// only add track errors if there isn't already one
		if sf.File == "" {
			sf.Errors["track"] = "no temporary file"
		}
		if sf.OriginalFilename == "" {
			sf.Errors["track"] = "no file selected"
		}
	}
	if sf.Comment == "" {
		sf.Errors["comment"] = "no comment supplied"
	}
	if sf.Daypass != "" {
		sf.IsDaypass = dp.Equal(sf.Daypass, nil)
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
