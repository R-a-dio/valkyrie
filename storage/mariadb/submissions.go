package mariadb

import (
	"database/sql"
	"time"

	"github.com/R-a-dio/valkyrie/errors"
	"github.com/jmoiron/sqlx"
)

// SubmissionStorage implements radio.SubmissionStorage
type SubmissionStorage struct {
	handle handle
}

// LastSubmissionTime implements radio.SubmissionStorage
func (ss SubmissionStorage) LastSubmissionTime(identifier string) (time.Time, error) {
	const op errors.Op = "mariadb/SubmissionStorage.LastSubmissionTime"

	var t time.Time

	query := "SELECT time FROM uploadtime WHERE ip=? ORDER BY time DESC LIMIT 1;"

	err := sqlx.Get(ss.handle, &t, query, identifier)
	if err == sql.ErrNoRows { // no rows means never uploaded, so it's OK
		err = nil
	}
	if err != nil {
		return t, errors.E(op, err)
	}

	return t, nil
}

// UpdateSubmissionTime implements radio.SubmissionStorage
func (ss SubmissionStorage) UpdateSubmissionTime(identifier string) error {
	const op errors.Op = "mariadb/SubmissionStorage.UpdateSubmissionTime"

	//query := "INSERT INTO uploadtime (ip, time) VALUES (?, NOW());"
	query := `
	INSERT INTO
		uploadtime (ip, time)
	VALUES
		(?, NOW())
	ON DUPLICATE KEY UPDATE
		time = NOW();
	`

	_, err := ss.handle.Exec(query, identifier)
	if err != nil {
		return errors.E(op, err)
	}
	return nil
}
