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
				requesting,
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
				:requestsenabled,
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
				requesting=:requestsenabled,
				djname=:streamername,
				lastset=NOW();
	`

	_, err := sqlx.NamedExec(ss.handle, query, status)
	if err != nil {
		return errors.E(op, err)
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
			IF(requesting = 1, 'true', 'false') AS requestsenabled,
			djname AS streamername
		FROM
			streamstatus
		WHERE
			id=1
		LIMIT 1;
	`

	var status radio.Status

	err := sqlx.Get(ss.handle, &status, query)
	if err != nil && err != sql.ErrNoRows {
		return nil, errors.E(op, err)
	}
	return &status, nil
}
