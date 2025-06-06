package public

import (
	"net/http"

	radio "github.com/R-a-dio/valkyrie"
	"github.com/R-a-dio/valkyrie/errors"
	"github.com/R-a-dio/valkyrie/website/middleware"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/hlog"
)

type HomeInput struct {
	middleware.Input

	Queue      radio.Queue
	LastPlayed []radio.Song
	News       []NewsInputPost
}

func NewHomeInput(r *http.Request) HomeInput {
	return HomeInput{
		Input: middleware.InputFromRequest(r),
	}
}

func (HomeInput) TemplateBundle() string {
	return "home"
}

func (s *State) GetHome(w http.ResponseWriter, r *http.Request) {
	err := s.getHome(w, r)
	if err != nil {
		hlog.FromRequest(r).Error().Ctx(r.Context()).Err(err).Msg("")
		return
	}
}

func (s *State) getHome(w http.ResponseWriter, r *http.Request) error {
	const op errors.Op = "website/public.getHome"

	input := NewHomeInput(r)
	ctx := r.Context()

	queue, err := s.Queue.Entries(ctx)
	if err != nil {
		zerolog.Ctx(ctx).Error().Ctx(r.Context()).Err(err).Msg("failed to retrieve queue")
	}
	input.Queue = queue

	lp, err := s.Storage.Song(ctx).LastPlayed(radio.LPKeyLast, 5)
	if err != nil {
		return errors.E(op, errors.InternalServer, err)
	}
	input.LastPlayed = lp

	news, err := s.Storage.News(ctx).ListPublic(3, 0)
	if err != nil {
		return errors.E(op, errors.InternalServer, err)
	}

	input.News, err = AsNewsInputPost(ctx, s.News, news.Entries)
	if err != nil {
		return errors.E(op, errors.InternalServer)
	}

	return s.Templates.Execute(w, r, input)
}
