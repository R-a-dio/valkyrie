package v1

import (
	"context"

	radio "github.com/R-a-dio/valkyrie"
	"github.com/R-a-dio/valkyrie/config"
	"github.com/R-a-dio/valkyrie/search"
	"github.com/R-a-dio/valkyrie/storage"
	"github.com/R-a-dio/valkyrie/templates"
	"github.com/go-chi/chi/v5"
)

func NewAPI(ctx context.Context, cfg config.Config, templates templates.Executor) (*API, error) {
	song, err := storage.Open(ctx, cfg)
	if err != nil {
		return nil, err
	}
	se, err := search.Open(ctx, cfg)
	if err != nil {
		return nil, err
	}

	api := &API{
		Context:   ctx,
		Config:    cfg,
		Search:    se,
		Templates: templates,
		sse:       NewStream(templates),
		manager:   cfg.Conf().Manager.Client(),
		streamer:  cfg.Conf().Streamer.Client(),
		song:      song,
	}

	// start up status updates
	api.runStatusUpdates(ctx)

	return api, nil
}

type API struct {
	Context   context.Context
	Config    config.Config
	Search    radio.SearchService
	Templates templates.Executor
	sse       *Stream
	manager   radio.ManagerService
	streamer  radio.StreamerService
	song      radio.SongStorageService
}

func (a *API) Route(r chi.Router) {
	r.Get("/sse", a.sse.ServeHTTP)
	r.Get("/search", a.SearchHTML)
}
