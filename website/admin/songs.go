package admin

import (
	"net/http"

	radio "github.com/R-a-dio/valkyrie"
	"github.com/R-a-dio/valkyrie/errors"
	"github.com/R-a-dio/valkyrie/website/middleware"
	"github.com/R-a-dio/valkyrie/website/shared"
	"github.com/rs/zerolog/hlog"
)

const songsPageSize = 20

type SongsInput struct {
	middleware.Input

	Forms []SongsForm
	Query string
	Page  *shared.Pagination
}

func (SongsInput) TemplateBundle() string {
	return "admin-songs"
}

type SongsForm struct {
	HasDelete bool
	HasEdit   bool
	Song      radio.Song
}

func (SongsForm) TemplateName() string {
	return "form_admin_songs"
}

func (SongsForm) TemplateBundle() string {
	return "admin-songs"
}

func NewSongsInput(s radio.SearchService, r *http.Request) (*SongsInput, error) {
	const op errors.Op = "website/admin.NewSongInput"
	ctx := r.Context()

	page, offset, err := shared.PageAndOffset(r, songsPageSize)
	if err != nil {
		return nil, errors.E(op, err)
	}

	query := r.FormValue("q")
	searchResult, err := s.Search(ctx, query, songsPageSize, offset)
	if err != nil {
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

	forms := make([]SongsForm, len(searchResult.Songs))
	for i := range searchResult.Songs {
		forms[i].Song = searchResult.Songs[i]
		forms[i].HasDelete = input.User.UserPermissions.Has(radio.PermDatabaseDelete)
		forms[i].HasEdit = input.User.UserPermissions.Has(radio.PermDatabaseEdit)
	}

	input.Forms = forms
	return input, nil
}

func (s *State) GetSongs(w http.ResponseWriter, r *http.Request) {
	input, err := NewSongsInput(s.Search, r)
	if err != nil {
		hlog.FromRequest(r).Error().Err(err).Msg("input creation failure")
		return
	}

	err = s.TemplateExecutor.Execute(w, r, input)
	if err != nil {
		hlog.FromRequest(r).Error().Err(err).Msg("template failure")
		return
	}
}

func (s *State) PostSongs(w http.ResponseWriter, r *http.Request) {

}

func (s *State) DeleteSongs(w http.ResponseWriter, r *http.Request) {

}
