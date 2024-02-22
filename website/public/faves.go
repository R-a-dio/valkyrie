package public

import (
	"net/http"

	radio "github.com/R-a-dio/valkyrie"
	"github.com/R-a-dio/valkyrie/website/middleware"
	"github.com/R-a-dio/valkyrie/website/shared"
)

const favesPageSize = 100

type FavesInput struct {
	middleware.Input
	Nickname string
	Faves    []radio.Song
	Page     *shared.Pagination
}

func (FavesInput) TemplateBundle() string {
	return "faves"
}

func NewFavesInput(ss radio.SongStorage, r *http.Request) (*FavesInput, error) {
	page, offset, err := getPageOffset(r, favesPageSize)
	if err != nil {
		return nil, err
	}
	_ = offset

	nickname := r.FormValue("nick")
	faves, err := ss.FavoritesOf(nickname)
	if err != nil {
		return nil, err
	}

	return &FavesInput{
		Nickname: nickname,
		Faves:    faves,
		Page: shared.NewPagination(
			page, shared.PageCount(int64(len(faves)), favesPageSize),
			r.URL,
		),
		Input: middleware.InputFromRequest(r),
	}, nil
}

func (s State) GetFaves(w http.ResponseWriter, r *http.Request) {
	input, err := NewFavesInput(s.Storage.Song(r.Context()), r)
	if err != nil {
		s.errorHandler(w, r, err)
		return
	}

	err = s.Templates.Execute(w, r, input)
	if err != nil {
		s.errorHandler(w, r, err)
		return
	}
}

func (s State) PostFaves(w http.ResponseWriter, r *http.Request) {
}
