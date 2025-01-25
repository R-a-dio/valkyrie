package shared

import (
	"net/http"

	"github.com/R-a-dio/valkyrie/errors"
	"github.com/R-a-dio/valkyrie/templates"
	"github.com/R-a-dio/valkyrie/util"
	"github.com/R-a-dio/valkyrie/website/middleware"
	"github.com/rs/xid"
	"github.com/rs/zerolog/hlog"
)

type ErrorInput struct {
	middleware.Input
	StatusCode int
	Message    string
	Error      error
	RequestID  xid.ID
}

func (ErrorInput) TemplateBundle() string {
	return "error"
}

var (
	ErrNotFound         = errors.New("page not found")
	ErrMethodNotAllowed = errors.New("method not allowed")
)

func ErrorHandler(exec templates.Executor, w http.ResponseWriter, r *http.Request, err error) {
	hlog.FromRequest(r).Error().Ctx(r.Context()).Err(err).Msg("")

	var statusCode = http.StatusInternalServerError
	var msg = http.StatusText(statusCode)

	switch {
	case errors.IsE(err, ErrNotFound):
		statusCode = http.StatusNotFound
		msg = "page not found"
	case errors.IsE(err, ErrMethodNotAllowed):
		statusCode = http.StatusMethodNotAllowed
		msg = "method not allowed"
	case errors.Is(errors.InvalidForm, err):
		statusCode = http.StatusBadRequest
		msg = "form is invalid"
	}

	rid, _ := hlog.IDFromRequest(r)

	input := ErrorInput{
		Input:      middleware.InputFromRequest(r),
		StatusCode: statusCode,
		Message:    msg,
		Error:      err,
		RequestID:  rid,
	}

	if util.IsHTMX(r) {
		// if this is a htmx request, we should retarget onto the content div
		w.Header().Set("HX-Retarget", "#content")
	} else {
		// if it's not htmx, we can send the status code as normal
		w.WriteHeader(statusCode)
	}

	err = exec.Execute(w, r, input)
	if err != nil {
		hlog.FromRequest(r).Error().Ctx(r.Context()).Err(err).Msg("error while rendering error page")
		http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
	}
}
