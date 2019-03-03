package mariadb

import (
	"database/sql"

	radio "github.com/R-a-dio/valkyrie"
	"github.com/R-a-dio/valkyrie/errors"
	"github.com/jmoiron/sqlx"
)

type StatusStorage struct {
	handle handle
}

// Store implements radio.StatusStorage
func (ss StatusStorage) Store(status radio.Status) error {
	const op errors.Op = "mariadb/StatusStorage.Store"

	// TODO(wessie): use INSERT INTO ... ON DUPLICATE UPDATE maybe?
	// we either want to do an UPDATE or an INSERT if our row doesn't exist
	var queries = []string{`
		UPDATE
			streamstatus
		SET
			djid=:user.dj.id,
			np=:song.metadata,
			listeners=:listeners,
			bitrate=192000,
			isafkstream=0,
			isstreamdesk=0,
			start_time=UNIX_TIMESTAMP(:songinfo.start),
			end_time=UNIX_TIMESTAMP(:songinfo.end),
			trackid=:song.trackid,
			thread=:thread,
			requesting=:requestsenabled,
			djname=:streamername,
			lastset=NOW()
		WHERE
			id=0;
		`, `
		INSERT INTO
			streamstatus
		(
			id,
			djid,
			np,
			listeners,
			isafkstream,
			start_time,
			end_time,
			trackid,
			thread,
			requesting,
			djname
		) VALUES (
			0,
			:user.dj.id,
			:song.metadata,
			:listeners,
			0,
			UNIX_TIMESTAMP(:songinfo.start),
			UNIX_TIMESTAMP(:songinfo.end),
			:song.trackid,
			:thread,
			:requestsenabled,
			:streamername
		);
	`}

	// now try the UPDATE and if that fails do the same with an INSERT
	for _, query := range queries {
		query, args, err := sqlx.Named(query, status)
		if err != nil {
			return errors.E(op, err)
		}

		res, err := ss.handle.Exec(query, args...)
		if err != nil {
			return errors.E(op, err)
		}

		// check if we've successfully updated, otherwise we need to do an insert
		if i, err := res.RowsAffected(); err != nil {
			return errors.E(op, err)
		} else if i > 0 { // success
			return nil
		}
	}

	return nil
}

// Load implements radio.StatusStorage
func (ss StatusStorage) Load() (*radio.Status, error) {
	const op errors.Op = "mariadb/StatusStorage.Load"

	var query = `
		SELECT
			djid AS 'user.dj.id',
			np AS 'song.metadata',
			listeners,
			from_unixtime(start_time) AS 'songinfo.start',
			from_unixtime(end_time) AS 'songinfo.end',
			trackid AS 'song.trackid',
			thread,
			requesting AS requestsenabled,
			djname AS 'streamername'
		FROM
			streamstatus
		WHERE
			id=0
		LIMIT 1;
	`

	var status radio.Status

	err := sqlx.Get(ss.handle, &status, query)
	if err != nil && err != sql.ErrNoRows {
		return nil, errors.E(op, err)
	}
	return &status, nil
}
