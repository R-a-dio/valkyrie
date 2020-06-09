package mariadb

import (
	radio "github.com/R-a-dio/valkyrie"
	"github.com/R-a-dio/valkyrie/errors"
	"github.com/jmoiron/sqlx"
)

// RelayStorage implements radio.RelayStorage
type RelayStorage struct {
	handle handle
}

// Update implements radio.RelayStorage
func (rs RelayStorage) Update(r radio.Relay) error {
	const op errors.Op = "mariadb/RelayStorage.Update"
	var query string

	_, err := rs.handle.Exec(query, r)
	if err != nil {
		return errors.E(op, err)
	}

	return nil
}

// All implements radio.SessionStorage
func (rs RelayStorage) All() ([]radio.Relay, error) {
	const op errors.Op = "mariadb/RelayStorage.All"

	var query = "SELECT * FROM relays;"

	relays := []radio.Relay{}

	err := sqlx.Select(rs.handle, &relays, query)
	if err != nil {
		return relays, errors.E(op, err)
	}
	/* if len(relays) == 0 {
		return relays, errors.E(op, errors.NoRelays)
	} */

	return relays, nil
}
