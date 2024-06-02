package storagetest

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func (suite *Suite) TestRequestLastRequestAndUpdate(t *testing.T) {
	s := suite.Storage(t)
	rs := s.Request(suite.ctx)

	// should not exist
	res, err := rs.LastRequest("no exist")
	require.NoError(t, err)
	require.True(t, res.IsZero(), "time returned should be zero")

	// add a test
	err = rs.UpdateLastRequest("test")
	require.NoError(t, err)

	// should be able to retrieve it
	res, err = rs.LastRequest("test")
	require.NoError(t, err)
	require.False(t, res.IsZero(), "time returned should not be zero")
	require.WithinDuration(t, time.Now(), res, time.Minute)
	firstRes := res

	// should still not exist
	res, err = rs.LastRequest("no exist")
	require.NoError(t, err)
	require.True(t, res.IsZero(), "time returned should be zero")

	time.Sleep(time.Second) // shit but we run too fast otherwise
	// add a second test
	err = rs.UpdateLastRequest("test")
	require.NoError(t, err)

	// should be able to retrieve it
	res, err = rs.LastRequest("test")
	require.NoError(t, err)
	require.False(t, res.IsZero(), "time returned should not be zero")
	require.True(t, firstRes.Before(res), "first result (%v) should be before second result (%v)", firstRes, res)
	require.WithinDuration(t, time.Now(), res, time.Minute)
}
