package public

import (
	"net/http"

	radio "github.com/R-a-dio/valkyrie"
	"github.com/R-a-dio/valkyrie/errors"
	"github.com/R-a-dio/valkyrie/website/middleware"
	"github.com/rs/zerolog/hlog"
)

func (s State) GetHome(w http.ResponseWriter, r *http.Request) {
	err := s.getHome(w, r)
	if err != nil {
		hlog.FromRequest(r).Error().Err(err).Msg("")
	}
}

func (s State) getHome(w http.ResponseWriter, r *http.Request) error {
	const op errors.Op = "website/public.getHome"

	ctx := r.Context()

	status, err := s.Manager.Status(ctx)
	if err != nil {
		return errors.E(op, errors.InternalServer, err)
	}

	queue, err := s.Streamer.Queue(ctx)
	if err != nil {
		return errors.E(op, errors.InternalServer, err)
	}

	lp, err := s.Storage.Song(ctx).LastPlayed(0, 5)
	if err != nil {
		return errors.E(op, errors.InternalServer, err)
	}

	news, err := s.Storage.News(ctx).ListPublic(3, 0)
	if err != nil {
		return errors.E(op, errors.InternalServer, err)
	}

	homeInput := struct {
		shared

		Status     *radio.Status
		Queue      []radio.QueueEntry
		LastPlayed []radio.Song
		News       []radio.NewsPost
	}{
		shared:     s.shared(r),
		Status:     status,
		Queue:      queue,
		LastPlayed: lp,
		News:       news.Entries,
	}

	theme := middleware.GetTheme(ctx)
	err = s.TemplateExecutor.ExecuteFull(theme, "home", w, homeInput)
	if err != nil {
		return err
	}

	return nil
}
