package v1

import (
	"context"

	radio "github.com/R-a-dio/valkyrie"
	"github.com/R-a-dio/valkyrie/config"
	"github.com/R-a-dio/valkyrie/storage"
	"github.com/R-a-dio/valkyrie/templates"
	"github.com/go-chi/chi/v5"
)

func NewAPI(ctx context.Context, cfg config.Config, templates *templates.Executor) (*API, error) {
	song, err := storage.Open(ctx, cfg)
	if err != nil {
		return nil, err
	}

	api := &API{
		Context:  ctx,
		Config:   cfg,
		sse:      NewStream(templates),
		manager:  cfg.Conf().Manager.Client(),
		streamer: cfg.Conf().Streamer.Client(),
		song:     song,
	}

	go api.runSSE(ctx)

	return api, nil
}

type API struct {
	Context  context.Context
	Config   config.Config
	sse      *Stream
	manager  radio.ManagerService
	streamer radio.StreamerService
	song     radio.SongStorageService
}

func (a *API) Router() chi.Router {
	r := chi.NewRouter()

	r.Get("/sse", a.sse.ServeHTTP)
	return r
}
