package shared

import (
	"html/template"
	"net/http"

	radio "github.com/R-a-dio/valkyrie"
	"github.com/R-a-dio/valkyrie/config"
	"github.com/R-a-dio/valkyrie/website/middleware"
)

type Input struct {
	IsUser    bool
	User      *radio.User
	StreamURL template.URL
}

type InputFactory struct {
	StreamURL template.URL
}

func (i *InputFactory) New(r *http.Request) Input {
	user := middleware.UserFromContext(r.Context())

	if i == nil {
		return Input{
			IsUser: user != nil,
			User:   user,
		}
	}

	return Input{
		IsUser:    user != nil,
		User:      user,
		StreamURL: i.StreamURL,
	}
}

func NewInputFactory(cfg config.Config) *InputFactory {
	return &InputFactory{
		StreamURL: template.URL(cfg.Conf().Website.PublicStreamURL),
	}
}

func (Input) TemplateName() string {
	return "full-page"
}
