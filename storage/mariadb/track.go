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
	esong.hash_link AS hashlink,
	to_go_duration(esong.len) AS length
`

const maybeSongColumns = `
	IFNULL(esong.id, 0) AS id,
	IFNULL(esong.meta, '') AS metadata,
	IFNULL(esong.hash, '') AS hash,
	IFNULL(esong.hash_link, '') AS hashlink,
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
		:hashlink,
		from_go_duration(:length)
	)
`

// Create implements radio.SongStorage
func (ss SongStorage) Create(song radio.Song) (*radio.Song, error) {
	const op errors.Op = "mariadb/SongStorage.Create"
	handle, deferFn := ss.handle.span(op)
	defer deferFn()

	// TODO: see if we want to not use hydrate here and leave it up to the caller instead
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
	handle, deferFn := ss.handle.span(op)
	defer deferFn()

	var song radio.Song

	err := sqlx.Get(handle, &song, songFromHashQuery, hash)
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
	IFNULL(users.id, 0) AS 'lastplayedby.id',
	IFNULL(users.user, '') AS 'lastplayedby.username',
	IFNULL(users.pass, '') AS 'lastplayedby.password',
	IFNULL(users.email, '') AS 'lastplayedby.email',
	IFNULL(users.ip, '') AS 'lastplayedby.ip',
	IFNULL(users.updated_at, TIMESTAMP('0000-00-00 00:00:00')) AS 'lastplayedby.updated_at',
	IFNULL(users.deleted_at, TIMESTAMP('0000-00-00 00:00:00')) AS 'lastplayedby.deleted_at',
	IFNULL(users.created_at, TIMESTAMP('0000-00-00 00:00:00')) AS 'lastplayedby.created_at',
	(SELECT group_concat(permission) FROM permissions WHERE user_id=users.id) AS 'lastplayedby.userpermissions',
	IFNULL(djs.id, 0) AS 'lastplayedby.dj.id',
	IFNULL(djs.regex, '') AS 'lastplayedby.dj.regex',
	IFNULL(djs.djname, '') AS 'lastplayedby.dj.name',
	IFNULL(djs.djtext, '') AS 'lastplayedby.dj.text',
	IFNULL(djs.djimage, '') AS 'lastplayedby.dj.image',
	IFNULL(djs.visible, 0) AS 'lastplayedby.dj.visible',
	IFNULL(djs.priority, 0) AS 'lastplayedby.dj.priority',
	IFNULL(djs.role, '') AS 'lastplayedby.dj.role',
	IFNULL(djs.css, '') AS 'lastplayedby.dj.css',
	IFNULL(djs.djcolor, '') AS 'lastplayedby.dj.color',
	IFNULL(themes.id, 0) AS 'lastplayedby.dj.theme.id',
	IFNULL(themes.name, 'default') AS 'lastplayedby.dj.theme.name',
	IFNULL(themes.display_name, 'default') AS 'lastplayedby.dj.theme.displayname',
	IFNULL(themes.author, 'unknown') AS 'lastplayedby.dj.theme.author',
	NOW() AS synctime
FROM
	esong
RIGHT JOIN
	eplay ON esong.id = eplay.isong
LEFT JOIN
	tracks ON esong.hash = tracks.hash
LEFT JOIN
	djs ON eplay.djs_id = djs.id
LEFT JOIN
	users ON djs.id = users.djid
LEFT JOIN
	themes ON djs.theme_id = themes.id
ORDER BY
	eplay.dt DESC, eplay.id DESC
LIMIT ? OFFSET ?;
`)

// LastPlayed implements radio.SongStorage
func (ss SongStorage) LastPlayed(offset, amount int64) ([]radio.Song, error) {
	const op errors.Op = "mariadb/SongStorage.LastPlayed"
	handle, deferFn := ss.handle.span(op)
	defer deferFn()

	var songs = make([]radio.Song, 0, amount)

	err := sqlx.Select(handle, &songs, songLastPlayedQuery, amount, offset)
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

	var query = `SELECT count(*) FROM eplay WHERE isong=?;`
	var playedCount int64

	err := sqlx.Get(handle, &playedCount, query, song.ID)
	if err != nil {
		return 0, errors.E(op, err)
	}
	return playedCount, nil
}

// AddPlay implements radio.SongStorage
func (ss SongStorage) AddPlay(song radio.Song, user radio.User, ldiff *radio.Listeners) error {
	const op errors.Op = "mariadb/SongStorage.AddPlay"
	handle, deferFn := ss.handle.span(op)
	defer deferFn()

	var query = `INSERT INTO eplay (isong, djs_id, ldiff) VALUES (?, ?, ?);`

	_, err := handle.Exec(query, song.ID, user.DJ.ID, ldiff)
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

	var query = `SELECT count(*) FROM efave WHERE isong=?;`
	var faveCount int64

	err := sqlx.Get(handle, &faveCount, query, song.ID)
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

	var query = `SELECT enick.nick FROM efave JOIN enick ON
	enick.id = efave.inick WHERE efave.isong=?`

	var users []string

	err := sqlx.Select(handle, &users, query, song.ID)
	if err != nil {
		return nil, errors.E(op, err)
	}

	return users, nil
}

func (ss SongStorage) UpdateHashLink(entry radio.SongHash, newLink radio.SongHash) error {
	const op errors.Op = "mariadb/SongStorage.UpdateHashLink"
	handle, deferFn := ss.handle.span(op)
	defer deferFn()

	var query = `UPDATE esong SET hash_link=? WHERE hash=?;`

	_, err := handle.Exec(query, newLink, entry)
	if err != nil {
		return errors.E(op, err)
	}
	return nil
}

var songFavoritesOfQuery = expand(`
SELECT
	{songColumns},
	{maybeTrackColumns},
	{lastplayedSelect},
	NOW() AS synctime
FROM
	enick
JOIN
	efave ON efave.inick = enick.id
JOIN
	esong ON esong.id = efave.isong
LEFT JOIN
	tracks ON tracks.hash = esong.hash
WHERE
	enick.nick = ?
ORDER BY efave.id ASC
LIMIT ? OFFSET ?;
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

	return songs, count, nil
}

// AddFavorite implements radio.SongStorage
func (ss SongStorage) AddFavorite(song radio.Song, nick string) (bool, error) {
	const op errors.Op = "mariadb/SongStorage.AddFavorite"
	handle, deferFn := ss.handle.span(op)
	defer deferFn()

	var query = `
	SELECT
		enick.id AS id,
		EXISTS(
			SELECT
				efave.id 
			FROM
				efave
			WHERE inick=enick.id AND isong=?
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

	err := sqlx.Get(handle, &info, query, song.ID, nick)
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

	var query = `DELETE efave FROM efave JOIN enick ON
	enick.id = efave.inick WHERE enick.nick=? AND efave.isong=?;`

	res, err := handle.Exec(query, nick, song.ID)
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

	var query = "UPDATE esong SET len=? WHERE id=?;"

	len := int(length / time.Second)
	_, err := handle.Exec(query, len, song.ID)
	if err != nil {
		return errors.E(op, err)
	}
	return nil
}

// TrackStorage implements radio.TrackStorage
type TrackStorage struct {
	handle handle
}

var trackNeedReplacementQuery = `
SELECT
	tracks.id AS trackid
FROM
	tracks
WHERE
	tracks.need_reupload = 1;
`

func (ts TrackStorage) NeedReplacement() ([]radio.TrackID, error) {
	const op errors.Op = "mariadb/TrackStorage.NeedReplacement"
	handle, deferFn := ts.handle.span(op)
	defer deferFn()

	var songs []radio.TrackID

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
JOIN
	eplay ON eplay.isong = esong.id;
`)

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
	handle, deferFn := ts.handle.span(op)
	defer deferFn()

	// validate
	if !song.HasTrack() {
		return errors.E(op, errors.InvalidArgument, "nil DatabaseTrack", song)
	}

	if song.TrackID == 0 {
		return errors.E(op, errors.InvalidArgument, "TrackID was zero", song)
	}

	// transaction
	handle, tx, err := requireTx(handle)
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
	const op errors.Op = "mariadb/TrackStorage.Update"
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
		// TODO: also update the whole graph
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

	var query = `
	UPDATE
		tracks
	SET
		usable=?
	WHERE
		id=?;
	`

	_, err := handle.Exec(query, state, song.ID)
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
	handle, deferFn := ts.handle.span(op)
	defer deferFn()

	// TODO(wessie): don't hardcode requestcount and priority
	var query = `UPDATE tracks SET lastrequested=NOW(),
	requestcount=requestcount+2, priority=priority+1 WHERE id=?;`

	_, err := handle.Exec(query, id)
	if err != nil {
		return errors.E(op, err)
	}
	return nil
}

// UpdateLastPlayed implements radio.TrackStorage
func (ts TrackStorage) UpdateLastPlayed(id radio.TrackID) error {
	const op errors.Op = "mariadb/TrackStorage.UpdateLastPlayed"
	handle, deferFn := ts.handle.span(op)
	defer deferFn()

	var query = `UPDATE tracks SET lastplayed=NOW() WHERE id=?;`

	_, err := handle.Exec(query, id)
	if err != nil {
		return errors.E(op, err)
	}
	return nil
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

	_, err := handle.Exec(query, id)
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

	_, err := handle.Exec(query, id)
	if err != nil {
		return errors.E(op, err)
	}
	return nil
}
