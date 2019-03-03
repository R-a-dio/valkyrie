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

	var t time.Time

	query := "SELECT time FROM requesttime WHERE ip=? LIMIT 1;"
	//query := "SELECT time FROM requesttime WHERE identifier=? LIMIT 1;"

	err := sqlx.Get(rs.handle, &t, query, identifier)
	if err == sql.ErrNoRows {
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

	query := "INSERT INTO requesttime (ip, time) VALUES (?, NOW()) ON DUPLICATE KEY UPDATE time=NOW();"
	//query := "INSERT INTO requesttime (identifier, time) VALUES (?, NOW()) ON DUPLICATE KEY UPDATE time=NOW();"

	_, err := rs.handle.Exec(query, identifier)
	if err != nil {
		return errors.E(op, err)
	}
	return nil
}
