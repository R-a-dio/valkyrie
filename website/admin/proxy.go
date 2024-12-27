package admin

import (
	"html/template"
	"net/http"

	radio "github.com/R-a-dio/valkyrie"
	"github.com/R-a-dio/valkyrie/errors"
	"github.com/R-a-dio/valkyrie/website/middleware"
	"github.com/gorilla/csrf"
)

type ProxyInput struct {
	middleware.Input
	CSRFTokenInput template.HTML

	Sources []radio.ProxySource
}

func (ProxyInput) TemplateBundle() string {
	return "proxy"
}

func NewProxyInput(ps radio.ProxyService, r *http.Request) (*ProxyInput, error) {
	const op errors.Op = "website/admin.NewProxyInput"

	sources, err := ps.ListSources(r.Context())
	if err != nil {
		return nil, errors.E(op, err)
	}

	input := &ProxyInput{
		Input:          middleware.InputFromRequest(r),
		CSRFTokenInput: csrf.TemplateField(r),
		Sources:        sources,
	}
	return input, nil
}

func (s *State) GetProxy(w http.ResponseWriter, r *http.Request) {
	input, err := NewProxyInput(s.Proxy, r)
	if err != nil {
		s.errorHandler(w, r, err, "")
		return
	}

	err = s.TemplateExecutor.Execute(w, r, input)
	if err != nil {
		s.errorHandler(w, r, err, "")
		return
	}
}

func (s *State) PostRemoveSource(w http.ResponseWriter, r *http.Request) {
	id, err := radio.ParseSourceID(r.FormValue("id"))
	if err != nil {
		s.errorHandler(w, r, err, "")
		return
	}

	err = s.Proxy.KickSource(r.Context(), id)
	if err != nil {
		s.errorHandler(w, r, err, "")
		return
	}

	s.GetProxy(w, r)
}
