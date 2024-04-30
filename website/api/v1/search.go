package v1

import (
	"net/http"
	"time"

	"github.com/R-a-dio/valkyrie/errors"
	"github.com/R-a-dio/valkyrie/website/public"
	"github.com/rs/zerolog/hlog"
)

const searchPageSize = 50

func (a *API) SearchHTML(w http.ResponseWriter, r *http.Request) {
	const op errors.Op = "website/api/v1.API.SearchHTML"

	input, err := public.NewSearchSharedInput(
		a.Search,
		a.storage.Request(r.Context()),
		r,
		time.Duration(a.Config.Conf().UserRequestDelay),
		searchPageSize,
	)
	if err != nil {
		hlog.FromRequest(r).Error().Err(err)
		return
	}

	if len(input.Songs) == 0 {
		return
	}

	err = a.Templates.Execute(w, r, SearchInput{*input})
	if err != nil {
		err = errors.E(op, err, errors.InternalServer)
		hlog.FromRequest(r).Error().Err(err).Msg("template error")
		return
	}
}

type SearchInput struct {
	public.SearchSharedInput
}

func (SearchInput) TemplateName() string {
	return "search-api"
}

func (SearchInput) TemplateBundle() string {
	return "search"
}
