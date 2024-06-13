package v1

import (
	"context"
	"net/http"

	radio "github.com/R-a-dio/valkyrie"
	"github.com/R-a-dio/valkyrie/config"
	"github.com/R-a-dio/valkyrie/search"
	"github.com/R-a-dio/valkyrie/storage"
	"github.com/R-a-dio/valkyrie/templates"
	"github.com/R-a-dio/valkyrie/util/secret"
	"github.com/R-a-dio/valkyrie/website/shared"
	"github.com/go-chi/chi/v5"
	"github.com/spf13/afero"
)

func NewAPI(ctx context.Context, cfg config.Config,
	templates templates.Executor,
	fs afero.Fs,
	songSecret secret.Secret) (*API, error) {
	sg, err := storage.Open(ctx, cfg)
	if err != nil {
		return nil, err
	}
	se, err := search.Open(ctx, cfg)
	if err != nil {
		return nil, err
	}

	api := &API{
		Context:    ctx,
		Config:     cfg,
		Search:     se,
		Templates:  templates,
		sse:        NewStream(templates),
		manager:    cfg.Manager,
		streamer:   cfg.Streamer,
		storage:    sg,
		songSecret: songSecret,
		fs:         fs,
	}

	// start up status updates
	api.runStatusUpdates(ctx)

	return api, nil
}

type API struct {
	Context    context.Context
	Config     config.Config
	Search     radio.SearchService
	Templates  templates.Executor
	sse        *Stream
	manager    radio.ManagerService
	streamer   radio.StreamerService
	storage    radio.StorageService
	songSecret secret.Secret
	fs         afero.Fs
}

func (a *API) Route(r chi.Router) {
	r.Get("/sse", a.sse.ServeHTTP)
	r.Get("/search", a.SearchHTML)
	r.Get("/song", a.GetSong)
	r.Post("/request", a.PostRequest)
}

func (a *API) Shutdown() error {
	a.sse.Shutdown()
	return nil
}

func (a *API) errorHandler(w http.ResponseWriter, r *http.Request, err error) {
	shared.ErrorHandler(a.Templates, w, r, err)
}
