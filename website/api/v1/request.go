package v1

import (
	"net/http"
	"time"

	radio "github.com/R-a-dio/valkyrie"
	"github.com/R-a-dio/valkyrie/errors"
	"github.com/R-a-dio/valkyrie/templates"
	"github.com/R-a-dio/valkyrie/util"
	"github.com/R-a-dio/valkyrie/website/public"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/hlog"
)

func (a *API) PostRequest(w http.ResponseWriter, r *http.Request) {
	var message string

	err := a.postRequest(r)
	if err != nil {
		switch {
		case errors.Is(errors.SongCooldown, err):
			message = "Song is on cooldown"
		case errors.Is(errors.UserCooldown, err):
			message = "You can't request yet"
		case errors.Is(errors.StreamerNoRequests, err):
			message = "Requests are currently disabled"
		case errors.Is(errors.InvalidForm, err):
			message = "Invalid form in request"
		case errors.Is(errors.SongUnknown, err):
			message = "Unknown song, how did you get here?"
		default:
			message = "something broke, report to IRC."
			hlog.FromRequest(r).Error().Err(err).Msg("request failed")
		}
	}

	if !util.IsHTMX(r) {
		// for non-htmx users we redirect them back to where they came from
		r, ok := util.RedirectBack(r)
		if !ok {
			// or to the homepage if RedirectBack fails
			r = util.RedirectPath(r, "/")
		}
		// use 303 (See Other) so that it does a GET request instead of a POST
		http.Redirect(w, r, r.URL.String(), http.StatusSeeOther)
		return
	}

	var input templates.TemplateSelectable
	ctx := r.Context()

	// figure out where our request came from
	if source := r.FormValue("s"); source == "fave" {
		fi, err := public.NewFavesInput(
			a.storage.Song(ctx),
			a.storage.Request(ctx),
			r,
			time.Duration(a.Config.Conf().UserRequestDelay),
		)
		if err != nil {
			zerolog.Ctx(ctx).Error().Err(err).Msg("")
			return
		}
		// TODO: message handling
		input = fi
	} else {
		si, err := public.NewSearchInput(
			a.Search,
			a.storage.Request(r.Context()),
			r,
			time.Duration(a.Config.Conf().UserRequestDelay),
		)
		if err != nil {
			zerolog.Ctx(ctx).Error().Err(err).Msg("")
			return
		}
		if message == "" {
			si.Message = "Thank you for requesting"
		} else {
			si.Message = message
			si.IsError = true
		}

		if source == "navbar" {
			input = SearchInput{si.SearchSharedInput}
		} else {
			input = si
		}
	}

	err = a.Templates.Execute(w, r, input)
	if err != nil {
		hlog.FromRequest(r).Error().Err(err).Msg("template failure")
		return
	}
}

func (a *API) postRequest(r *http.Request) error {
	const op errors.Op = "website/api/v1/API.postRequest"

	ctx := r.Context()

	tid, err := radio.ParseTrackID(r.FormValue("trackid"))
	if err != nil {
		return errors.E(op, err, errors.InvalidForm)
	}

	song, err := a.storage.Track(ctx).Get(tid)
	if err != nil {
		return errors.E(op, err, errors.SongUnknown)
	}

	err = a.streamer.RequestSong(ctx, *song, r.RemoteAddr)
	if err != nil {
		return err
	}

	return nil
}
