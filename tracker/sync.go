package tracker

import (
	"context"
	"net/http"

	radio "github.com/R-a-dio/valkyrie"
	"github.com/R-a-dio/valkyrie/config"
	"github.com/R-a-dio/valkyrie/errors"
	"github.com/rs/zerolog"
)

func (r *Recorder) Sync(ctx context.Context, other []radio.Listener) {
	var new = make(map[radio.ListenerClientID]*radio.Listener, len(other))
	var highestID radio.ListenerClientID
	for _, o := range other {
		highestID = max(highestID, o.ID)
		new[o.ID] = &o
	}

	// first remove any entries that exist in our current live data, but not
	// in the sync data
	r.listeners.Range(func(id radio.ListenerClientID, value *Listener) bool {
		if id >= highestID {
			// if the entry ID is above the highest sync ID it probably means
			// a new listener has appeared between us getting the sync data and
			// this range loop starting, so skip them
			zerolog.Ctx(ctx).Info().Msg("skipping listener because ID is higher")
			return true
		}

		if _, ok := new[id]; !ok {
			// entry doesn't exist in the new data
			r.ListenerRemove(ctx, id)
		}
		return true
	})

	// then we add the entries that are in the sync data, but not yet in our live
	// data
	for _, v := range other {
		r.ListenerAdd(ctx, v)
	}
}

func GetIcecastListClients(ctx context.Context, cfg config.Config) ([]radio.Listener, error) {
	const op errors.Op = "tracker/GetIcecastListClients"
	conf := cfg.Conf()

	uri := conf.Tracker.MasterServer.URL()
	uri.Path = "/admin/listclients"
	query := uri.Query()
	query.Add("mount", cfg.Conf().Tracker.MountName)
	uri.RawQuery = query.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, uri.String(), nil)
	if err != nil {
		return nil, errors.E(op, err)
	}
	req.SetBasicAuth(conf.Tracker.MasterUsername, conf.Tracker.MasterPassword)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, errors.E(op, err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, errors.E(op, http.StatusText(resp.StatusCode))
	}

	res, err := ParseListClientsXML(resp.Body)
	if err != nil {
		return nil, errors.E(op, err)
	}
	return res, nil
}
