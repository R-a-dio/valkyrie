package mariadb

import (
	"database/sql"
	"time"

	radio "github.com/R-a-dio/valkyrie"
	"github.com/R-a-dio/valkyrie/errors"
	"github.com/jmoiron/sqlx"
	"github.com/rs/zerolog"
)

type StatusStorage struct {
	handle handle
}

// Store implements radio.StatusStorage
func (ss StatusStorage) Store(status radio.Status) error {
	const op errors.Op = "mariadb/StatusStorage.Store"
	handle, deferFn := ss.handle.span(op)
	defer deferFn()

	if status.IsZero() {
		return errors.E(op, errors.InvalidArgument)
	}

	// the named query below will try to access some fields that are beyond
	// a pointer type, so make sure those fields actually exist before we
	// pass it to the database driver
	if !status.Song.HasTrack() {
		status.Song.DatabaseTrack = &radio.DatabaseTrack{}
	}
	if status.StreamUser == nil {
		status.StreamUser = &radio.User{}
	}

	// we also have the info Start/End times that could be zero, if they are
	// they would be outside of the supported range of mariadb, so just mock
	// them to be the current time
	if status.SongInfo.Start.IsZero() {
		status.SongInfo.Start = time.Now()
	}
	if status.SongInfo.End.IsZero() {
		status.SongInfo.End = status.SongInfo.Start
	}

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
				djname,
				stream_user
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
				:streamername,
				:streamuser.id
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
				stream_user=:streamuser.id,
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
			djname AS streamername,
			COALESCE(stream_user, 0) AS 'streamuser.id'
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

	// since we don't store the full information we need to fill in the
	// rest of the information before we return the status, we could do
	// this with a join in the query, but since this is a low-frequency
	// method it should be fine to just do 3 extra queries
	if status.Song.Metadata != "" {
		song, err := SongStorage{handle}.FromMetadata(status.Song.Metadata)
		if err != nil {
			zerolog.Ctx(handle.ctx).Warn().
				Err(err).
				Msg("retrieving database metadata")
		} else {
			status.Song = *song
		}
	}
	if status.User.DJ.ID != 0 {
		user, err := UserStorage{handle}.GetByDJID(status.User.DJ.ID)
		if err != nil {
			zerolog.Ctx(handle.ctx).Warn().
				Err(err).
				Msg("retrieving database user")
		} else {
			status.User = *user
		}
	}
	if status.StreamUser != nil && status.StreamUser.ID != 0 {
		user, err := UserStorage{handle}.GetByID(status.StreamUser.ID)
		if err != nil {
			zerolog.Ctx(handle.ctx).Warn().
				Err(err).
				Msg("retrieving database stream user")
		} else {
			status.StreamUser = user
		}
	}

	return &status, nil
}
