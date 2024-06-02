package storagetest

import (
	"testing"

	radio "github.com/R-a-dio/valkyrie"
	"github.com/R-a-dio/valkyrie/errors"
	"github.com/stretchr/testify/require"
)

func capAt(s string, n int) string {
	if len(s) > n {
		return s[:n]
	}
	return s
}

func (suite *Suite) TestSubmissionInsertPostPending(t *testing.T) {
	s := suite.Storage(t)
	ss := s.Submissions(suite.ctx)

	a := newArbitrary()
	song := generate[radio.PendingSong](a)

	song.UserIdentifier = capAt(song.UserIdentifier, 50)
	song.Format = capAt(song.Format, 10)
	song.EncodingMode = capAt(song.EncodingMode, 10)

	err := ss.InsertPostPending(song)
	require.NoError(t, err)
}

func (suite *Suite) TestSubmissionRemove(t *testing.T) {
	s := suite.Storage(t)
	ss := s.Submissions(suite.ctx)

	err := ss.RemoveSubmission(0)
	require.NoError(t, err)
}

func (suite *Suite) TestSubmissionLastTime(t *testing.T) {
	s := suite.Storage(t)
	ss := s.Submissions(suite.ctx)

	res, err := ss.LastSubmissionTime("noexist")
	require.NoError(t, err)
	require.True(t, res.IsZero(), "returned time (%v) should be zero", res)
}

func (suite *Suite) TestSubmissionUpdateTime(t *testing.T) {
	s := suite.Storage(t)
	ss := s.Submissions(suite.ctx)

	err := ss.UpdateSubmissionTime("noexist")
	require.NoError(t, err)
}

func (suite *Suite) TestSubmissionStats(t *testing.T) {
	s := suite.Storage(t)
	ss := s.Submissions(suite.ctx)

	_, err := ss.SubmissionStats("noexist")
	require.NoError(t, err)
}

func (suite *Suite) TestSubmissionInsert(t *testing.T) {
	s := suite.Storage(t)
	ss := s.Submissions(suite.ctx)

	a := newArbitrary()
	song := generate[radio.PendingSong](a)
	song.UserIdentifier = capAt(song.UserIdentifier, 50)
	song.Format = capAt(song.Format, 10)
	song.EncodingMode = capAt(song.EncodingMode, 10)

	err := ss.InsertSubmission(song)
	require.NoError(t, err)
}

func (suite *Suite) TestSubmissionAll(t *testing.T) {
	s := suite.Storage(t)
	ss := s.Submissions(suite.ctx)

	res, err := ss.All()
	require.NoError(t, err)
	require.Nil(t, res)
}

func (suite *Suite) TestSubmissionGet(t *testing.T) {
	s := suite.Storage(t)
	ss := s.Submissions(suite.ctx)

	res, err := ss.GetSubmission(1)
	require.True(t, errors.Is(errors.SubmissionUnknown, err), "error should be unknown: %v", err)
	require.Nil(t, res)
}
