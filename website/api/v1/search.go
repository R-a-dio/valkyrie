package v1

import (
	"net/http"

	radio "github.com/R-a-dio/valkyrie"
	"github.com/R-a-dio/valkyrie/errors"
	"github.com/rs/zerolog/hlog"
)

type SearchInput struct {
	Result *radio.SearchResult
}

func (SearchInput) TemplateBundle() string {
	return "search"
}

func (SearchInput) TemplateName() string {
	return "search-api"
}

func (a *API) SearchHTML(w http.ResponseWriter, r *http.Request) {
	const op errors.Op = "website/api/v1.API.SearchHTML"

	err := r.ParseForm()
	if err != nil {
		hlog.FromRequest(r).Error().Err(err)
		return
	}

	res, err := a.Search.Search(r.Context(), r.Form.Get("q"), 50, 0)
	if err != nil {
		err = errors.E(op, err, errors.InternalServer)
		hlog.FromRequest(r).Error().Err(err).Msg("database error")
		return
	}
	input := SearchInput{
		Result: res,
	}

	if input.Result.TotalHits == 0 {
		return
	}

	err = a.Templates.Execute(w, r, input)
	if err != nil {
		err = errors.E(op, err, errors.InternalServer)
		hlog.FromRequest(r).Error().Err(err).Msg("template error")
		return
	}
}
