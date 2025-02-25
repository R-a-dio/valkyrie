package admin

import (
	"fmt"
	"html/template"
	"net/http"

	radio "github.com/R-a-dio/valkyrie"
	"github.com/R-a-dio/valkyrie/errors"
	"github.com/R-a-dio/valkyrie/util"
	"github.com/R-a-dio/valkyrie/util/secret"
	"github.com/R-a-dio/valkyrie/website/middleware"
	"github.com/R-a-dio/valkyrie/website/shared"
	"github.com/gorilla/csrf"
)

const songsPageSize = 20

type SongsInput struct {
	middleware.Input

	Forms []SongsForm
	Query string
	Page  *shared.Pagination
}

func (SongsInput) TemplateBundle() string {
	return "database"
}

type SongsForm struct {
	CSRFTokenInput template.HTML

	Errors  map[string]string
	Success bool

	// HasDelete indicates if we should show the delete button
	HasDelete bool
	// HasEdit indicates if we should allow editing of the form
	HasEdit bool
	Song    radio.Song
	SongURL string
}

func (SongsForm) TemplateName() string {
	return "form_admin_songs"
}

func (SongsForm) TemplateBundle() string {
	return "database"
}

func NewSongsInput(s radio.SearchService, ss secret.Secret, r *http.Request) (*SongsInput, error) {
	const op errors.Op = "website/admin.NewSongInput"
	ctx := r.Context()

	page, offset, err := shared.PageAndOffset(r, songsPageSize)
	if err != nil {
		return nil, errors.E(op, err)
	}

	query := r.FormValue("q")
	searchResult, err := s.Search(ctx, query, radio.SearchOptions{
		Limit:  songsPageSize,
		Offset: offset,
	})
	if err != nil && !errors.Is(errors.SearchNoResults, err) {
		return nil, errors.E(op, err)
	}

	// generate the input we can so far, since we need some data from it
	input := &SongsInput{
		Input: middleware.InputFromContext(ctx),
		Query: query,
		Page: shared.NewPagination(
			page, shared.PageCount(int64(searchResult.TotalHits), songsPageSize),
			r.URL,
		),
	}

	csrfInput := csrf.TemplateField(r)
	hasDelete := input.User.UserPermissions.Has(radio.PermDatabaseDelete)
	hasEdit := input.User.UserPermissions.Has(radio.PermDatabaseEdit)
	forms := make([]SongsForm, len(searchResult.Songs))
	for i := range searchResult.Songs {
		forms[i].CSRFTokenInput = csrfInput
		forms[i].Song = searchResult.Songs[i]
		forms[i].HasDelete = hasDelete
		forms[i].HasEdit = hasEdit
		forms[i].SongURL = GenerateSongURL(ss, searchResult.Songs[i])
	}

	input.Forms = forms
	return input, nil
}

func GenerateSongURL(ss secret.Secret, song radio.Song) string {
	key := ss.Get(song.Hash[:])
	return fmt.Sprintf("/v1/song?key=%s&id=%d", key, song.TrackID)
}

func (s *State) GetSongs(w http.ResponseWriter, r *http.Request) {
	input, err := NewSongsInput(s.Search, s.SongSecret, r)
	if err != nil {
		s.errorHandler(w, r, err, "")
		return
	}

	err = s.TemplateExecutor.Execute(w, r, input)
	if err != nil {
		s.errorHandler(w, r, err, "")
		return
	}
}

func (s *State) PostSongs(w http.ResponseWriter, r *http.Request) {
	form, err := s.postSongs(r)
	if err != nil {
		s.errorHandler(w, r, err, "")
		return
	}

	if util.IsHTMX(r) {
		if form == nil {
			return
		}
		err = s.TemplateExecutor.Execute(w, r, form)
		if err != nil {
			s.errorHandler(w, r, err, "")
			return
		}
		return
	}

	// otherwise just return to the existing listing
	r, _ = util.RedirectBack(r) // TODO: why is this here?
	s.GetSongs(w, r)
}

func (s *State) postSongs(r *http.Request) (*SongsForm, error) {
	const op errors.Op = "website/admin.postSongs"
	ctx := r.Context()

	// parse the form explicitly, net/http otherwise eats any errors
	if err := r.ParseForm(); err != nil {
		return nil, errors.E(op, err, errors.InvalidForm)
	}

	ts := s.Storage.Track(r.Context())
	user := middleware.UserFromContext(ctx)
	if !user.IsValid() {
		return nil, errors.E(op, errors.AccessDenied)
	}

	// construct the new updated song form from the input
	form, err := NewSongsForm(ts, *user, r)
	if err != nil {
		return nil, errors.E(op, err)
	}

	// delete action is separate from all the others
	if r.Form.Get("action") == "delete" {
		// make sure the user has permission to do this, since the route
		// only checks for PermDatabaseEdit
		if !user.UserPermissions.Has(radio.PermDatabaseDelete) {
			return form, errors.E(op, errors.AccessDenied)
		}

		err = ts.Delete(form.Song.TrackID)
		if err != nil {
			return nil, errors.E(op, err)
		}

		// successfully deleted the song from the database, now we just
		// need to remove the file we have on-disk
		toRemovePath := util.AbsolutePath(s.Config.MusicPath(), form.Song.FilePath)

		err = s.FS.Remove(toRemovePath)
		if err != nil {
			return nil, errors.E(op, err, errors.InternalServer)
		}
		return nil, nil
	}

	// anything but delete is effectively an update
	err = ts.UpdateMetadata(form.Song)
	if err != nil {
		return form, errors.E(op, err, errors.InternalServer)
	}

	form.Success = true
	return form, nil
}

func NewSongsForm(ts radio.TrackStorage, user radio.User, r *http.Request) (*SongsForm, error) {
	const op errors.Op = "website/admin.NewSongsForm"

	var form SongsForm
	values := r.Form

	tid, err := radio.ParseTrackID(values.Get("id"))
	if err != nil {
		return nil, errors.E(op, err, errors.InvalidForm, errors.Info("missing or malformed id in form"))
	}

	song, err := ts.Get(tid)
	if err != nil {
		return nil, errors.E(op, err, errors.InvalidForm)
	}

	song.Artist = values.Get("artist")
	song.Album = values.Get("album")
	song.Title = values.Get("title")
	song.Tags = values.Get("tags")

	if values.Get("action") == "mark-replacement" {
		song.NeedReplacement = true
	} else if values.Get("action") == "unmark-replacement" {
		song.NeedReplacement = false
	}
	song.LastEditor = user.Username

	form.Song = *song
	form.HasDelete = user.UserPermissions.Has(radio.PermDatabaseDelete)
	form.HasEdit = user.UserPermissions.Has(radio.PermDatabaseEdit)
	form.CSRFTokenInput = csrf.TemplateField(r)
	return &form, nil
}

func (sf *SongsForm) Validate() bool {
	sf.Errors = make(map[string]string)

	if len(sf.Song.Artist) > radio.LimitArtistLength {
		sf.Errors["artist"] = "artist name too long"
	}
	if len(sf.Song.Title) > radio.LimitTitleLength {
		sf.Errors["title"] = "title too long"
	}
	if len(sf.Song.Album) > radio.LimitAlbumLength {
		sf.Errors["album"] = "album name too long"
	}

	return len(sf.Errors) == 0
}
