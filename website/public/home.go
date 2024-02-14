package public

import (
	"net/http"

	radio "github.com/R-a-dio/valkyrie"
	"github.com/R-a-dio/valkyrie/errors"
	"github.com/R-a-dio/valkyrie/website/shared"
	"github.com/rs/zerolog/hlog"
)

type HomeInput struct {
	shared.Input

	Status     *radio.Status
	Queue      []radio.QueueEntry
	LastPlayed []radio.Song
	News       []radio.NewsPost
}

func NewHomeInput(f *shared.InputFactory, r *http.Request) HomeInput {
	return HomeInput{
		Input: f.New(r),
	}
}

func (HomeInput) TemplateBundle() string {
	return "home"
}

func (s State) GetHome(w http.ResponseWriter, r *http.Request) {
	err := s.getHome(w, r)
	if err != nil {
		hlog.FromRequest(r).Error().Err(err).Msg("")
		return
	}
}

func (s State) getHome(w http.ResponseWriter, r *http.Request) error {
	const op errors.Op = "website/public.getHome"

	input := NewHomeInput(s.Shared, r)
	ctx := r.Context()

	status, err := s.Manager.Status(ctx)
	if err != nil {
		return errors.E(op, errors.InternalServer, err)
	}
	input.Status = status

	queue, err := s.Streamer.Queue(ctx)
	if err != nil {
		return errors.E(op, errors.InternalServer, err)
	}
	input.Queue = queue

	lp, err := s.Storage.Song(ctx).LastPlayed(0, 5)
	if err != nil {
		return errors.E(op, errors.InternalServer, err)
	}
	input.LastPlayed = lp

	news, err := s.Storage.News(ctx).ListPublic(3, 0)
	if err != nil {
		return errors.E(op, errors.InternalServer, err)
	}
	input.News = news.Entries

	return s.Templates.Execute(w, r, input)
}
