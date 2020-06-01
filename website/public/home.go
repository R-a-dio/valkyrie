package public

import (
	"log"
	"net/http"

	radio "github.com/R-a-dio/valkyrie"
	"github.com/R-a-dio/valkyrie/errors"
)

func (s State) GetHome(w http.ResponseWriter, r *http.Request) {
	err := s.getHome(w, r)
	if err != nil {
		log.Println(err)
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
		sharedInput

		Status     *radio.Status
		Queue      []radio.QueueEntry
		LastPlayed []radio.Song
		News       []radio.NewsPost
	}{
		Status:     status,
		Queue:      queue,
		LastPlayed: lp,
		News:       news.Entries,
	}

	err = s.Templates[theme]["home"].ExecuteDev(w, homeInput)
	if err != nil {
		return errors.E(op, errors.InternalServer, err)
	}

	return nil
}
