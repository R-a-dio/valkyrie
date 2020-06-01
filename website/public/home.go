package public

import (
	"log"
	"net/http"

	radio "github.com/R-a-dio/valkyrie"
	"github.com/R-a-dio/valkyrie/errors"
)

func (s State) GetHome(w http.ResponseWriter, r *http.Request) {
	err := s.getHome(w, r)
	if err != nil {
		log.Println(err)
	}
}

func (s State) getHome(w http.ResponseWriter, r *http.Request) error {
	const op errors.Op = "website/public.getHome"

	status, err := s.Manager.Status(r.Context())
	if err != nil {
		return errors.E(op, errors.InternalServer, err)
	}

	homeInput := struct {
		sharedInput

		Status *radio.Status
	}{
		Status: status,
	}

	err = s.Templates[theme]["home"].ExecuteDev(w, homeInput)
	if err != nil {
		return errors.E(op, errors.InternalServer, err)
	}

	return nil
}
