package v1

import (
	"net/http"
	"time"

	radio "github.com/R-a-dio/valkyrie"
	"github.com/R-a-dio/valkyrie/errors"
	"github.com/R-a-dio/valkyrie/util"
	"github.com/rs/zerolog/hlog"
)

func (a *API) PostRequest(w http.ResponseWriter, r *http.Request) {
	res := a.postRequest(r)

	if !util.IsHTMX(r) {
		// for non-htmx users we redirect them back to where they came from
		r = util.RedirectBack(r)
		// use 303 (See Other) so that it does a GET request instead of a POST
		http.Redirect(w, r, r.URL.String(), http.StatusSeeOther)
		return
	}

	err := a.Templates.Execute(w, r, &res)
	if err != nil {
		hlog.FromRequest(r).Error().Err(err).Msg("template failure")
		return
	}
}

type RequestInput struct {
	radio.Song

	Message string
	Error   string
}

func (RequestInput) TemplateBundle() string {
	return "search"
}

func (RequestInput) TemplateName() string {
	return "request-response"
}

func (a *API) postRequest(r *http.Request) RequestInput {
	var res RequestInput

	ctx := r.Context()

	tid, err := radio.ParseTrackID(r.FormValue("trackid"))
	if err != nil {
		hlog.FromRequest(r).Error().Err(err).Msg("invalid request form")
		res.Error = "Invalid Request"
		return res
	}

	song, err := a.storage.Track(ctx).Get(tid)
	if err != nil {
		hlog.FromRequest(r).Error().Err(err).Msg("invalid request form")
		res.Error = "Unknown Song"
		return res
	}
	res.Song = *song

	err = a.streamer.RequestSong(ctx, *song, r.RemoteAddr)
	if err != nil {
		switch {
		case errors.Is(errors.SongCooldown, err):
			res.Error = "Song is on cooldown"
		case errors.Is(errors.UserCooldown, err):
			res.Error = "You can't request yet"
		case errors.Is(errors.StreamerNoRequests, err):
			res.Error = "Requests are disabled"
		default:
			res.Error = "something broke, report to IRC."
			hlog.FromRequest(r).Error().Err(err).Msg("request failed")
		}
		return res
	}

	res.Song.LastRequested = time.Now()
	res.Message = "Thanks for requesting"
	return res
}
