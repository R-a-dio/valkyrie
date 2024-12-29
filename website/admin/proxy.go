package admin

import (
	"cmp"
	"html/template"
	"net/http"
	"slices"

	radio "github.com/R-a-dio/valkyrie"
	"github.com/R-a-dio/valkyrie/errors"
	"github.com/R-a-dio/valkyrie/website/middleware"
	"github.com/gorilla/csrf"
)

type ProxyInput struct {
	middleware.Input
	CSRFTokenInput template.HTML

	Sources map[string][]radio.ProxySource
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

	// generate a mapping of mountname to sources
	sm := make(map[string][]radio.ProxySource, 3)
	for _, source := range sources {
		sm[source.MountName] = append(sm[source.MountName], source)
	}

	// then sort the sources by their priority value
	for _, sources := range sm {
		slices.SortStableFunc(sources, func(a, b radio.ProxySource) int {
			return cmp.Compare(a.Priority, b.Priority)
		})
	}

	input := &ProxyInput{
		Input:          middleware.InputFromRequest(r),
		CSRFTokenInput: csrf.TemplateField(r),
		Sources:        sm,
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
