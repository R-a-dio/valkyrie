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

type SessionDeleteParams struct {
	Token radio.SessionToken
}

// input: SessionDeleteParams
const SessionDeleteQuery = `
DELETE FROM
	sessions
WHERE
	token=:token;
`

// Delete implements radio.SessionStorage
func (ss SessionStorage) Delete(token radio.SessionToken) error {
	const op errors.Op = "mariadb/SessionStorage.Delete"
	handle, deferFn := ss.handle.span(op)
	defer deferFn()

	_, err := sqlx.NamedExec(handle, SessionDeleteQuery, SessionDeleteParams{
		Token: token,
	})
	if err != nil {
		return errors.E(op, err)
	}

	return nil
}

// input: radio.Session
const SessionSaveQuery = `
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
		expiry=VALUE(expiry), data=VALUE(data);
`

// Save implements radio.SessionStorage
func (ss SessionStorage) Save(session radio.Session) error {
	const op errors.Op = "mariadb/SessionStorage.Save"
	handle, deferFn := ss.handle.span(op)
	defer deferFn()

	_, err := sqlx.NamedExec(handle, SessionSaveQuery, session)
	if err != nil {
		return errors.E(op, err)
	}
	return nil
}

type SessionGetParams struct {
	Token radio.SessionToken
}

// input: SessionGetParams
// output: radio.Session
const SessionGetQuery = `
SELECT
	token,
	expiry,
	data
FROM
	sessions
WHERE
	token=:token;
`

// Get implements radio.SessionStorage
func (ss SessionStorage) Get(token radio.SessionToken) (radio.Session, error) {
	const op errors.Op = "mariadb/SessionStorage.Get"
	handle, deferFn := ss.handle.span(op)
	defer deferFn()

	var session radio.Session

	err := handle.Get(&session, SessionGetQuery, SessionGetParams{
		Token: token,
	})
	if err != nil {
		if errors.IsE(err, sql.ErrNoRows) {
			return session, errors.E(op, errors.SessionUnknown)
		}
		return session, errors.E(op, err)
	}

	return session, nil
}
