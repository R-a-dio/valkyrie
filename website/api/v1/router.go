package v1

import (
	"context"
	"log"
	"time"

	radio "github.com/R-a-dio/valkyrie"
	"github.com/R-a-dio/valkyrie/config"
	"github.com/R-a-dio/valkyrie/util/eventstream"
	"github.com/go-chi/chi/v5"
)

func NewAPI(ctx context.Context, cfg config.Config) (*API, error) {
	api := &API{
		Context: ctx,
		Config:  cfg,
		sse:     NewStream(),
		manager: cfg.Conf().Manager.Client(),
	}

	go func() {
		defer api.sse.Shutdown()

		m := cfg.Conf().Manager.Client()

		var s eventstream.Stream[*radio.SongUpdate]
		var err error
		for {
			s, err = m.CurrentSong(ctx)
			if err == nil {
				break
			}

			log.Println("v1/api:setup:", err)
			time.Sleep(time.Second * 3)
		}

		for {
			us, err := s.Next()
			if err != nil {
				log.Println("v1/api:loop:", err)
				break
			}
			if us == nil {
				log.Println("v1/api:loop: nil value")
				continue
			}

			log.Println("v1/api:sending:", us.Metadata)
			api.sse.SendEvent(EventMetadata, []byte(us.Metadata))
		}
	}()

	return api, nil
}

type API struct {
	Context context.Context
	Config  config.Config
	sse     *Stream
	manager radio.ManagerService
}

func (a *API) Router() chi.Router {
	r := chi.NewRouter()

	r.Get("/sse", a.sse.ServeHTTP)
	return r
}
