package storagetest

import (
	"reflect"
	"slices"
	"testing"

	radio "github.com/R-a-dio/valkyrie"
	"github.com/leanovate/gopter/arbitrary"
	"github.com/leanovate/gopter/gen"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func (suite *Suite) TestQueueStore(t *testing.T) {
	s := suite.Storage(t)
	qs := s.Queue(suite.ctx)

	err := qs.Store("test", nil)
	require.NoError(t, err)

	a := newArbitrary()
	queue := generate[[]radio.QueueEntry](a)
	err = qs.Store("test", queue)
	require.NoError(t, err)
}

func (suite *Suite) TestQueueLoad(t *testing.T) {
	s := suite.Storage(t)
	qs := s.Queue(suite.ctx)

	queue, err := qs.Load("test")
	require.NoError(t, err)
	require.Len(t, queue, 0)
}

func (suite *Suite) TestQueueStoreAndLoad(t *testing.T) {
	s := suite.Storage(t)
	qs := s.Queue(suite.ctx)
	ts := s.Track(suite.ctx)
	ss := s.Song(suite.ctx)

	a := newArbitrary()
	queue := generate[[]radio.QueueEntry](a)
	for i, entry := range queue {
		entry.LastPlayedBy = nil // nil this since it's hard to test
		// create the song entry for the song it generated
		entry.Song.Hydrate() // sync metadata and hashes
		song, err := ss.Create(entry.Song)
		require.NoError(t, err)
		song.DatabaseTrack = entry.DatabaseTrack // transplant the DatabaseTrack
		entry.Song = *song                       // replace the song with what the storage gave us back
		// create the tracks entry for the song it generated
		entry.TrackID = 0 // Insert doesn't like if there is an existing ID
		tid, err := ts.Insert(entry.Song)
		require.NoError(t, err)
		entry.TrackID = tid // replace the ID with what the storage gave us back
		queue[i] = entry
	}

	err := qs.Store("test", queue)
	require.NoError(t, err)

	out, err := qs.Load("test")
	require.NoError(t, err)
	require.Len(t, out, len(queue))
	slices.CompareFunc(queue, out, func(a, b radio.QueueEntry) int {
		assert.EqualExportedValues(t, a, b)
		return 0
	})
}

func newArbitrary() *arbitrary.Arbitraries {
	a := arbitrary.DefaultArbitraries()
	a.RegisterGen(gen.AlphaString())
	a.RegisterGen(genDuration())
	a.RegisterGen(genTime())
	a.RegisterGen(genTimePtr())
	var i []interface{}
	for _, perm := range radio.AllUserPermissions() {
		i = append(i, perm)
	}
	a.RegisterGen(gen.OneConstOf(i...))
	return a
}

func generate[T any](a *arbitrary.Arbitraries) T {
	return OneOff[T](a.GenForType(reflect.TypeFor[T]()))
}
