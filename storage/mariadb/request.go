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

const RequestLastRequestQuery = `
SELECT
	time
FROM
	requesttime
WHERE
	ip=:identifier
ORDER BY time DESC
LIMIT 1;
`

var _ = CheckQuery[RequestLastRequestParams](RequestLastRequestQuery)

type RequestLastRequestParams struct {
	Identifier string
}

// LastRequest implements radio.RequestStorage
func (rs RequestStorage) LastRequest(identifier string) (time.Time, error) {
	const op errors.Op = "mariadb/RequestStorage.LastRequest"
	handle, deferFn := rs.handle.span(op)
	defer deferFn()

	var t time.Time

	err := handle.Get(&t, RequestLastRequestQuery, RequestLastRequestParams{identifier})
	if errors.IsE(err, sql.ErrNoRows) {
		err = nil
	}
	if err != nil {
		return t, errors.E(op, err)
	}

	return t, nil
}

const RequestUpdateLastRequest = `
INSERT INTO
	requesttime (ip, time)
VALUES
	(:identifier, NOW());
`

var _ = CheckQuery[RequestUpdateLastRequestParams](RequestUpdateLastRequest)

type RequestUpdateLastRequestParams struct {
	Identifier string
}

// UpdateLastRequest implements radio.RequestStorage
func (rs RequestStorage) UpdateLastRequest(identifier string) error {
	const op errors.Op = "mariadb/RequestStorage.UpdateLastRequest"
	handle, deferFn := rs.handle.span(op)
	defer deferFn()

	_, err := sqlx.NamedExec(handle, RequestUpdateLastRequest, RequestUpdateLastRequestParams{identifier})
	if err != nil {
		return errors.E(op, err)
	}
	return nil
}
