package storagetest

import (
	"reflect"
	"slices"
	"strconv"
	"testing"
	"time"

	radio "github.com/R-a-dio/valkyrie"
	"github.com/R-a-dio/valkyrie/errors"
	"github.com/leanovate/gopter"
	"github.com/leanovate/gopter/arbitrary"
	"github.com/leanovate/gopter/gen"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func (suite *Suite) TestSongPlayedCount(t *testing.T) {
	s := suite.Storage(t)
	ss := s.Song(suite.ctx)

	// invalid argument
	res, err := ss.PlayedCount(radio.Song{})
	Require(t, errors.InvalidArgument, err)
	require.Zero(t, res)

	// non-existant argument
	res, err = ss.PlayedCount(radio.NewSong("played-count-test"))
	// TODO: fix this, we currently can't tell the difference in the
	// current mariadb implementation
	//Require(t, errors.SongUnknown, err)
	require.NoError(t, err)
	require.Zero(t, res)
}

func (suite *Suite) TestSongFavoriteCount(t *testing.T) {
	s := suite.Storage(t)
	ss := s.Song(suite.ctx)

	// invalid argument
	res, err := ss.FavoriteCount(radio.Song{})
	Require(t, errors.InvalidArgument, err)
	require.Zero(t, res)

	// non-existant argument
	res, err = ss.FavoriteCount(radio.NewSong("favorite-count-test"))
	// TODO: fix this, we currently can't tell the difference in the
	// current mariadb implementation
	// Require(t, errors.SongUnknown, err)
	require.NoError(t, err)
	require.Zero(t, res)
}

func (suite *Suite) TestSongFavorites(t *testing.T) {
	s := suite.Storage(t)
	ss := s.Song(suite.ctx)

	// invalid argument
	res, err := ss.Favorites(radio.Song{})
	Require(t, errors.InvalidArgument, err)
	require.Zero(t, res)

	// non-existant argument
	res, err = ss.Favorites(radio.NewSong("favorites-test"))
	require.NoError(t, err)
	require.Len(t, res, 0)
}

func (suite *Suite) TestSongRemoveFavorite(t *testing.T) {
	s := suite.Storage(t)
	ss := s.Song(suite.ctx)

	// invalid song argument
	removed, err := ss.RemoveFavorite(radio.Song{}, "nickname")
	Require(t, errors.InvalidArgument, err)
	require.False(t, removed, "should not return removed=true")

	// invalid nick argument
	removed, err = ss.RemoveFavorite(radio.NewSong("test-remove-favorite"), "")
	Require(t, errors.InvalidArgument, err)
	require.False(t, removed, "should not return removed=true")

	// non-existant argument
	removed, err = ss.RemoveFavorite(radio.NewSong("test-remove-favorite"), "test")
	require.NoError(t, err)
	require.False(t, removed, "should not return removed=true")
}

func (suite *Suite) TestSongFaves(t *testing.T) {
	s := suite.Storage(t)
	ss := s.Song(suite.ctx)

	nick := "test"

	// create a song
	song, err := ss.Create(radio.NewSong("test-remove-favorite"))
	require.NoError(t, err)
	require.NotNil(t, song)

	// add a favorite, should succeed
	added, err := ss.AddFavorite(*song, nick)
	require.NoError(t, err)
	require.True(t, added, "should have added=true")

	// ask for it to favorite the same thing again, should succeed but tell us
	// nothing was added
	addedAgain, err := ss.AddFavorite(*song, nick)
	require.NoError(t, err)
	require.False(t, addedAgain, "should have added=false since we just added this")

	// ask for the list of faves, should have the one we added above
	faves, n, err := ss.FavoritesOf(nick, 20, 0)
	require.NoError(t, err)
	require.EqualValues(t, 1, n)
	require.Len(t, faves, 1)

	// ask for the favorite count of the song, should also be one
	n, err = ss.FavoriteCount(*song)
	require.NoError(t, err)
	require.EqualValues(t, 1, n)

	// ask for all users that have the song on their list
	nicks, err := ss.Favorites(*song)
	require.NoError(t, err)
	require.Len(t, nicks, 1)
	require.Equal(t, nick, nicks[0])

	// and now we try and remove it
	removed, err := ss.RemoveFavorite(*song, nick)
	require.NoError(t, err)
	require.True(t, removed, "should have removed=true")

	// and then we repeat the checks from above but just with us expecting
	// nothing instead of one entry

	// ask for the list of faves, should have nothing now
	faves, n, err = ss.FavoritesOf(nick, 20, 0)
	require.NoError(t, err)
	require.EqualValues(t, 0, n)
	require.Len(t, faves, 0)

	// ask for the favorite count of the song, should now be zero
	n, err = ss.FavoriteCount(*song)
	require.NoError(t, err)
	require.EqualValues(t, 0, n)

	// ask for all users that have the song on their list
	nicks, err = ss.Favorites(*song)
	require.NoError(t, err)
	require.Len(t, nicks, 0)
}

func (suite *Suite) TestSongUpdateLength(t *testing.T) {
	s := suite.Storage(t)
	ss := s.Song(suite.ctx)

	// invalid argument
	err := ss.UpdateLength(radio.Song{}, time.Second*60)
	Require(t, errors.InvalidArgument, err)

	// non-existant argument
	non := radio.NewSong("test-update-length")
	non.ID = 5 // needs an ID
	err = ss.UpdateLength(non, time.Second*60)
	Require(t, errors.SongUnknown, err)

	song, err := ss.Create(radio.NewSong("an actual song"))
	require.NoError(t, err)
	require.Zero(t, song.Length)

	// update it to be a minute
	err = ss.UpdateLength(*song, time.Minute)
	require.NoError(t, err)
	// retrieve song from storage again, should now have the new length
	new, err := ss.FromHash(song.Hash)
	require.NoError(t, err)
	require.Equal(t, time.Minute, new.Length)
}

func (suite *Suite) TestSongCreateAndRetrieve(t *testing.T) {
	ss := suite.Storage(t).Song(suite.ctx)

	song := radio.Song{
		Metadata: "test-song-create-and-retrieve",
		Length:   time.Second * 300,
	}
	song.Hydrate()

	new, err := ss.Create(song)
	if assert.NoError(t, err) {
		assert.NotNil(t, new)
		assert.True(t, song.EqualTo(*new))
		assert.Equal(t, song.Length, new.Length)
	}

	fromHash, err := ss.FromHash(song.Hash)
	if assert.NoError(t, err) {
		assert.NotNil(t, fromHash)
		assert.True(t, song.EqualTo(*fromHash))
		assert.Equal(t, song.Length, fromHash.Length)
	}

	fromMetadata, err := ss.FromMetadata(song.Metadata)
	if assert.NoError(t, err) {
		assert.NotNil(t, fromMetadata)
		assert.True(t, song.EqualTo(*fromMetadata))
		assert.Equal(t, song.Length, fromMetadata.Length)
	}
}

func (suite *Suite) TestSongCreateAndRetrieveWithTrack(t *testing.T) {
	ss := suite.Storage(t).Song(suite.ctx)
	ts := suite.Storage(t).Track(suite.ctx)

	song := radio.Song{
		Metadata: "test-song-create-and-retrieve",
		Length:   time.Second * 300,
		DatabaseTrack: &radio.DatabaseTrack{
			Title:  "testing title",
			Artist: "testing artist",
			Album:  "testing album",
		},
	}
	song.Hydrate()

	tid, err := ts.Insert(song)
	if assert.NoError(t, err) {
		assert.NotZero(t, tid)
	}
	song.TrackID = tid

	fromHash, err := ss.FromHash(song.Hash)
	if assert.NoError(t, err) {
		assert.NotNil(t, fromHash)
		assert.True(t, song.EqualTo(*fromHash))
		assert.Equal(t, song.Length, fromHash.Length)
		assert.Equal(t, song.DatabaseTrack, fromHash.DatabaseTrack)
	}

	fromMetadata, err := ss.FromMetadata(song.Metadata)
	if assert.NoError(t, err) {
		assert.NotNil(t, fromMetadata)
		assert.True(t, song.EqualTo(*fromMetadata))
		assert.Equal(t, song.Length, fromMetadata.Length)
		assert.Equal(t, song.DatabaseTrack, fromMetadata.DatabaseTrack)
	}
}

func (suite *Suite) TestSongLastPlayed(t *testing.T) {
	ss := suite.Storage(t).Song(suite.ctx)

	base := radio.Song{
		Metadata: "test-song-last-played-",
	}
	user := radio.User{
		DJ: radio.DJ{
			ID: 10,
		},
	}
	amount := 50
	// create 50 testing songs
	var songs []radio.Song
	for i := 0; i < amount; i++ {
		song := base
		song.Length = time.Duration(i*2) * time.Second
		song.Metadata = song.Metadata + strconv.Itoa(i)
		song.Hydrate()

		new, err := ss.Create(song)
		require.NoError(t, err)
		require.NotNil(t, new)
		assert.True(t, song.EqualTo(*new))

		songs = append(songs, *new)
	}

	// now have them all play
	for i, song := range songs {
		err := ss.AddPlay(song, user, nil)
		require.NoError(t, err)

		if i == 15 || i == 40 { // Artificially wait a second in the middle somewhere
			time.Sleep(time.Second)
		}
	}

	n, err := ss.LastPlayedCount()
	require.NoError(t, err)
	assert.Equal(t, amount, int(n))

	// test the full list of songs
	lp, err := ss.LastPlayed(radio.LPKeyLast, amount)
	require.NoError(t, err)
	// reverse them since we added them in 0-49 order but we will get them back as 49-0 order
	slices.Reverse(lp)
	for i, original := range songs {
		assert.True(t, original.EqualTo(lp[i]), "all: expected %s got %s", original.Metadata, lp[i].Metadata)
		// we also don't have any database tracks or users associated with these songs
		// so the songs returned by this call of LastPlayed should have the following properties:
		// 		.HasTrack() should be false
		//		.DatabaseTrack should be nil
		//		.LastPlayedBy should be nil
		assert.False(t, lp[i].HasTrack(), "has track should be false")
		assert.Nil(t, lp[i].DatabaseTrack, "database track should be nil")
		assert.Nil(t, lp[i].LastPlayedBy, "last played by should be nil")
	}

	// test a subset of the list
	lp, err = ss.LastPlayed(radio.LPKeyLast, 20)
	require.NoError(t, err)
	slices.Reverse(lp)
	for i, original := range songs[amount-20 : amount] {
		assert.True(t, original.EqualTo(lp[i]), "subset start: expected %s got %s", original.Metadata, lp[i].Metadata)
	}

	_, next, err := ss.LastPlayedPagination(radio.LPKeyLast, 20, 5)
	require.NoError(t, err)
	require.Len(t, next, 2, "expected to have two pages")

	// test the other end of the subset
	lp, err = ss.LastPlayed(next[1], 20)
	require.NoError(t, err)
	slices.Reverse(lp)

	for i, original := range songs[:10] {
		assert.True(t, original.EqualTo(lp[i]), "subset end: expected %s got %s", original.Metadata, lp[i].Metadata)
	}

	// the below scenario is done by the irc bot, see if that is handled correctly
	_, next, err = ss.LastPlayedPagination(radio.LPKeyLast, 1, 50)
	require.NoError(t, err)
	require.Len(t, next, 50)

	for index := range 20 {
		key := radio.LPKeyLast
		if index > 0 {
			key = next[index-1]
		}
		lp, err = ss.LastPlayed(key, 1)
		require.NoError(t, err)
		original := songs[len(songs)-1-index]
		assert.True(t, original.EqualTo(lp[0]), "expected %s got %s", original, lp[0])
	}
}

func (suite *Suite) TestTrackUpdateMetadata(t *testing.T) {
	s := suite.Storage(t)
	ts := s.Track(suite.ctx)
	ss := s.Song(suite.ctx)

	original := radio.Song{
		DatabaseTrack: &radio.DatabaseTrack{
			Artist:     "artist test",
			Album:      "album test",
			Title:      "title test",
			Acceptor:   "test user",
			LastEditor: "test user",
		},
	}
	original.Hydrate()

	new, err := ts.Insert(original)
	require.NoError(t, err)
	require.NotZero(t, new)

	updated := original
	updated.DatabaseTrack = &radio.DatabaseTrack{
		TrackID:    new,
		Artist:     "new artist",
		Album:      original.Album,
		Title:      original.Title,
		Acceptor:   original.Acceptor,
		LastEditor: "some other user",
	}

	err = ts.UpdateMetadata(updated)
	require.NoError(t, err)

	// we can now get an updated version with all fields we care about updated from the db
	updatedSong, err := ts.Get(new)
	require.NoError(t, err)
	require.NotNil(t, updatedSong)

	// and the old song entry from before we updated
	originalSong, err := ss.FromHash(original.Hash)
	require.NoError(t, err)
	require.NotNil(t, originalSong)

	assert.Equal(t, updatedSong.Hash.String(), originalSong.HashLink.String(),
		"original song entry's hashlink should be pointing to the updated hash")
	assert.Equal(t, updatedSong.Artist, updated.Artist)
	assert.Equal(t, updatedSong.Album, updated.Album)
	assert.Equal(t, updatedSong.Title, updated.Title)
}

func (suite *Suite) TestSongFavoritesOf(t *testing.T) {
	s := suite.Storage(t)
	ss := s.Song(suite.ctx)

	var nick = "test"
	var entries []radio.Song
	for i := range 1000 {
		song, err := ss.Create(radio.NewSong(strconv.Itoa(i)))
		require.NoError(t, err)
		entries = append(entries, *song)
	}

	var faveCountExpected = int64(500)
	for _, song := range entries[:faveCountExpected] {
		added, err := ss.AddFavorite(song, nick)
		require.NoError(t, err)
		require.True(t, added)
	}

	var limit = 50
	var offset = 0
	faves, count, err := ss.FavoritesOf(nick, int64(limit), int64(offset))
	require.NoError(t, err)
	require.Len(t, faves, limit)
	require.Equal(t, faveCountExpected, count)

	db, err := ss.FavoritesOfDatabase(nick)
	require.NoError(t, err)
	require.Len(t, db, 0)
}

func (suite *Suite) TestTrackNeedReplacement(t *testing.T) {
	s := suite.Storage(t)
	ts := s.Track(suite.ctx)

	var entries []radio.Song
	for i := 0; i < 50; i++ {
		song := generateTrack()
		song.NeedReplacement = i%2 == 0
		tid, err := ts.Insert(song)
		require.NoError(t, err)
		song.TrackID = tid
		entries = append(entries, song)
	}

	ret, err := ts.NeedReplacement()
	require.NoError(t, err)

	assert.Len(t, ret, 25)
	for _, song := range ret {
		assert.True(t, song.NeedReplacement, "should have NeedReplacement set")
		exists := slices.ContainsFunc(entries, func(a radio.Song) bool {
			return a.EqualTo(song)
		})
		assert.True(t, exists, "song should be one we inserted")
	}
}

func generateTrack() radio.Song {
	a := arbitrary.DefaultArbitraries()
	generator := gen.Struct(reflect.TypeFor[radio.Song](), map[string]gopter.Gen{
		"ID":           genForType[radio.SongID](a),
		"Length":       genDuration(),
		"LastPlayed":   genTime(),
		"LastPlayedBy": gen.PtrOf(genUser()),
		"DatabaseTrack": gen.StructPtr(reflect.TypeFor[radio.DatabaseTrack](), map[string]gopter.Gen{
			"Artist":   gen.AlphaString(),
			"Title":    gen.AlphaString(),
			"Album":    gen.AlphaString(),
			"Tags":     gen.AlphaString(),
			"FilePath": gen.AlphaString(),
		}),
	})

	song := OneOff[radio.Song](generator)
	song.Hydrate()
	return song
}

func genForType[T any](a *arbitrary.Arbitraries) gopter.Gen {
	return a.GenForType(reflect.TypeFor[T]())
}

func genDuration() gopter.Gen {
	g := gen.Int64Range(0, int64(time.Hour*24))
	return genAsType[int64, time.Duration](g)
}

func genAsType[F, T any](g gopter.Gen) gopter.Gen {
	g = g.WithShrinker(nil)
	return gopter.Gen(func(gp *gopter.GenParameters) *gopter.GenResult {
		res := g(gp)
		v := res.Result.(F)
		vt := reflect.ValueOf(v).Convert(reflect.TypeFor[T]()).Interface()
		return gopter.NewGenResult(vt, nil)
	}).WithShrinker(nil)
}

func (suite *Suite) TestTrackRandom(t *testing.T) {
	s := suite.Storage(t)
	ts := s.Track(suite.ctx)

	songs, err := ts.Random(100)
	assert.NoError(t, err)
	assert.Len(t, songs, 0)
	// TODO: add actual retrieval tests
}

func (suite *Suite) TestTrackRandomFavoriteOf(t *testing.T) {
	s := suite.Storage(t)
	ts := s.Track(suite.ctx)

	songs, err := ts.RandomFavoriteOf("test", 100)
	assert.NoError(t, err)
	assert.Len(t, songs, 0)
	// TODO: add actual retrieval tests
}

func (suite *Suite) TestTrackDelete(t *testing.T) {
	s := suite.Storage(t)
	ts := s.Track(suite.ctx)

	err := ts.Delete(1)
	Require(t, errors.SongUnknown, err)
}

func (suite *Suite) TestTrackQueueCandidates(t *testing.T) {
	s := suite.Storage(t)
	ts := s.Track(suite.ctx)

	res, err := ts.QueueCandidates()
	require.NoError(t, err)
	require.Len(t, res, 0)
}

func (suite *Suite) TestTrackDecrementRequestCount(t *testing.T) {
	s := suite.Storage(t)
	ts := s.Track(suite.ctx)

	err := ts.DecrementRequestCount(time.Now())
	require.NoError(t, err)
}

func (suite *Suite) TestTrackBeforeLastRequested(t *testing.T) {
	s := suite.Storage(t)
	ts := s.Track(suite.ctx)

	res, err := ts.BeforeLastRequested(time.Now())
	require.NoError(t, err)
	require.Len(t, res, 0)
}

func (suite *Suite) TestTrackUpdateLastRequested(t *testing.T) {
	s := suite.Storage(t)
	ts := s.Track(suite.ctx)

	// non-existant song, should error
	err := ts.UpdateLastRequested(5)
	Require(t, errors.SongUnknown, err)
}

func (suite *Suite) TestTrackUpdateLastPlayed(t *testing.T) {
	s := suite.Storage(t)
	ts := s.Track(suite.ctx)

	// non-existant song, should error
	err := ts.UpdateLastPlayed(5)
	Require(t, errors.SongUnknown, err)
}

func (suite *Suite) TestTrackUpdateRequestInfo(t *testing.T) {
	s := suite.Storage(t)
	ts := s.Track(suite.ctx)

	// non-existant song, should error
	err := ts.UpdateRequestInfo(5)
	Require(t, errors.SongUnknown, err)
}

func (suite *Suite) TestTrackUpdateUsable(t *testing.T) {
	s := suite.Storage(t)
	ts := s.Track(suite.ctx)

	// non-existant song, should error
	err := ts.UpdateUsable(radio.Song{
		DatabaseTrack: &radio.DatabaseTrack{
			TrackID: 5,
		},
	}, radio.TrackStateUnverified)
	Require(t, errors.SongUnknown, err)

	// should not like it if we give it an empty song
	err = ts.UpdateUsable(radio.Song{}, radio.TrackStatePlayable)
	Require(t, errors.InvalidArgument, err)
}

func (suite *Suite) TestTrackAll(t *testing.T) {
	s := suite.Storage(t)
	ts := s.Track(suite.ctx)

	// should have no songs
	res, err := ts.All()
	require.NoError(t, err)
	require.Len(t, res, 0)
}

func (suite *Suite) TestTrackUnusable(t *testing.T) {
	s := suite.Storage(t)
	ts := s.Track(suite.ctx)

	// should have no songs
	res, err := ts.Unusable()
	require.NoError(t, err)
	require.Len(t, res, 0)
}

func Assert(t *testing.T, kind errors.Kind, err error) bool {
	t.Helper()
	return assert.True(t, errors.Is(kind, err), "error should be kind %s but is: %v", kind, err)
}

func Require(t *testing.T, kind errors.Kind, err error) {
	t.Helper()
	require.True(t, errors.Is(kind, err), "error should be kind %s but is: %v", kind, err)
}
