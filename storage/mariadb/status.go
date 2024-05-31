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
	handle, deferFn := ss.handle.span(op)
	defer deferFn()

	var query = `
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
				djname
			) VALUES (
				1,
				:user.dj.id,
				:song.metadata,
				:listeners,
				0,
				UNIX_TIMESTAMP(:songinfo.start),
				UNIX_TIMESTAMP(:songinfo.end),
				:song.trackid,
				:thread,
				:streamername
			) ON DUPLICATE KEY UPDATE 
				djid=:user.dj.id,
				np=:song.metadata,
				listeners=:listeners,
				isafkstream=0,
				start_time=UNIX_TIMESTAMP(:songinfo.start),
				end_time=UNIX_TIMESTAMP(:songinfo.end),
				trackid=:song.trackid,
				thread=:thread,
				djname=:streamername,
				lastset=NOW();
	`

	_, err := sqlx.NamedExec(handle, query, status)
	if err != nil {
		return errors.E(op, err)
	}
	return nil
}

// Load implements radio.StatusStorage
func (ss StatusStorage) Load() (*radio.Status, error) {
	const op errors.Op = "mariadb/StatusStorage.Load"
	handle, deferFn := ss.handle.span(op)
	defer deferFn()

	var query = `
		SELECT
			djid AS 'user.dj.id',
			np AS 'song.metadata',
			listeners,
			from_unixtime(start_time) AS 'songinfo.start',
			from_unixtime(end_time) AS 'songinfo.end',
			trackid AS 'song.trackid',
			thread,
			djname AS streamername
		FROM
			streamstatus
		WHERE
			id=1
		LIMIT 1;
	`

	var status radio.Status

	err := sqlx.Get(handle, &status, query)
	if err != nil && err != sql.ErrNoRows {
		return nil, errors.E(op, err)
	}
	return &status, nil
}
