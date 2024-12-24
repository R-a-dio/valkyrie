package mariadb

import (
	"database/sql"
	"slices"
	"strings"
	"time"

	radio "github.com/R-a-dio/valkyrie"
	"github.com/R-a-dio/valkyrie/errors"
	"github.com/R-a-dio/valkyrie/util"
	"github.com/go-sql-driver/mysql"
	"github.com/jmoiron/sqlx"
)

const (
	// FavePriorityIncrement is the amount we increase/decrease priority by
	// on a track when it gets favorited/unfavorited
	FavePriorityIncrement = 1
	// RequestPriorityIncremenet is the amount we increase priority by when
	// a track gets requested
	RequestPriorityIncrement = 1
	// RequestCountIncrement is the amount we increase requestcount by when
	// a track gets requested
	RequestCountIncrement = 2
)

func expand(query string) string {
	var orig = query
	query = strings.ReplaceAll(query, "{trackColumns}", trackColumns)
	query = strings.ReplaceAll(query, "{maybeTrackColumns}", maybeTrackColumns)
	query = strings.ReplaceAll(query, "{songColumns}", songColumns)
	query = strings.ReplaceAll(query, "{maybeSongColumns}", maybeSongColumns)
	query = strings.ReplaceAll(query, "{lastplayedSelect}", lastplayedSelect)
	query = strings.ReplaceAll(query, "{newsColumns}", newsColumns)
	if orig == query {
		panic("expand called but nothing was expanded")
	}
	return query
}

const trackColumns = `
	tracks.id AS trackid,
	COALESCE(tracks.artist, '') AS artist,
	COALESCE(tracks.track, '') AS title,
	COALESCE(tracks.album, '') AS album,
	COALESCE(tracks.path, '') AS filepath,
	COALESCE(tracks.tags, '') AS tags,
	tracks.accepter AS acceptor,
	tracks.lasteditor,
	tracks.priority,
	IF(tracks.usable, TRUE, FALSE) AS usable,
	IF(tracks.need_reupload, TRUE, FALSE) AS needreplacement,
	tracks.lastrequested,
	tracks.requestcount
`

const maybeTrackColumns = `
	COALESCE(tracks.id, CAST(0 AS UNSIGNED INT)) AS trackid,
	COALESCE(tracks.artist, '') AS artist,
	COALESCE(tracks.track, '') AS title,
	COALESCE(tracks.album, '') AS album,
	COALESCE(tracks.path, '') AS filepath,
	COALESCE(tracks.tags, '') AS tags,
	COALESCE(tracks.accepter, '') AS acceptor,
	COALESCE(tracks.lasteditor, '') AS lasteditor,
	COALESCE(tracks.priority, 0) AS priority,
	IF(tracks.usable, TRUE, FALSE) AS usable,
	IF(tracks.need_reupload, TRUE, FALSE) AS needreplacement,
	COALESCE(tracks.lastrequested, TIMESTAMP('0000-00-00 00:00:00')) AS lastrequested,
	COALESCE(tracks.requestcount, 0) AS requestcount
`

const songColumns = `
	esong.id AS id,
	esong.meta AS metadata,
	esong.hash AS hash,
	esong.hash_link AS hashlink,
	to_go_duration(esong.len) AS length
`

const maybeSongColumns = `
	COALESCE(esong.id, CAST(0 AS UNSIGNED INT)) AS id,
	COALESCE(esong.meta, '') AS metadata,
	COALESCE(esong.hash, '') AS hash,
	COALESCE(esong.hash_link, '') AS hashlink,
	COALESCE(to_go_duration(esong.len), CAST(0 AS UNSIGNED INT)) AS length
`

const lastplayedSelect = `
	COALESCE((SELECT dt FROM eplay JOIN esong AS esong2 ON esong2.id = eplay.isong WHERE esong2.hash_link=esong.hash_link ORDER BY dt DESC LIMIT 1), TIMESTAMP('0000-00-00 00:00:00')) AS lastplayed
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
		:hashlink,
		from_go_duration(:length)
	)
`

// Create implements radio.SongStorage
func (ss SongStorage) Create(song radio.Song) (*radio.Song, error) {
	const op errors.Op = "mariadb/SongStorage.Create"
	handle, deferFn := ss.handle.span(op)
	defer deferFn()

	if song.Metadata == "" {
		return nil, errors.E(op, errors.InvalidArgument)
	}
	// call hydrate for the caller, should basically be a no-op anyway
	song.Hydrate()

	_, err := sqlx.NamedExec(handle, songCreateQuery, song)
	if err != nil && !IsDuplicateKeyErr(err) {
		return nil, errors.E(op, err)
	}

	new, err := SongStorage{handle}.FromHash(song.Hash)
	if err != nil {
		return nil, errors.E(op, err)
	}
	return new, nil
}

// FromMetadata implements radio.SongStorage
func (ss SongStorage) FromMetadata(metadata string) (*radio.Song, error) {
	const op errors.Op = "mariadb/SongStorage.FromMetadata"
	handle, deferFn := ss.handle.span(op)
	defer deferFn()

	if metadata == "" {
		return nil, errors.E(op, errors.InvalidArgument)
	}

	song, err := SongStorage{handle}.FromHash(radio.NewSongHash(metadata))
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
	esong
LEFT JOIN
	tracks ON tracks.hash=esong.hash_link
WHERE
	esong.hash=?
LIMIT 1;
`)

// FromHash implements radio.SongStorage
func (ss SongStorage) FromHash(hash radio.SongHash) (*radio.Song, error) {
	const op errors.Op = "mariadb/SongStorage.FromHash"
	handle, deferFn := ss.handle.span(op)
	defer deferFn()

	if hash.IsZero() {
		return nil, errors.E(op, errors.InvalidArgument)
	}

	var song radio.Song

	err := sqlx.Get(handle, &song, songFromHashQuery, hash)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, errors.E(op, errors.SongUnknown)
		}
		return nil, errors.E(op, err)
	}
	// remove a DatabaseTrack if we allocated it but it ended up being
	// zero
	if song.HasTrack() && song.TrackID == 0 {
		song.DatabaseTrack = nil
	}

	return &song, nil
}

var songLastPlayedQuery = expand(`
SELECT
	{maybeTrackColumns},
	{songColumns},
	eplay.dt AS lastplayed,
	COALESCE(users.id, 0) AS 'lastplayedby.id',
	COALESCE(users.user, '') AS 'lastplayedby.username',
	COALESCE(users.pass, '') AS 'lastplayedby.password',
	COALESCE(users.email, '') AS 'lastplayedby.email',
	COALESCE(users.ip, '') AS 'lastplayedby.ip',
	COALESCE(users.updated_at, TIMESTAMP('0000-00-00 00:00:00')) AS 'lastplayedby.updated_at',
	COALESCE(users.deleted_at, TIMESTAMP('0000-00-00 00:00:00')) AS 'lastplayedby.deleted_at',
	COALESCE(users.created_at, TIMESTAMP('0000-00-00 00:00:00')) AS 'lastplayedby.created_at',
	(SELECT group_concat(permission) FROM permissions WHERE user_id=users.id) AS 'lastplayedby.userpermissions',
	COALESCE(djs.id, 0) AS 'lastplayedby.dj.id',
	COALESCE(djs.regex, '') AS 'lastplayedby.dj.regex',
	COALESCE(djs.djname, '') AS 'lastplayedby.dj.name',
	COALESCE(djs.djtext, '') AS 'lastplayedby.dj.text',
	COALESCE(djs.djimage, '') AS 'lastplayedby.dj.image',
	COALESCE(djs.visible, 0) AS 'lastplayedby.dj.visible',
	COALESCE(djs.priority, 0) AS 'lastplayedby.dj.priority',
	COALESCE(djs.role, '') AS 'lastplayedby.dj.role',
	COALESCE(djs.css, '') AS 'lastplayedby.dj.css',
	COALESCE(djs.djcolor, '') AS 'lastplayedby.dj.color',
	COALESCE(themes.id, 0) AS 'lastplayedby.dj.theme.id',
	COALESCE(themes.name, 'default') AS 'lastplayedby.dj.theme.name',
	COALESCE(themes.display_name, 'default') AS 'lastplayedby.dj.theme.displayname',
	COALESCE(themes.author, 'unknown') AS 'lastplayedby.dj.theme.author',
	NOW() AS synctime
FROM
	esong
RIGHT JOIN
	eplay ON esong.id = eplay.isong
LEFT JOIN
	tracks ON esong.hash_link = tracks.hash
LEFT JOIN
	djs ON eplay.djs_id = djs.id
LEFT JOIN
	users ON djs.id = users.djid
LEFT JOIN
	themes ON djs.theme_id = themes.id
WHERE
	eplay.id < ?
ORDER BY
	eplay.id DESC
LIMIT ?;
`)

// LastPlayed implements radio.SongStorage
func (ss SongStorage) LastPlayed(key radio.LastPlayedKey, amountPerPage int) ([]radio.Song, error) {
	const op errors.Op = "mariadb/SongStorage.LastPlayed"
	handle, deferFn := ss.handle.span(op)
	defer deferFn()

	var songs = make([]radio.Song, 0, amountPerPage)

	err := sqlx.Select(handle, &songs, songLastPlayedQuery, key, amountPerPage)
	if err != nil {
		return nil, errors.E(op, err)
	}

	for i := range songs {
		if songs[i].DatabaseTrack != nil && songs[i].TrackID == 0 {
			songs[i].DatabaseTrack = nil
		}
		if songs[i].LastPlayedBy != nil && songs[i].LastPlayedBy.ID == 0 {
			songs[i].LastPlayedBy = nil
		}
	}

	return songs, nil
}

func (ss SongStorage) LastPlayedPagination(key radio.LastPlayedKey, amountPerPage, pageCount int) (prev, next []radio.LastPlayedKey, err error) {
	const op errors.Op = "mariadb/SongStorage.LastPlayedPagination"
	handle, deferFn := ss.handle.span(op)
	defer deferFn()

	total := amountPerPage * pageCount
	tmp := make([]radio.LastPlayedKey, 0, total)

	query := `
	SELECT
		id
	FROM
		eplay
	WHERE
		id < ?
	ORDER BY
		id DESC
	LIMIT ?;
	`

	err = sqlx.Select(handle, &tmp, query, key, total)
	if err != nil {
		return nil, nil, errors.E(op, err)
	}
	// reduce to just the page boundaries
	next = util.ReduceWithStep(tmp, amountPerPage)

	// reset tmp for the prev set
	tmp = tmp[:0]
	query = `
	SELECT
		id
	FROM
		eplay
	WHERE
		id > ?
	ORDER BY
		id ASC
	LIMIT ?;
	`

	err = sqlx.Select(handle, &tmp, query, key, total)
	if err != nil {
		return nil, nil, errors.E(op, err)
	}

	// reduce to just the page boundaries
	prev = util.ReduceWithStep(tmp, amountPerPage)
	if util.ReduceHasLeftover(tmp, amountPerPage) {
		prev = append(prev, radio.LPKeyLast)
	}
	// reverse since they're in ascending order
	slices.Reverse(prev)

	return prev, next, nil
}

// LastPlayedCount implements radio.SongStorage
func (ss SongStorage) LastPlayedCount() (int64, error) {
	const op errors.Op = "mariadb/SongStorage.LastPlayedCount"
	handle, deferFn := ss.handle.span(op)
	defer deferFn()

	var query = `SELECT count(*) FROM eplay;`
	var playCount int64

	err := sqlx.Get(handle, &playCount, query)
	if err != nil {
		return 0, errors.E(op, err)
	}
	return playCount, nil
}

// PlayedCount implements radio.SongStorage
func (ss SongStorage) PlayedCount(song radio.Song) (int64, error) {
	const op errors.Op = "mariadb/SongStorage.PlayedCount"
	handle, deferFn := ss.handle.span(op)
	defer deferFn()

	if song.HashLink.IsZero() {
		return 0, errors.E(op, errors.InvalidArgument)
	}

	var query = `
		SELECT
			count(*)
		FROM
			eplay
		JOIN
			esong ON esong.id = eplay.isong
		WHERE
			esong.hash_link=?;
	`
	var playedCount int64

	err := sqlx.Get(handle, &playedCount, query, song.HashLink)
	if err != nil {
		if errors.IsE(err, sql.ErrNoRows) {
			return 0, errors.E(op, errors.SongUnknown) // TODO: implement this
		}
		return 0, errors.E(op, err)
	}
	return playedCount, nil
}

// AddPlay implements radio.SongStorage
func (ss SongStorage) AddPlay(song radio.Song, user radio.User, ldiff *radio.Listeners) error {
	const op errors.Op = "mariadb/SongStorage.AddPlay"
	handle, deferFn := ss.handle.span(op)
	defer deferFn()

	if song.ID == 0 {
		return errors.E(op, errors.InvalidArgument)
	}

	var djid *radio.DJID
	if user.DJ.ID != 0 {
		djid = &user.DJ.ID
	}

	var query = `INSERT INTO eplay (isong, djs_id, ldiff) VALUES (?, ?, ?);`

	_, err := handle.Exec(query, song.ID, djid, ldiff)
	if err != nil {
		return errors.E(op, err)
	}
	return nil
}

// FavoriteCount implements radio.SongStorage
func (ss SongStorage) FavoriteCount(song radio.Song) (int64, error) {
	const op errors.Op = "mariadb/SongStorage.FavoriteCount"
	handle, deferFn := ss.handle.span(op)
	defer deferFn()

	if song.HashLink.IsZero() {
		return 0, errors.E(op, errors.InvalidArgument)
	}

	var query = `
		SELECT
			count(*)
		FROM
			efave
		JOIN
			esong ON esong.id = efave.isong
		WHERE
			esong.hash_link=?;
	`
	var faveCount int64

	err := sqlx.Get(handle, &faveCount, query, song.HashLink)
	if err != nil {
		return 0, errors.E(op, err)
	}
	return faveCount, nil
}

// Favorites implements radio.SongStorage
func (ss SongStorage) Favorites(song radio.Song) ([]string, error) {
	const op errors.Op = "mariadb/SongStorage.Favorites"
	handle, deferFn := ss.handle.span(op)
	defer deferFn()

	if song.HashLink.IsZero() {
		return nil, errors.E(op, errors.InvalidArgument)
	}

	var query = `
	SELECT DISTINCT
		enick.nick
	FROM
		efave
	JOIN
		enick ON enick.id = efave.inick
	JOIN
		esong ON esong.id = efave.isong
	WHERE
		esong.hash_link=?;
	`

	var users []string

	err := sqlx.Select(handle, &users, query, song.HashLink)
	if err != nil {
		return nil, errors.E(op, err)
	}

	return users, nil
}

func (ss SongStorage) UpdateHashLink(old, new radio.SongHash) error {
	const op errors.Op = "mariadb/SongStorage.UpdateHashLink"
	handle, deferFn := ss.handle.span(op)
	defer deferFn()

	if old.IsZero() || new.IsZero() {
		return errors.E(op, errors.InvalidArgument)
	}

	var query = `UPDATE esong SET hash_link=? WHERE hash=?;`

	_, err := handle.Exec(query, new, old)
	if err != nil {
		return errors.E(op, err)
	}

	// update any hash_links pointing to old to the new value
	query = `UPDATE esong SET hash_link=? WHERE hash_link=?;`
	_, err = handle.Exec(query, new, old)
	if err != nil {
		return errors.E(op, err)
	}
	return nil
}

var songFavoritesOfQuery = expand(`
WITH
	esong
AS (SELECT DISTINCT
		esong.*
	FROM
		enick
	JOIN
		efave ON efave.inick = enick.id
	JOIN
		esong ON esong.id = efave.isong
	WHERE
		enick.nick = ?
	ORDER BY efave.id ASC)
SELECT
	{songColumns},
	{maybeTrackColumns},
	COALESCE(eplay.dt, TIMESTAMP('0000-00-00 00:00:00')) AS lastplayed,
	NOW() AS synctime
FROM
	esong
LEFT JOIN
	tracks ON tracks.hash = esong.hash
LEFT JOIN
	(SELECT
		MAX(dt) AS dt,
		isong
	FROM
		eplay
	GROUP BY
		isong) AS eplay ON eplay.isong = esong.id
LIMIT ? OFFSET ?;
`)

var songFavoritesOfDatabaseOnlyQuery = expand(`
WITH
	esong
AS (SELECT DISTINCT
		esong.*
	FROM
		enick
	JOIN
		efave ON efave.inick = enick.id
	JOIN
		esong ON esong.id = efave.isong
	WHERE
		enick.nick = ?)
SELECT
	{songColumns},
	{trackColumns},
	COALESCE(eplay.dt, TIMESTAMP('0000-00-00 00:00:00')) AS lastplayed,
	NOW() AS synctime
FROM
	tracks
JOIN
	esong ON tracks.hash = esong.hash
LEFT JOIN
	(SELECT
		MAX(dt) AS dt,
		isong
	FROM
		eplay
	GROUP BY
		isong) AS eplay ON eplay.isong = esong.id;
`)

var songFavoritesOfCountQuery = `
SELECT 
	count(*)
FROM 
	enick
JOIN
	efave ON efave.inick = enick.id
WHERE
	enick.nick = ?;
`

// FavoritesOf implements radio.SongStorage
func (ss SongStorage) FavoritesOf(nick string, limit, offset int64) ([]radio.Song, int64, error) {
	const op errors.Op = "mariadb/SongStorage.FavoritesOf"
	handle, deferFn := ss.handle.span(op)
	defer deferFn()

	if nick == "" {
		return nil, 0, errors.E(op, errors.InvalidArgument)
	}

	var songs = []radio.Song{}
	var count int64

	err := sqlx.Select(handle, &songs, songFavoritesOfQuery, nick, limit, offset)
	if err != nil {
		return nil, 0, errors.E(op, err)
	}

	err = sqlx.Get(handle, &count, songFavoritesOfCountQuery, nick)
	if err != nil {
		return nil, 0, errors.E(op, err)
	}

	for i := range songs {
		if songs[i].DatabaseTrack != nil && songs[i].TrackID == 0 {
			songs[i].DatabaseTrack = nil
		}
	}

	return songs, count, nil
}

// FavoritesOfDatabase implements radio.SongStorage
func (ss SongStorage) FavoritesOfDatabase(nick string) ([]radio.Song, error) {
	const op errors.Op = "mariadb/SongStorage.FavoritesOfDatabase"
	handle, deferFn := ss.handle.span(op)
	defer deferFn()

	if nick == "" {
		return nil, errors.E(op, errors.InvalidArgument)
	}

	var songs = []radio.Song{}

	err := sqlx.Select(handle, &songs, songFavoritesOfDatabaseOnlyQuery, nick)
	if err != nil {
		return nil, errors.E(op, err)
	}

	return songs, nil
}

// AddFavorite implements radio.SongStorage
func (ss SongStorage) AddFavorite(song radio.Song, nick string) (bool, error) {
	const op errors.Op = "mariadb/SongStorage.AddFavorite"
	handle, deferFn := ss.handle.span(op)
	defer deferFn()

	if song.HashLink.IsZero() || nick == "" {
		return false, errors.E(op, errors.InvalidArgument)
	}

	var query = `
	SELECT
		enick.id AS id,
		EXISTS(
			SELECT
				efave.id 
			FROM
				efave
			JOIN
				esong ON esong.id = efave.isong
			WHERE
				inick=enick.id AND esong.hash_link=?
		) AS hasfave
	FROM 
		enick
	WHERE 
		enick.nick=?
	UNION SELECT 0 AS id, 0 AS hasfave FROM DUAL;`

	var info = struct {
		ID      int64
		HasFave bool
	}{}

	err := sqlx.Get(handle, &info, query, song.HashLink, nick)
	if err != nil {
		return false, errors.E(op, err)
	}

	if info.HasFave {
		return false, nil
	}

	// we're gonna do some inserting and updating, do this in a transaction
	handle, tx, err := requireTx(handle)
	if err != nil {
		return false, errors.E(op, err)
	}
	defer tx.Rollback()

	if info.ID == 0 {
		query = `INSERT INTO enick (nick) VALUES (?)`
		res, err := handle.Exec(query, nick)
		if err != nil {
			return false, errors.E(op, err)
		}

		info.ID, err = res.LastInsertId()
		if err != nil {
			panic("LastInsertId not supported")
		}
	}

	query = `INSERT INTO efave (inick, isong) VALUES (?, ?)`
	_, err = handle.Exec(query, info.ID, song.ID)
	if err != nil {
		return false, errors.E(op, err)
	}

	// we increase a search priority when a song gets favorited
	if song.DatabaseTrack != nil && song.TrackID != 0 {
		query = `UPDATE tracks SET priority=priority+? WHERE id=?`
		_, err = handle.Exec(query, FavePriorityIncrement, song.TrackID)
		if err != nil {
			return false, errors.E(op, err)
		}
	}

	if err = tx.Commit(); err != nil {
		return false, errors.E(op, err)
	}

	return true, nil
}

// RemoveFavorite implements radio.SongStorage
func (ss SongStorage) RemoveFavorite(song radio.Song, nick string) (bool, error) {
	const op errors.Op = "mariadb/SongStorage.RemoveFavorite"
	handle, deferFn := ss.handle.span(op)
	defer deferFn()

	if song.HashLink.IsZero() || nick == "" {
		return false, errors.E(op, errors.InvalidArgument)
	}

	var query = `
	DELETE
		efave
	FROM
		efave
	JOIN
		enick ON enick.id = efave.inick
	JOIN
		esong ON efave.isong = esong.id
	WHERE
		enick.nick=? AND esong.hash_link=?;
	`

	res, err := handle.Exec(query, nick, song.HashLink)
	if err != nil {
		return false, errors.E(op, err)
	}

	n, err := res.RowsAffected()
	if err != nil {
		panic("RowsAffected not supported")
	}

	// we decrease a search priority when a song gets unfavorited
	if n > 0 && song.HasTrack() {
		query = `UPDATE tracks SET priority=priority-? WHERE id=?`
		_, err = handle.Exec(query, FavePriorityIncrement, song.TrackID)
		if err != nil {
			return false, errors.E(op, err)
		}
	}
	return n > 0, nil
}

// UpdateLength implements radio.SongStorage
func (ss SongStorage) UpdateLength(song radio.Song, length time.Duration) error {
	const op errors.Op = "mariadb/SongStorage.UpdateLength"
	handle, deferFn := ss.handle.span(op)
	defer deferFn()

	if song.ID == 0 {
		return errors.E(op, errors.InvalidArgument)
	}

	var query = "UPDATE esong SET len=? WHERE id=?;"

	len := int(length / time.Second)
	res, err := handle.Exec(query, len, song.ID)
	if err != nil {
		return errors.E(op, err)
	}

	n, err := res.RowsAffected()
	if err != nil || n > 0 {
		// either RowsAffected is not supported, or we had more than zero rows
		// affected so we succeeded
		return nil
	}

	return errors.E(op, errors.SongUnknown)
}

// TrackStorage implements radio.TrackStorage
type TrackStorage struct {
	handle handle
}

var trackNeedReplacementQuery = expand(`
SELECT
	{trackColumns},
	{maybeSongColumns},
	{lastplayedSelect},
	NOW() AS synctime
FROM
	tracks
LEFT JOIN
	esong ON esong.hash = tracks.hash
WHERE
	tracks.need_reupload = 1;
`)

func (ts TrackStorage) NeedReplacement() ([]radio.Song, error) {
	const op errors.Op = "mariadb/TrackStorage.NeedReplacement"
	handle, deferFn := ts.handle.span(op)
	defer deferFn()

	var songs []radio.Song

	err := sqlx.Select(handle, &songs, trackNeedReplacementQuery)
	if err != nil {
		return nil, errors.E(op, err)
	}

	return songs, nil
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
	handle, deferFn := ts.handle.span(op)
	defer deferFn()

	var song radio.Song

	err := sqlx.Get(handle, &song, trackGetQuery, id)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, errors.E(op, errors.SongUnknown)
		}
		return nil, errors.E(op, err)
	}

	song.Hydrate()
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
`)

func (ts TrackStorage) AllRaw() ([]radio.Song, error) {
	const op errors.Op = "mariadb/TrackStorage.All"
	handle, deferFn := ts.handle.span(op)
	defer deferFn()

	query := expand(`
	SELECT
		{trackColumns},
		tracks.hash AS hash,
		tracks.lastplayed AS lastplayed
	FROM
		tracks;
	`)
	var songs = []radio.Song{}

	err := sqlx.Select(handle, &songs, query)
	if err != nil {
		return nil, errors.E(op, err)
	}

	return songs, nil
}

// All implements radio.TrackStorage
func (ts TrackStorage) All() ([]radio.Song, error) {
	const op errors.Op = "mariadb/TrackStorage.All"
	handle, deferFn := ts.handle.span(op)
	defer deferFn()

	var songs = []radio.Song{}

	err := sqlx.Select(handle, &songs, trackAllQuery)
	if err != nil {
		return nil, errors.E(op, err)
	}

	for i := range songs {
		songs[i].Hydrate()
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
	handle, deferFn := ts.handle.span(op)
	defer deferFn()

	var songs = []radio.Song{}

	err := sqlx.Select(handle, &songs, trackUnusableQuery)
	if err != nil {
		return nil, errors.E(op, err)
	}

	for i := range songs {
		songs[i].Hydrate()
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
	handle, deferFn := ts.handle.span(op)
	defer deferFn()

	// validation
	if !song.HasTrack() {
		return 0, errors.E(op, errors.InvalidArgument, "nil DatabaseTrack", song)
	}

	if song.TrackID != 0 {
		return 0, errors.E(op, errors.InvalidArgument, "TrackID was not zero", song)
	}

	// transaction
	handle, tx, err := requireTx(handle)
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
	hash=:hash,
	tags=:tags,
	need_reupload=:needreplacement,
	lasteditor=:lasteditor
WHERE id=:trackid;
`

func (ts TrackStorage) UpdateMetadata(song radio.Song) error {
	const op errors.Op = "mariadb/TrackStorage.UpdateMetadata"
	handle, deferFn := ts.handle.span(op)
	defer deferFn()

	// validation
	if !song.HasTrack() {
		return errors.E(op, errors.InvalidArgument, "nil DatabaseTrack", song)
	}

	if song.TrackID == 0 {
		return errors.E(op, errors.InvalidArgument, "TrackID was zero", song)
	}

	handle, tx, err := requireTx(handle)
	if err != nil {
		return errors.E(op, err, song)
	}
	defer tx.Rollback()

	// since a metadata update means a hash update we recalculate the hash here
	// to see if either Artist or Title has changed
	oldHash := song.Hash

	// set the metadata to nothing so Hydrate will fill it in for us
	song.Metadata = ""
	song.Hydrate()

	if song.Hash != oldHash {
		// hash is different, so make sure we have a song entry for it
		otherSong, err := SongStorage{handle}.Create(song)
		if err != nil {
			return errors.E(op, err, song)
		}
		// update the song entries ID since we might've just created a new one
		song.ID = otherSong.ID

		// update the old song entry's hash link to point to our newly created song
		err = SongStorage{handle}.UpdateHashLink(oldHash, song.Hash)
		if err != nil {
			return errors.E(op, err)
		}
	}

	// after all of that we can now finally update our tracks entry
	_, err = sqlx.NamedExec(handle, trackUpdateMetadataQuery, song)
	if err != nil {
		return errors.E(op, err, song)
	}

	err = tx.Commit()
	if err != nil {
		return errors.E(op, err, song)
	}
	return nil
}

func (ts TrackStorage) UpdateUsable(song radio.Song, state radio.TrackState) error {
	const op errors.Op = "mariadb/TrackStorage.UpdateUsable"
	handle, deferFn := ts.handle.span(op)
	defer deferFn()

	if !song.HasTrack() {
		return errors.E(op, errors.InvalidArgument)
	}

	var query = `
	UPDATE
		tracks
	SET
		usable=?
	WHERE
		id=?;
	`

	res, err := handle.Exec(query, state, song.TrackID)
	if err != nil {
		return errors.E(op, err)
	}

	n, err := res.RowsAffected()
	if err != nil || n > 0 {
		// either RowsAffected is not supported, or we had more than zero rows
		// affected so we succeeded
		return nil
	}

	return errors.E(op, errors.SongUnknown)
}

// UpdateRequestInfo updates the time the track given was last requested
// and increases the time between requests for the song.
//
// implements radio.TrackStorage
func (ts TrackStorage) UpdateRequestInfo(id radio.TrackID) error {
	const op errors.Op = "mariadb/TrackStorage.UpdateRequestInfo"
	handle, deferFn := ts.handle.span(op)
	defer deferFn()

	var query = `UPDATE tracks SET lastrequested=NOW(),
	requestcount=requestcount+?, priority=priority+? WHERE id=?;`

	res, err := handle.Exec(query, RequestCountIncrement, RequestPriorityIncrement, id)
	if err != nil {
		return errors.E(op, err)
	}

	n, err := res.RowsAffected()
	if err != nil || n > 0 {
		// either RowsAffected is not supported, or we had more than zero rows
		// affected so we succeeded
		return nil
	}

	return errors.E(op, errors.SongUnknown)
}

// UpdateLastPlayed implements radio.TrackStorage
func (ts TrackStorage) UpdateLastPlayed(id radio.TrackID) error {
	const op errors.Op = "mariadb/TrackStorage.UpdateLastPlayed"
	handle, deferFn := ts.handle.span(op)
	defer deferFn()

	var query = `UPDATE tracks SET lastplayed=NOW() WHERE id=?;`

	res, err := handle.Exec(query, id)
	if err != nil {
		return errors.E(op, err)
	}

	n, err := res.RowsAffected()
	if err != nil || n > 0 {
		// either RowsAffected is not supported, or we had more than zero rows
		// affected so we succeeded
		return nil
	}

	return errors.E(op, errors.SongUnknown)
}

// UpdateLastRequested implements radio.TrackStorage
func (ts TrackStorage) UpdateLastRequested(id radio.TrackID) error {
	const op errors.Op = "mariadb/TrackStorage.UpdateLastRequested"
	handle, deferFn := ts.handle.span(op)
	defer deferFn()

	var query = `
	UPDATE
		tracks
	SET 
		lastrequested=NOW()
	WHERE
		id=?;
	`

	res, err := handle.Exec(query, id)
	if err != nil {
		return errors.E(op, err)
	}

	n, err := res.RowsAffected()
	if err != nil || n > 0 {
		// either RowsAffected is not supported, or we had more than zero rows
		// affected so we succeeded
		return nil
	}

	return errors.E(op, errors.SongUnknown)
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
	tracks.lastrequested < ?
AND
	requestcount > 0;
`)

// BeforeLastRequested implements radio.TrackStorage
func (ts TrackStorage) BeforeLastRequested(before time.Time) ([]radio.Song, error) {
	const op errors.Op = "mariadb/TrackStorage.BeforeLastRequested"
	handle, deferFn := ts.handle.span(op)
	defer deferFn()

	var songs = []radio.Song{}

	err := sqlx.Select(handle, &songs, trackBeforeLastRequestedQuery, before)
	if err != nil {
		return nil, errors.E(op, err)
	}

	for i := range songs {
		songs[i].Hydrate()
	}
	return songs, nil
}

// DecrementRequestCount implements radio.TrackStorage
func (ts TrackStorage) DecrementRequestCount(before time.Time) error {
	const op errors.Op = "mariadb/TrackStorage.DecrementRequestCount"
	handle, deferFn := ts.handle.span(op)
	defer deferFn()

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

	_, err := handle.Exec(query, before)
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
	handle, deferFn := ts.handle.span(op)
	defer deferFn()

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
	err := sqlx.Select(handle, &candidates, query)
	if err != nil {
		return nil, errors.E(op, err)
	}

	return candidates, nil
}

func (ts TrackStorage) Delete(id radio.TrackID) error {
	const op errors.Op = "mariadb/TrackStorage.Delete"
	handle, deferFn := ts.handle.span(op)
	defer deferFn()

	var query = `DELETE FROM tracks WHERE id=?`

	res, err := handle.Exec(query, id)
	if err != nil {
		return errors.E(op, err)
	}

	n, err := res.RowsAffected()
	if err != nil || n > 0 {
		// either RowsAffected is not supported, or we had more than zero rows
		// affected so we succeeded
		return nil
	}

	return errors.E(op, errors.SongUnknown)
}

var trackRandomQuery = expand(`SELECT
	{trackColumns},
	{maybeSongColumns},
	{lastplayedSelect},
	NOW() AS synctime
FROM 
    tracks 
JOIN
    (SELECT tracks.id FROM tracks WHERE usable=1 ORDER BY rand() LIMIT 0,1) AS a ON tracks.id = a.id
LEFT JOIN
    esong on tracks.hash = esong.hash;
`)

func (ts TrackStorage) Random() (*radio.Song, error) {
	const op errors.Op = "mariadb/TrackStorage.Random"
	handle, deferFn := ts.handle.span(op)
	defer deferFn()

	var song radio.Song

	err := sqlx.Get(handle, &song, trackRandomQuery)
	if err != nil {
		return nil, errors.E(op, err)
	}

	return &song, nil
}
