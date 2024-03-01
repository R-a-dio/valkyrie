package mariadb

import (
	"database/sql"

	radio "github.com/R-a-dio/valkyrie"
	"github.com/R-a-dio/valkyrie/errors"
	"github.com/jmoiron/sqlx"
)

// SessionStorage implements radio.SessionStorage
type SessionStorage struct {
	handle handle
}

// Delete implements radio.SessionStorage
func (ss SessionStorage) Delete(token radio.SessionToken) error {
	const op errors.Op = "mariadb/SessionStorage.Delete"
	handle, deferFn := ss.handle.span(op)
	defer deferFn()

	var query = `
	DELETE FROM 
		sessions
	WHERE
		token=?;
	`

	_, err := handle.Exec(query, token)
	if err != nil {
		return errors.E(op, err)
	}

	return nil
}

// Save implements radio.SessionStorage
func (ss SessionStorage) Save(session radio.Session) error {
	const op errors.Op = "mariadb/SessionStorage.Save"
	handle, deferFn := ss.handle.span(op)
	defer deferFn()

	var query = `
	INSERT INTO
		sessions (
			token,
			expiry,
			data
		) VALUES (
			:token,
			:expiry,
			:data
		) ON DUPLICATE KEY UPDATE
			expiry=:expiry, data=:data;
	`

	_, err := sqlx.NamedExec(handle, query, session)
	if err != nil {
		return errors.E(op, err)
	}
	return nil
}

// Get implements radio.SessionStorage
func (ss SessionStorage) Get(token radio.SessionToken) (radio.Session, error) {
	const op errors.Op = "mariadb/SessionStorage.Get"
	handle, deferFn := ss.handle.span(op)
	defer deferFn()

	var query = `
	SELECT
		token,
		expiry,
		data
	FROM
		sessions
	WHERE
		token=?;
	`

	var session radio.Session

	err := sqlx.Get(handle, &session, query, token)
	if err != nil {
		if err == sql.ErrNoRows {
			return session, errors.E(op, errors.SessionUnknown)
		}
		return session, errors.E(op, err)
	}

	return session, nil
}
