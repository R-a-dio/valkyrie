package shared

import (
	"net/http"

	"github.com/R-a-dio/valkyrie/errors"
	"github.com/R-a-dio/valkyrie/templates"
	"github.com/R-a-dio/valkyrie/website/middleware"
	"github.com/rs/xid"
	"github.com/rs/zerolog/hlog"
)

type ErrorInput struct {
	middleware.Input
	Message   string
	Error     error
	RequestID xid.ID
}

func (ErrorInput) TemplateBundle() string {
	return "error"
}

var (
	ErrNotFound         = errors.New("page not found")
	ErrMethodNotAllowed = errors.New("method not allowed")
)

func ErrorHandler(exec templates.Executor, w http.ResponseWriter, r *http.Request, err error) {
	hlog.FromRequest(r).Error().Err(err).Msg("")

	var msg string

	switch {
	case errors.IsE(err, ErrNotFound):
		msg = "page not found"
		w.WriteHeader(http.StatusNotFound)
	case errors.IsE(err, ErrMethodNotAllowed):
		msg = "method not allowed"
		w.WriteHeader(http.StatusMethodNotAllowed)
	case errors.Is(errors.InvalidForm, err):
		msg = "form is invalid"
		w.WriteHeader(http.StatusBadRequest)
	default:
		msg = "internal server error"
		w.WriteHeader(http.StatusInternalServerError)
	}

	rid, _ := hlog.IDFromRequest(r)

	input := ErrorInput{
		Input:     middleware.InputFromRequest(r),
		Message:   msg,
		Error:     err,
		RequestID: rid,
	}

	err = exec.Execute(w, r, input)
	if err != nil {
		hlog.FromRequest(r).Error().Err(err).Msg("error while rendering error page")
		http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
	}
}
