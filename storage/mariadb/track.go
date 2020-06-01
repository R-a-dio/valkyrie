package mariadb

import (
	"database/sql"
	"time"

	radio "github.com/R-a-dio/valkyrie"
	"github.com/R-a-dio/valkyrie/errors"
	"github.com/go-sql-driver/mysql"
	"github.com/jmoiron/sqlx"
)

// FavePriorityIncrement is the amount we increase/decrease priority by
// on a track when it gets favorited/unfavorited
const FavePriorityIncrement = 1

// databaseTrack is the type used to communicate with the database
type databaseTrack struct {
	// Hash is shared between tracks and esong
	Hash radio.SongHash
	// LastPlayed is shared between tracks and eplay
	LastPlayed mysql.NullTime

	// esong fields
	ID       sql.NullInt64
	Length   sql.NullFloat64
	Metadata sql.NullString

	// tracks fields
	TrackID       sql.NullInt64
	Artist        sql.NullString
	Track         sql.NullString
	Album         sql.NullString
	Path          sql.NullString
	Tags          sql.NullString
	Priority      sql.NullInt64
	LastRequested mysql.NullTime
	Usable        sql.NullInt64
	Acceptor      sql.NullString
	LastEditor    sql.NullString

	RequestCount    sql.NullInt64
	NeedReplacement sql.NullInt64
}

func (dt databaseTrack) ToSong() radio.Song {
	var track *radio.DatabaseTrack
	if dt.TrackID.Valid {
		track = &radio.DatabaseTrack{
			TrackID:  radio.TrackID(dt.TrackID.Int64),
			Artist:   dt.Artist.String,
			Title:    dt.Track.String,
			Album:    dt.Album.String,
			FilePath: dt.Path.String,
			Tags:     dt.Tags.String,

			Acceptor:   dt.Acceptor.String,
			LastEditor: dt.LastEditor.String,

			Priority: int(dt.Priority.Int64),
			Usable:   dt.Usable.Int64 == 1,

			LastRequested: dt.LastRequested.Time,
			RequestCount:  int(dt.RequestCount.Int64),
			RequestDelay:  radio.CalculateRequestDelay(int(dt.RequestCount.Int64)),
		}
	}

	song := radio.Song{
		ID:            radio.SongID(dt.ID.Int64),
		Hash:          dt.Hash,
		Metadata:      dt.Metadata.String,
		Length:        time.Duration(float64(time.Second) * dt.Length.Float64),
		LastPlayed:    dt.LastPlayed.Time,
		DatabaseTrack: track,
		SyncTime:      time.Now(),
	}
	song.FillMetadata()
	return song
}

func (dt databaseTrack) ToSongPtr() *radio.Song {
	song := dt.ToSong()
	return &song
}

// SongStorage implements radio.SongStorage
type SongStorage struct {
	handle handle
}

// Create implements radio.SongStorage
func (ss SongStorage) Create(metadata string) (*radio.Song, error) {
	const op errors.Op = "mariadb/SongStorage.Create"

	var query = `INSERT INTO esong (meta, hash, hash_link, len) VALUES (?, ?, ?, ?)`
	hash := radio.NewSongHash(metadata)

	_, err := ss.handle.Exec(query, metadata, hash, hash, 0)
	if err != nil {
		return nil, errors.E(op, err)
	}

	song, err := ss.FromHash(hash)
	if err != nil {
		return nil, errors.E(op, err)
	}
	return song, nil
}

// FromMetadata implements radio.SongStorage
func (ss SongStorage) FromMetadata(metadata string) (*radio.Song, error) {
	const op errors.Op = "mariadb/SongStorage.FromMetadata"

	song, err := ss.FromHash(radio.NewSongHash(metadata))
	if err != nil {
		return nil, errors.E(op, err)
	}
	return song, nil
}

// FromHash implements radio.SongStorage
func (ss SongStorage) FromHash(hash radio.SongHash) (*radio.Song, error) {
	const op errors.Op = "mariadb/SongStorage.FromHash"

	var tmp databaseTrack

	var query = `
	SELECT tracks.id AS trackid, esong.id AS id, esong.hash AS hash, esong.meta AS metadata,
	len AS length, eplay.dt AS lastplayed, artist, track, album, path,
	tags, accepter AS acceptor, lasteditor, priority, usable, lastrequested,
	requestcount FROM tracks RIGHT JOIN esong ON tracks.hash = esong.hash LEFT JOIN eplay ON
	esong.id = eplay.isong WHERE esong.hash=? ORDER BY eplay.dt DESC LIMIT 1;`

	err := sqlx.Get(ss.handle, &tmp, query, hash)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, errors.E(op, errors.SongUnknown)
		}
		return nil, errors.E(op, err)
	}

	return tmp.ToSongPtr(), nil
}

// LastPlayed implements radio.SongStorage
func (ss SongStorage) LastPlayed(offset, amount int) ([]radio.Song, error) {
	const op errors.Op = "mariadb/SongStorage.LastPlayed"

	var query = `
	SELECT
		esong.id AS id,
		esong.hash AS hash,
		esong.meta AS metadata,
		esong.len AS length,
		tracks.id AS trackid,
		eplay.dt AS lastplayed,
		tracks.id AS trackid,
		tracks.artist,
		tracks.track,
		tracks.album,
		tracks.path,
		tracks.tags,
		tracks.accepter AS acceptor,
		tracks.lasteditor,
		tracks.priority,
		tracks.usable,
		tracks.lastrequested,
		tracks.requestcount
	FROM
		esong
	RIGHT JOIN
		eplay ON esong.id = eplay.isong
	LEFT JOIN
		tracks ON esong.hash = tracks.hash
	ORDER BY 
		eplay.dt DESC
	LIMIT ? OFFSET ?;`

	var tmps = make([]databaseTrack, 0, amount)

	err := sqlx.Select(ss.handle, &tmps, query, amount, offset)
	if err != nil {
		return nil, errors.E(op, err)
	}

	var songs = make([]radio.Song, len(tmps))
	for i, tmp := range tmps {
		songs[i] = tmp.ToSong()
	}

	return songs, nil
}

// PlayedCount implements radio.SongStorage
func (ss SongStorage) PlayedCount(song radio.Song) (int64, error) {
	const op errors.Op = "mariadb/SongStorage.PlayedCount"

	var query = `SELECT count(*) FROM eplay WHERE isong=?;`
	var playedCount int64

	err := sqlx.Get(ss.handle, &playedCount, query, song.ID)
	if err != nil {
		return 0, errors.E(op, err)
	}
	return playedCount, nil
}

// AddPlay implements radio.SongStorage
func (ss SongStorage) AddPlay(song radio.Song, ldiff *int) error {
	const op errors.Op = "mariadb/SongStorage.AddPlay"

	var query = `INSERT INTO eplay (isong, ldiff) VALUES (?, ?);`

	_, err := ss.handle.Exec(query, song.ID, ldiff)
	if err != nil {
		return errors.E(op, err)
	}
	return nil
}

// FavoriteCount implements radio.SongStorage
func (ss SongStorage) FavoriteCount(song radio.Song) (int64, error) {
	const op errors.Op = "mariadb/SongStorage.FavoriteCount"

	var query = `SELECT count(*) FROM efave WHERE isong=?;`
	var faveCount int64

	err := sqlx.Get(ss.handle, &faveCount, query, song.ID)
	if err != nil {
		return 0, errors.E(op, err)
	}
	return faveCount, nil
}

// Favorites implements radio.SongStorage
func (ss SongStorage) Favorites(song radio.Song) ([]string, error) {
	const op errors.Op = "mariadb/SongStorage.Favorites"

	var query = `SELECT enick.nick FROM efave JOIN enick ON
	enick.id = efave.inick WHERE efave.isong=?`

	var users []string

	err := sqlx.Select(ss.handle, &users, query, song.ID)
	if err != nil {
		return nil, errors.E(op, err)
	}

	return users, nil
}

// FavoritesOf implements radio.SongStorage
func (ss SongStorage) FavoritesOf(nick string) ([]radio.Song, error) {
	const op errors.Op = "mariadb/SongStorage.FavoritesOf"

	var query = `
	SELECT
		esong.id AS id,
		esong.hash AS hash,
		esong.meta AS metadata,
		esong.len AS length,
		tracks.id AS trackid,
		tracks.lastplayed,
		tracks.artist,
		tracks.track,
		tracks.album,
		tracks.path,
		tracks.tags,
		tracks.accepter AS acceptor,
		tracks.lasteditor,
		tracks.priority,
		tracks.usable,
		tracks.lastrequested,
		tracks.requestcount
	FROM
		tracks
	LEFT JOIN
		esong ON tracks.hash = esong.hash
	JOIN
		efave ON efave.isong = esong.id
	JOIN
		enick ON efave.inick = enick.id
	WHERE
		tracks.usable=1 AND
		enick.nick=?;
	`

	var tmps = []databaseTrack{}

	err := sqlx.Select(ss.handle, &tmps, query, nick)
	if err != nil {
		return nil, errors.E(op, err)
	}

	var songs = make([]radio.Song, len(tmps))
	for i, tmp := range tmps {
		songs[i] = tmp.ToSong()
	}

	return songs, nil
}

// AddFavorite implements radio.SongStorage
func (ss SongStorage) AddFavorite(song radio.Song, nick string) (bool, error) {
	const op errors.Op = "mariadb/SongStorage.AddFavorite"

	var query = `SELECT enick.id AS id, EXISTS(SELECT efave.id FROM efave WHERE
		inick=enick.id AND isong=?) AS hasfave FROM enick WHERE enick.nick=?;`

	var info = struct {
		ID      int64
		HasFave bool
	}{}

	err := sqlx.Get(ss.handle, &info, query, song.ID, nick)
	if err != nil {
		return false, errors.E(op, err)
	}

	if info.HasFave {
		return false, nil
	}

	// TODO(wessie): use a transaction here
	if info.ID == 0 {
		query = `INSERT INTO enick (nick) VALUES (?)`
		res, err := ss.handle.Exec(query, nick)
		if err != nil {
			return false, errors.E(op, err)
		}

		info.ID, err = res.LastInsertId()
		if err != nil {
			panic("LastInsertId not supported")
		}
	}

	query = `INSERT INTO efave (inick, isong) VALUES (?, ?)`
	_, err = ss.handle.Exec(query, info.ID, song.ID)
	if err != nil {
		return false, errors.E(op, err)
	}

	// we increase a search priority when a song gets favorited
	if song.DatabaseTrack != nil && song.TrackID != 0 {
		query = `UPDATE tracks SET priority=priority+? WHERE id=?`
		_, err = ss.handle.Exec(query, FavePriorityIncrement, song.TrackID)
		if err != nil {
			return false, errors.E(op, err)
		}
	}

	return true, nil
}

// RemoveFavorite implements radio.SongStorage
func (ss SongStorage) RemoveFavorite(song radio.Song, nick string) (bool, error) {
	const op errors.Op = "mariadb/SongStorage.RemoveFavorite"

	var query = `DELETE efave FROM efave JOIN enick ON
	enick.id = efave.inick WHERE enick.nick=? AND efave.isong=?;`

	res, err := ss.handle.Exec(query, nick, song.ID)
	if err != nil {
		return false, errors.E(op, err)
	}

	n, err := res.RowsAffected()
	if err != nil {
		panic("RowsAffected not supported")
	}

	// we decrease a search priority when a song gets unfavorited
	if n > 0 && song.DatabaseTrack != nil && song.TrackID != 0 {
		query = `UPDATE tracks SET priority=priority-? WHERE id=?`
		_, err = ss.handle.Exec(query, FavePriorityIncrement, song.TrackID)
		if err != nil {
			return false, errors.E(op, err)
		}
	}
	return n > 0, nil
}

// UpdateLength implements radio.SongStorage
func (ss SongStorage) UpdateLength(song radio.Song, length time.Duration) error {
	const op errors.Op = "mariadb/SongStorage.UpdateLength"

	var query = "UPDATE esong SET len=? WHERE id=?;"

	len := int(length / time.Second)
	_, err := ss.handle.Exec(query, len, song.ID)
	if err != nil {
		return errors.E(op, err)
	}
	return nil
}

// TrackStorage implements radio.TrackStorage
type TrackStorage struct {
	handle handle
}

// Get implements radio.TrackStorage
func (ts TrackStorage) Get(id radio.TrackID) (*radio.Song, error) {
	const op errors.Op = "mariadb/TrackStorage.Get"

	// we create a temporary struct to handle NULL values returned by
	// the query, both Length and Song.ID can be NULL due to the LEFT JOIN
	// not necessarily having an entry in the `esong` table.
	// Song.ID is handled by the SongID type implementing sql.Scanner, but
	// we don't want a separate type for Length, so we're doing it separately.
	var tmp databaseTrack

	var query = `
	SELECT tracks.id AS trackid, esong.id AS id, tracks.hash AS hash,
	len AS length, lastplayed, artist, track, album, path,
	tags, accepter AS acceptor, lasteditor, priority, usable, lastrequested,
	requestcount FROM tracks LEFT JOIN esong ON tracks.hash = esong.hash WHERE 
	tracks.id=?;`

	err := sqlx.Get(ts.handle, &tmp, query, id)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, errors.E(op, errors.SongUnknown)
		}
		return nil, errors.E(op, err)
	}

	return tmp.ToSongPtr(), nil
}

// All implements radio.TrackStorage
func (ts TrackStorage) All() ([]radio.Song, error) {
	const op errors.Op = "mariadb/TrackStorage.All"

	var tmps = []databaseTrack{}

	var query = `
	SELECT tracks.id AS trackid, esong.id AS id, tracks.hash AS hash,
	len AS length, lastplayed, artist, track, album, path,
	tags, accepter AS acceptor, lasteditor, priority, usable, lastrequested,
	requestcount FROM tracks LEFT JOIN esong ON tracks.hash = esong.hash;`

	err := sqlx.Select(ts.handle, &tmps, query)
	if err != nil {
		return nil, errors.E(op, err)
	}

	var tracks = make([]radio.Song, len(tmps))
	for i, tmp := range tmps {
		tracks[i] = tmp.ToSong()
	}

	return tracks, nil
}

// Unusable implements radio.TrackStorage
func (ts TrackStorage) Unusable() ([]radio.Song, error) {
	const op errors.Op = "mariadb/TrackStorage.Unusable"

	var tmps = []databaseTrack{}

	var query = `
	SELECT 
		esong.id AS id,
		esong.hash AS hash,
		esong.meta AS metadata,
		esong.len AS length,
		tracks.id AS trackid,
		tracks.lastplayed,
		tracks.artist,
		tracks.track,
		tracks.album,
		tracks.path,
		tracks.tags,
		tracks.accepter AS acceptor,
		tracks.lasteditor,
		tracks.priority,
		tracks.usable,
		tracks.lastrequested,
		tracks.requestcount
	FROM
		tracks
	LEFT JOIN 
		esong
	ON
		tracks.hash = esong.hash
	WHERE
		tracks.usable != 1;
	`

	err := sqlx.Select(ts.handle, &tmps, query)
	if err != nil {
		return nil, errors.E(op, err)
	}

	var tracks = make([]radio.Song, len(tmps))
	for i, tmp := range tmps {
		tracks[i] = tmp.ToSong()
	}

	return tracks, nil
}

func (ts TrackStorage) UpdateUsable(song radio.Song, state int) error {
	const op errors.Op = "mariadb/TrackStorage.UpdateUsable"

	var query = `
	UPDATE
		tracks
	SET
		usable=?
	WHERE
		id=?;
	`

	_, err := ts.handle.Exec(query, state, song.ID)
	if err != nil {
		return errors.E(op, err)
	}
	return nil
}

// UpdateRequestInfo updates the time the track given was last requested
// and increases the time between requests for the song.
//
// implements radio.TrackStorage
func (ts TrackStorage) UpdateRequestInfo(id radio.TrackID) error {
	const op errors.Op = "mariadb/TrackStorage.UpdateRequestInfo"

	// TODO(wessie): don't hardcode requestcount and priority
	var query = `UPDATE tracks SET lastrequested=NOW(),
	requestcount=requestcount+2, priority=priority+1 WHERE id=?;`

	_, err := ts.handle.Exec(query, id)
	if err != nil {
		return errors.E(op, err)
	}
	return nil
}

// UpdateLastPlayed implements radio.TrackStorage
func (ts TrackStorage) UpdateLastPlayed(id radio.TrackID) error {
	const op errors.Op = "mariadb/TrackStorage.UpdateLastPlayed"

	var query = `UPDATE tracks SET lastplayed=NOW() WHERE id=?;`

	_, err := ts.handle.Exec(query, id)
	if err != nil {
		return errors.E(op, err)
	}
	return nil
}

// UpdateLastRequested implements radio.TrackStorage
func (ts TrackStorage) UpdateLastRequested(id radio.TrackID) error {
	const op errors.Op = "mariadb/TrackStorage.UpdateLastRequested"

	var query = `
	UPDATE
		tracks
	SET 
		lastrequested=NOW()
	WHERE
		id=?;
	`

	_, err := ts.handle.Exec(query, id)
	if err != nil {
		return errors.E(op, err)
	}
	return nil
}

// BeforeLastRequested implements radio.TrackStorage
func (ts TrackStorage) BeforeLastRequested(before time.Time) ([]radio.Song, error) {
	const op errors.Op = "mariadb/TrackStorage.BeforeLastRequested"

	var query = `
		SELECT
			esong.id AS id,
			esong.hash AS hash,
			esong.meta AS metadata,
			esong.len AS length,
			tracks.id AS trackid,
			tracks.lastplayed,
			tracks.artist,
			tracks.track,
			tracks.album,
			tracks.path,
			tracks.tags,
			tracks.accepter AS acceptor,
			tracks.lasteditor,
			tracks.priority,
			tracks.usable,
			tracks.lastrequested,
			tracks.requestcount
		FROM
			tracks
		LEFT JOIN
			esong ON tracks.hash = esong.hash
		WHERE
			lastrequested < ?
		AND
			requestcount > 0;
	`

	var tmps = []databaseTrack{}

	err := sqlx.Select(ts.handle, &tmps, query, before)
	if err != nil {
		return nil, errors.E(op, err)
	}

	var songs = make([]radio.Song, len(tmps))
	for i, tmp := range tmps {
		songs[i] = tmp.ToSong()
	}

	return songs, nil
}

// DecrementRequestCount implements radio.TrackStorage
func (ts TrackStorage) DecrementRequestCount(before time.Time) error {
	const op errors.Op = "mariadb/TrackStorage.DecrementRequestCount"

	var query = `
		UPDATE
			tracks
		SET
			requestcount=requestcount - 1
		WHERE
			lastrequested < ?
		AND
			requestcount > 0;
	`

	_, err := ts.handle.Exec(query, before)
	if err != nil {
		return errors.E(op, err)
	}
	return nil
}

// QueueCandidates implements radio.TrackStorage
//
// Candidates are returned based on their lastplayed and lastrequested values
func (ts TrackStorage) QueueCandidates() ([]radio.TrackID, error) {
	const op errors.Op = "mariadb/TrackStorage.QueueCandidates"

	var query = `
		SELECT
			id
		FROM
			tracks
		WHERE
			usable=1
		ORDER BY (
			UNIX_TIMESTAMP(lastplayed) + 1)*(UNIX_TIMESTAMP(lastrequested) + 1)
		ASC LIMIT 100;
	`
	var candidates = []radio.TrackID{}
	err := sqlx.Select(ts.handle, &candidates, query)
	if err != nil {
		return nil, errors.E(op, err)
	}

	return candidates, nil
}
