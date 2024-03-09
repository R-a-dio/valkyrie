package proxy

import (
	"testing"

	"github.com/rs/xid"
	"github.com/stretchr/testify/assert"
)

func TestLeastPriority(t *testing.T) {
	// helper functions to create MountSourceClient in the test cases
	prio := func(p uint) *MountSourceClient {
		return &MountSourceClient{
			Priority: p,
		}
	}
	prioSlice := func(ps ...uint) []*MountSourceClient {
		var sources = make([]*MountSourceClient, 0, len(ps))
		for _, p := range ps {
			sources = append(sources, prio(p))
		}
		return sources
	}

	testCases := []struct {
		name     string
		sources  []*MountSourceClient
		expected uint
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
	prio := func(p uint) *MountSourceClient {
		return &MountSourceClient{
			Source: &SourceClient{
				ID: SourceID{xid.New()},
			},
			Priority: p,
		}
	}
	prioSlice := func(ps ...uint) []*MountSourceClient {
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
