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

type BoothInput struct {
	middleware.Input
	CSRFTokenInput template.HTML

	ProxyStatus *radio.ProxySource
	IsLive      bool
}

func (BoothInput) TemplateBundle() string {
	return "booth"
}

func NewBoothInput(ps radio.ProxyService, r *http.Request) (*BoothInput, error) {
	const op errors.Op = "website/admin.NewBoothInput"

	sources, err := ps.ListSources(r.Context())
	if err != nil {
		return nil, errors.E(op, err)
	}

	// setup the input here, we need some of the fields it populates
	input := &BoothInput{
		Input:          middleware.InputFromRequest(r),
		CSRFTokenInput: csrf.TemplateField(r),
	}

	sm := proxySourceListToMap(sources)
	// find the first source that we match the user ID with
	// TODO: support multiple connections(?)
	for _, source := range sources {
		if source.User.ID == input.User.ID {
			if sm[source.MountName][0].ID == source.ID {
				input.IsLive = true
			}
			input.ProxyStatus = &source
			break
		}
	}

	return input, nil
}

func (s *State) GetBooth(w http.ResponseWriter, r *http.Request) {
	input, err := NewBoothInput(s.Proxy, r)
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

func proxySourceListToMap(sources []radio.ProxySource) map[string][]radio.ProxySource {
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
	return sm
}
