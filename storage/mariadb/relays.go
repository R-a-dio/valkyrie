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
	handle, deferFn := rs.handle.span(op)
	defer deferFn()

	var query = `UPDATE relays SET 
	status = :status,
	stream = :stream,
	online = :online,
	disabled = :disabled,
	noredir = :noredir,
	listeners = :listeners,
	err = :err,
	max = :max
	WHERE name = :name;`

	_, err := sqlx.NamedExec(handle, query, r)
	if err != nil {
		return errors.E(op, err)
	}

	return nil
}

// All implements radio.SessionStorage
func (rs RelayStorage) All() ([]radio.Relay, error) {
	const op errors.Op = "mariadb/RelayStorage.All"
	handle, deferFn := rs.handle.span(op)
	defer deferFn()

	var query = "SELECT * FROM relays;"

	relays := []radio.Relay{}

	err := sqlx.Select(handle, &relays, query)
	if err != nil {
		return relays, errors.E(op, err)
	}
	if len(relays) == 0 {
		return relays, errors.E(op, errors.NoRelays)
	}

	return relays, nil
}
