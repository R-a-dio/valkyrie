package mariadb

import (
	"database/sql"
	"strings"
	"time"

	radio "github.com/R-a-dio/valkyrie"
	"github.com/R-a-dio/valkyrie/errors"
	"github.com/go-sql-driver/mysql"
	"github.com/jmoiron/sqlx"
)

// FavePriorityIncrement is the amount we increase/decrease priority by
// on a track when it gets favorited/unfavorited
const FavePriorityIncrement = 1

func expand(query string) string {
	var orig = query
	query = strings.ReplaceAll(query, "{trackColumns}", trackColumns)
	query = strings.ReplaceAll(query, "{maybeTrackColumns}", maybeTrackColumns)
	query = strings.ReplaceAll(query, "{songColumns}", songColumns)
	query = strings.ReplaceAll(query, "{maybeSongColumns}", maybeSongColumns)
	query = strings.ReplaceAll(query, "{lastplayedSelect}", lastplayedSelect)
	if orig == query {
		panic("expand called but nothing was expanded")
	}
	return query
}

const trackColumns = `
	tracks.id AS trackid,
	IFNULL(tracks.artist, '') AS artist,
	IFNULL(tracks.track, '') AS title,
	IFNULL(tracks.album, '') AS album,
	IFNULL(tracks.path, '') AS filepath,
	IFNULL(tracks.tags, '') AS tags,
	tracks.accepter AS acceptor,
	tracks.lasteditor,
	tracks.priority,
	IF(tracks.usable, TRUE, FALSE) AS usable,
	IF(tracks.need_reupload, TRUE, FALSE) AS needreplacement,
	tracks.lastrequested,
	tracks.requestcount
`

const maybeTrackColumns = `
	IFNULL(tracks.id, 0) AS trackid,
	IFNULL(tracks.artist, '') AS artist,
	IFNULL(tracks.track, '') AS title,
	IFNULL(tracks.album, '') AS album,
	IFNULL(tracks.path, '') AS filepath,
	IFNULL(tracks.tags, '') AS tags,
	IFNULL(tracks.accepter, '') AS acceptor,
	IFNULL(tracks.lasteditor, '') AS lasteditor,
	IFNULL(tracks.priority, 0) AS priority,
	IF(tracks.usable, TRUE, FALSE) AS usable,
	IF(tracks.need_reupload, TRUE, FALSE) AS needreplacement,
	IFNULL(tracks.lastrequested, TIMESTAMP('0000-00-00 00:00:00')) AS lastrequested,
	IFNULL(tracks.requestcount, 0) AS requestcount
`

const songColumns = `
	esong.id AS id,
	esong.meta AS metadata,
	esong.hash AS hash,
	to_go_duration(esong.len) AS length
`

const maybeSongColumns = `
	IFNULL(esong.id, 0) AS id,
	IFNULL(esong.meta, '') AS metadata,
	IFNULL(esong.hash, '') AS hash,
	IFNULL(to_go_duration(esong.len), 0) AS length
`

const lastplayedSelect = `
	IFNULL((SELECT dt FROM eplay WHERE eplay.isong = esong.id ORDER BY dt DESC LIMIT 1), TIMESTAMP('0000-00-00 00:00:00')) AS lastplayed
`

// SongStorage implements radio.SongStorage
type SongStorage struct {
	handle handle
}

const songCreateQuery = `
INSERT INTO
	esong (
		meta,
		hash,
		hash_link,
		len
	) VALUES (
		:metadata,
		:hash,
		:hash,
		from_go_duration(:length)
	)
`

// Create implements radio.SongStorage
func (ss SongStorage) Create(song radio.Song) (*radio.Song, error) {
	const op errors.Op = "mariadb/SongStorage.Create"

	// TODO: see if we want to not use hydrate here and leave it up to the caller instead
	song.Hydrate()

	_, err := sqlx.NamedExec(ss.handle, songCreateQuery, song)
	if err != nil && !IsDuplicateKeyErr(err) {
		return nil, errors.E(op, err)
	}

	new, err := ss.FromHash(song.Hash)
	if err != nil {
		return nil, errors.E(op, err)
	}
	return new, nil
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

var songFromHashQuery = expand(`
SELECT
	{maybeTrackColumns},
	{songColumns},
	{lastplayedSelect},
	NOW() AS synctime
FROM
	tracks
RIGHT JOIN
	esong ON tracks.hash=esong.hash
WHERE
	esong.hash=?
LIMIT 1;
`)

// FromHash implements radio.SongStorage
func (ss SongStorage) FromHash(hash radio.SongHash) (*radio.Song, error) {
	const op errors.Op = "mariadb/SongStorage.FromHash"

	var song radio.Song

	err := sqlx.Get(ss.handle, &song, songFromHashQuery, hash)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, errors.E(op, errors.SongUnknown)
		}
		return nil, errors.E(op, err)
	}

	return &song, nil
}

var songLastPlayedQuery = expand(`
SELECT
	{maybeTrackColumns},
	{songColumns},
	eplay.dt AS lastplayed,
	NOW() AS synctime
FROM
	esong
RIGHT JOIN
	eplay ON esong.id=eplay.isong
LEFT JOIN
	tracks ON esong.hash=tracks.hash
ORDER BY
	eplay.dt DESC
LIMIT ? OFFSET ?;
`)

// LastPlayed implements radio.SongStorage
func (ss SongStorage) LastPlayed(offset, amount int) ([]radio.Song, error) {
	const op errors.Op = "mariadb/SongStorage.LastPlayed"

	var songs = make([]radio.Song, 0, amount)

	err := sqlx.Select(ss.handle, &songs, songLastPlayedQuery, amount, offset)
	if err != nil {
		return nil, errors.E(op, err)
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

var songFavoritesOfQuery = expand(`
SELECT
	{songColumns},
	{trackColumns},
	{lastplayedSelect},
	NOW() AS synctime
FROM
	tracks
LEFT JOIN
	esong ON tracks.hash = esong.hash
JOIN
	efave ON efave.isong = esong.id
JOIN
	enick ON efave.inick = enick.id
WHERE
	tracks.usable = 1
AND
	enick.nick = ?;
`)

// FavoritesOf implements radio.SongStorage
func (ss SongStorage) FavoritesOf(nick string) ([]radio.Song, error) {
	const op errors.Op = "mariadb/SongStorage.FavoritesOf"

	var songs = []radio.Song{}

	err := sqlx.Select(ss.handle, &songs, songFavoritesOfQuery, nick)
	if err != nil {
		return nil, errors.E(op, err)
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

var trackGetQuery = expand(`
SELECT
	{trackColumns},
	{maybeSongColumns},
	{lastplayedSelect},
	NOW() AS synctime
FROM
	tracks
LEFT JOIN
	esong ON tracks.hash = esong.hash
WHERE
	tracks.id = ?;
`)

// Get implements radio.TrackStorage
func (ts TrackStorage) Get(id radio.TrackID) (*radio.Song, error) {
	const op errors.Op = "mariadb/TrackStorage.Get"

	var song radio.Song

	err := sqlx.Get(ts.handle, &song, trackGetQuery, id)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, errors.E(op, errors.SongUnknown)
		}
		return nil, errors.E(op, err)
	}

	return &song, nil
}

var trackAllQuery = expand(`
SELECT
	{trackColumns},
	{maybeSongColumns},
	{lastplayedSelect},
	NOW() AS synctime
FROM
	tracks
LEFT JOIN
	esong ON tracks.hash = esong.hash
JOIN
	eplay ON eplay.isong = esong.id;
`)

// All implements radio.TrackStorage
func (ts TrackStorage) All() ([]radio.Song, error) {
	const op errors.Op = "mariadb/TrackStorage.All"

	var songs = []radio.Song{}

	err := sqlx.Select(ts.handle, &songs, trackAllQuery)
	if err != nil {
		return nil, errors.E(op, err)
	}

	return songs, nil
}

var trackUnusableQuery = expand(`
SELECT
	{maybeSongColumns},
	{trackColumns},
	{lastplayedSelect},
	NOW() as synctime
FROM
	tracks
LEFT JOIN
	esong ON tracks.hash = esong.hash
WHERE
	tracks.usable != 1;
`)

// Unusable implements radio.TrackStorage
func (ts TrackStorage) Unusable() ([]radio.Song, error) {
	const op errors.Op = "mariadb/TrackStorage.Unusable"

	var songs = []radio.Song{}

	err := sqlx.Select(ts.handle, &songs, trackUnusableQuery)
	if err != nil {
		return nil, errors.E(op, err)
	}

	return songs, nil
}

func IsDuplicateKeyErr(err error) bool {
	var mysqlError *mysql.MySQLError
	if !errors.As(err, &mysqlError) {
		return false
	}
	return mysqlError.Number == 1062
}

const trackUpdateQuery = `
UPDATE tracks SET 
	artist=:artist,
	track=:title,
	album=:album,
	path=:filepath,
	tags=:tags,
	priority=:priority,
	lastplayed=:lastplayed,
	lastrequested=:lastrequested,
	usable=:usable,
	accepter=:acceptor,
	lasteditor=:lasteditor,
	hash=:hash,
	requestcount=:requestcount,
	need_reupload=:needreplacement
WHERE
	id=:trackid;
`

func (ts TrackStorage) Update(song radio.Song) error {
	const op errors.Op = "mariadb/TrackStorage.Update"

	// validate
	if !song.HasTrack() {
		return errors.E(op, errors.InvalidArgument, "nil DatabaseTrack", song)
	}

	if song.TrackID == 0 {
		return errors.E(op, errors.InvalidArgument, "TrackID was zero", song)
	}

	// transaction
	handle, tx, err := requireTx(ts.handle)
	if err != nil {
		return errors.E(op, errors.TransactionBegin)
	}
	defer tx.Rollback()

	// execute
	ss := SongStorage{handle}
	// If the song exists the length won't be updated and instead silently not-updated
	if _, err = ss.Create(song); err != nil {
		return errors.E(op, err)
	}

	_, err = sqlx.NamedExec(handle, trackUpdateQuery, song)
	if err != nil {
		return errors.E(op, err)
	}

	return tx.Commit()
}

const trackInsertQuery = `
INSERT INTO
	tracks (
		id,
		artist,
		track,
		album,
		path,
		tags,
		priority,
		lastplayed,
		lastrequested,
		usable,
		accepter,
		lasteditor,
		hash,
		requestcount,
		need_reupload
	) VALUES (
		0,
		:artist,
		:title,
		:album,
		:filepath,
		:tags,
		:priority,
		:lastplayed,
		:lastrequested,
		:usable,
		:acceptor,
		:lasteditor,
		:hash,
		:requestcount,
		:needreplacement
	);
`

func (ts TrackStorage) Insert(song radio.Song) (radio.TrackID, error) {
	const op errors.Op = "mariadb/TrackStorage.Insert"

	// validation
	if !song.HasTrack() {
		return 0, errors.E(op, errors.InvalidArgument, "nil DatabaseTrack", song)
	}

	if song.TrackID != 0 {
		return 0, errors.E(op, errors.InvalidArgument, "TrackID was not zero", song)
	}

	// transaction
	handle, tx, err := requireTx(ts.handle)
	if err != nil {
		return 0, errors.E(op, errors.TransactionBegin)
	}
	defer tx.Rollback()

	// create song if not exist
	_, err = SongStorage{handle}.Create(song)
	if err != nil {
		return 0, errors.E(op, err, song)
	}

	// insert into track table
	new, err := namedExecLastInsertId(handle, trackInsertQuery, song)
	if err != nil {
		return 0, errors.E(op, err, song)
	}

	return radio.TrackID(new), tx.Commit()
}

const trackUpdateMetadataQuery = `
UPDATE tracks SET
	artist=:artist,
	track=:title,
	album=:album,
	path=:filepath,
	tags=:tags,
	need_reupload=:needreplacement
WHERE id=:trackid;
`

func (ts TrackStorage) UpdateMetadata(song radio.Song) error {
	const op errors.Op = "mariadb/TrackStorage.Update"

	// validation
	if !song.HasTrack() {
		return errors.E(op, errors.InvalidArgument, "nil DatabaseTrack", song)
	}

	if song.TrackID == 0 {
		return errors.E(op, errors.InvalidArgument, "TrackID was zero", song)
	}

	// execute
	_, err := sqlx.NamedExec(ts.handle, trackUpdateMetadataQuery, song)
	if err != nil {
		return errors.E(op, err, song)
	}

	return nil
}

func (ts TrackStorage) UpdateUsable(song radio.Song, state radio.TrackState) error {
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

var trackBeforeLastRequestedQuery = expand(`
SELECT
	{maybeSongColumns},
	{trackColumns},
	{lastplayedSelect},
	NOW() AS synctime
FROM
	tracks
LEFT JOIN
	esong ON tracks.hash = esong.hash
WHERE
	track.lastrequested < ?
AND
	requestcount > 0;
`)

// BeforeLastRequested implements radio.TrackStorage
func (ts TrackStorage) BeforeLastRequested(before time.Time) ([]radio.Song, error) {
	const op errors.Op = "mariadb/TrackStorage.BeforeLastRequested"

	var songs = []radio.Song{}

	err := sqlx.Select(ts.handle, &songs, trackBeforeLastRequestedQuery, before)
	if err != nil {
		return nil, errors.E(op, err)
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
