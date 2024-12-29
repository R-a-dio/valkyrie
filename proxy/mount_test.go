package proxy

import (
	"bytes"
	"context"
	"net"
	"net/http/httptest"
	"testing"

	radio "github.com/R-a-dio/valkyrie"
	"github.com/R-a-dio/valkyrie/config"
	"github.com/rs/xid"
	"github.com/stretchr/testify/assert"
)

func TestLeastPriority(t *testing.T) {
	// helper functions to create MountSourceClient in the test cases
	prio := func(p uint32) *MountSourceClient {
		return &MountSourceClient{
			Priority: p,
		}
	}
	prioSlice := func(ps ...uint32) []*MountSourceClient {
		var sources = make([]*MountSourceClient, 0, len(ps))
		for _, p := range ps {
			sources = append(sources, prio(p))
		}
		return sources
	}

	testCases := []struct {
		name     string
		sources  []*MountSourceClient
		expected uint32
	}{
		{
			name:     "empty",
			sources:  []*MountSourceClient{},
			expected: 0,
		},
		{
			name:     "nil",
			sources:  nil,
			expected: 0,
		},
		{
			name:     "simple gaps",
			sources:  prioSlice(5, 10, 15, 20),
			expected: 21,
		},
		{
			name:     "simple sequential",
			sources:  prioSlice(0, 1, 2, 3, 4, 5),
			expected: 6,
		},
		{
			name:     "reversed",
			sources:  prioSlice(10, 9, 8, 7, 6, 5, 4, 3, 2, 1, 0),
			expected: 11,
		},
	}
	for _, c := range testCases {
		t.Run(c.name, func(t *testing.T) {
			prio := leastPriority(c.sources)
			assert.Equal(t, c.expected, prio)
		})
	}
}

type mostPriorityTestCase struct {
	name     string
	sources  []*MountSourceClient
	expected *MountSourceClient
}

func TestMostPriority(t *testing.T) {
	// helper functions to create MountSourceClient in the test cases
	prio := func(p uint32) *MountSourceClient {
		return &MountSourceClient{
			Source: &SourceClient{
				ID: radio.SourceID{xid.New()},
			},
			Priority: p,
		}
	}
	prioSlice := func(ps ...uint32) []*MountSourceClient {
		var sources = make([]*MountSourceClient, 0, len(ps))
		for _, p := range ps {
			sources = append(sources, prio(p))
		}
		return sources
	}

	prioCase := func(name string, sources []*MountSourceClient, expectedIndex int) mostPriorityTestCase {
		return mostPriorityTestCase{
			name:     name,
			sources:  sources,
			expected: sources[expectedIndex],
		}
	}

	testCases := []mostPriorityTestCase{
		{"empty", prioSlice(), nil},
		{"nil", nil, nil},
		prioCase("simple gaps", prioSlice(5, 10, 15, 20), 0),
		prioCase("simple sequential", prioSlice(0, 1, 2, 3, 4, 5), 0),
		prioCase("reversed", prioSlice(10, 9, 8, 7, 6, 5, 4, 3, 2, 1, 0), 10),
		prioCase("random", prioSlice(8, 2, 1, 7, 4, 5, 0, 10, 9, 3, 6), 6),
	}
	for _, c := range testCases {
		t.Run(c.name, func(t *testing.T) {
			m := mostPriority(c.sources)
			if c.expected == nil {
				assert.Nil(t, m)
			} else {
				assert.Equal(t, c.expected.Source.ID, m.Source.ID)
			}
		})
	}
}

func TestMountRemoveSource(t *testing.T) {
	ctx := context.Background()
	cfg := config.TestConfig()

	eh := NewEventHandler(ctx, cfg)

	mountName := "/test.mp3"
	contentType := "audio/mpeg"

	conn1, conn2 := net.Pipe()

	mount := NewMount(ctx, cfg, nil, eh, mountName, contentType, nil)

	user := newTestUser("test", "test")
	req := httptest.NewRequest("PUT", mountName, conn2)
	id := radio.SourceID{xid.New()}
	identifier := IdentFromRequest(req)
	metadata := &Metadata{}
	_ = conn1
	source := NewSourceClient(id, "test", contentType, mountName, conn2, *user, identifier, metadata)

	mount.AddSource(ctx, source)

	assert.Equal(t, 1, getSourcesLength(mount), "should have one source")
	assert.True(t, getSource(mount, 0).MW.GetLive(), "only source should be live")

	mount.RemoveSource(ctx, radio.SourceID{})
	assert.Equal(t, 1, getSourcesLength(mount), "should still have one source")
	mount.RemoveSource(ctx, source.ID)
	assert.Equal(t, 0, getSourcesLength(mount), "should have no sources")
}

func getSourcesLength(mount *Mount) int {
	mount.SourcesMu.RLock()
	defer mount.SourcesMu.RUnlock()
	return len(mount.Sources)
}

func getSource(mount *Mount, i int) *MountSourceClient {
	mount.SourcesMu.RLock()
	defer mount.SourcesMu.RUnlock()
	return mount.Sources[i]
}

func TestMountMetadataWriterWrite(t *testing.T) {
	// zero MountMetadataWriter has no output and should just be eating
	// any data we send it
	var mmw MountMetadataWriter

	// test with absolutely nothing
	var data []byte

	n, err := mmw.Write(data)
	if assert.NoError(t, err) {
		assert.EqualValues(t, len(data), n)
	}

	// still no output so should just be silently eaten
	data = []byte("but then with a bit of data")
	n, err = mmw.Write(data)
	if assert.NoError(t, err) {
		assert.EqualValues(t, len(data), n)
	}

	// setup an output
	var buf bytes.Buffer
	mmw.SetWriter(&buf)

	// we now have an output so the data should show up in the output
	data = []byte("but then with a bit of data that should arrive")
	n, err = mmw.Write(data)
	if assert.NoError(t, err) {
		assert.EqualValues(t, len(data), n)
		assert.Equal(t, data, buf.Bytes())
	}

	lastData := data
	// set it back to nothing
	mmw.SetWriter(nil)
	data = []byte("but then with a bit of data that should be eaten again")
	n, err = mmw.Write(data)
	if assert.NoError(t, err) {
		assert.EqualValues(t, len(data), n)
		assert.Equal(t, lastData, buf.Bytes(), "should have same value as before the Write call")
	}
}

func TestMountMetadataWriterLive(t *testing.T) {
	var mmw MountMetadataWriter
	ctx := context.Background()

	assert.False(t, mmw.GetLive(), "zero value should not be live")

	var called bool
	mmw.metadataFn = func(ctx context.Context, s string) error {
		called = true
		return nil
	}

	// we're not live so this should not call metadataFn
	mmw.sendMetadata(ctx)
	assert.False(t, called, "metadataFn should've not been called")

	// now go live, this should call metadataFn
	mmw.SetLive(ctx, true)
	assert.True(t, mmw.GetLive(), "value should be true after SetLive(..., true)")
	assert.True(t, called, "metadataFn should've been called after going live")

	mmw.SetLive(ctx, false)
	assert.False(t, mmw.GetLive(), "value should be false after SetLive(..., false)")
}

func TestMountMetadataWriterSendMetadata(t *testing.T) {
	var mmw MountMetadataWriter
	ctx := context.Background()

	var called bool
	var calledValue string

	mmw.metadataFn = func(ctx context.Context, s string) error {
		calledValue = s
		called = true
		return nil
	}

	meta := Metadata{
		Value: "some testing data",
	}

	// we're not live so this should not call metadataFn
	mmw.SendMetadata(ctx, &meta)
	assert.False(t, called, "metadataFn should've not been called")
	assert.Zero(t, calledValue)
	assert.Equal(t, meta.Value, mmw.Metadata, "SendMetadata should've stored the metadata")

	mmw.SetLive(ctx, true)
	assert.True(t, called, "metadataFn should've been called after going live")
	assert.Equal(t, meta.Value, calledValue)
}

type adjustPriorityTestCase struct {
	name     string
	sources  []*MountSourceClient
	expected []uint32
}

func TestMountAdjustPriority(t *testing.T) {
	// helper functions to create MountSourceClient in the test cases
	prio := func(p uint32) *MountSourceClient {
		return &MountSourceClient{
			Source: &SourceClient{
				ID: radio.SourceID{xid.New()},
			},
			Priority: p,
		}
	}
	prioSlice := func(ps ...uint32) []*MountSourceClient {
		var sources = make([]*MountSourceClient, 0, len(ps))
		for _, p := range ps {
			sources = append(sources, prio(p))
		}
		return sources
	}

	prioCase := func(name string, sources []*MountSourceClient, expected []uint32) adjustPriorityTestCase {
		return adjustPriorityTestCase{
			name:     name,
			sources:  sources,
			expected: expected,
		}
	}

	testCases := []adjustPriorityTestCase{
		{"empty", prioSlice(), nil},
		{"nil", nil, nil},
		prioCase("simple gaps", prioSlice(5, 10, 15, 20), []uint32{0, 1, 2, 3}),
		prioCase("simple sequential", prioSlice(0, 1, 2, 3, 4, 5), []uint32{0, 1, 2, 3, 4, 5}),
		prioCase("reversed", prioSlice(10, 9, 8, 7, 6, 5, 4, 3, 2, 1, 0), []uint32{0, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10}),
		prioCase("random", prioSlice(8, 2, 1, 7, 4, 5, 0, 10, 9, 3, 6), []uint32{0, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10}),
		prioCase("large gaps", prioSlice(500, 1510, 11215, 122320), []uint32{0, 1, 2, 3}),
	}
	for _, c := range testCases {
		t.Run(c.name, func(t *testing.T) {
			adjustPriority(c.sources)

			for i, s := range c.sources {
				if !assert.Equal(t, c.expected[i], s.Priority) {
					t.Log(c.expected[i], s.Priority)
				}
			}
		})
	}
}
