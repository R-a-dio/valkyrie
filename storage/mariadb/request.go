package mariadb

import (
	"database/sql"
	"time"

	"github.com/R-a-dio/valkyrie/errors"
	"github.com/jmoiron/sqlx"
)

// RequestStorage implements radio.RequestStorage
type RequestStorage struct {
	handle handle
}

// LastRequest implements radio.RequestStorage
func (rs RequestStorage) LastRequest(identifier string) (time.Time, error) {
	const op errors.Op = "mariadb/RequestStorage.LastRequest"
	handle, deferFn := rs.handle.span(op)
	defer deferFn()

	var t time.Time

	query := "SELECT time FROM requesttime WHERE ip=? ORDER BY time DESC LIMIT 1;"
	//query := "SELECT time FROM requesttime WHERE identifier=? ORDER BY time DESC LIMIT 1;"

	err := sqlx.Get(handle, &t, query, identifier)
	if errors.IsE(err, sql.ErrNoRows) {
		err = nil
	}
	if err != nil {
		return t, errors.E(op, err)
	}

	return t, nil
}

// UpdateLastRequest implements radio.RequestStorage
func (rs RequestStorage) UpdateLastRequest(identifier string) error {
	const op errors.Op = "mariadb/RequestStorage.UpdateLastRequest"
	handle, deferFn := rs.handle.span(op)
	defer deferFn()

	query := "INSERT INTO requesttime (ip, time) VALUES (?, NOW());"
	//query := "INSERT INTO requesttime (identifier, time) VALUES (?, NOW())";

	_, err := handle.Exec(query, identifier)
	if err != nil {
		return errors.E(op, err)
	}
	return nil
}
