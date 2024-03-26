package storagetest

import (
	"testing"
	"time"

	radio "github.com/R-a-dio/valkyrie"
	"github.com/R-a-dio/valkyrie/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func (suite *Suite) TestTransactionCommit(t *testing.T) {
	s := suite.Storage(t)
	ss, tx, err := s.SessionsTx(suite.ctx, nil)
	require.NoError(t, err)

	session := radio.Session{
		Token:  "transaction commit test token",
		Expiry: time.Now(),
		Data:   []byte("transaction commit test data"),
	}

	err = ss.Save(session)
	require.NoError(t, err)

	err = tx.Commit()
	require.NoError(t, err)

	got, err := s.Sessions(suite.ctx).Get(session.Token)
	if assert.NoError(t, err) {
		assert.Equal(t, session.Token, got.Token)
		assert.WithinDuration(t, session.Expiry, got.Expiry, time.Second)
		assert.Equal(t, session.Data, got.Data)
	}
}

func (suite *Suite) TestTransactionRollback(t *testing.T) {
	s := suite.Storage(t)
	ss, tx, err := s.SessionsTx(suite.ctx, nil)
	require.NoError(t, err)

	session := radio.Session{
		Token:  "transaction rollback test token",
		Expiry: time.Now(),
		Data:   []byte("transaction rollback test data"),
	}

	err = ss.Save(session)
	require.NoError(t, err)

	err = tx.Rollback()
	require.NoError(t, err)

	_, err = s.Sessions(suite.ctx).Get(session.Token)
	require.True(t, errors.Is(errors.SessionUnknown, err))
}
