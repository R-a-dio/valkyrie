package storagetest

import (
	"database/sql"
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

func (suite *Suite) TestTransactionNestedRollback(t *testing.T) {
	s := suite.Storage(t)
	// create our actual transaction
	ss, tx, err := s.SessionsTx(suite.ctx, nil)
	require.NoError(t, err)

	session := radio.Session{
		Token:  "transaction rollback test token",
		Expiry: time.Now(),
		Data:   []byte("transaction rollback test data"),
	}

	err = ss.Save(session)
	require.NoError(t, err)

	// now before we test rollback run a normal nested tx, this one shouldn't be allowed
	// to commit the transaction since it isn't the owner of it
	{
		ss, tx, err := s.SessionsTx(suite.ctx, tx)
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

		// the commit above should be 'fake' and not actually commit anything
		// so we expect nothing to be retrieved by this Get
		_, err = s.Sessions(suite.ctx).Get(session.Token)
		require.True(t, errors.Is(errors.SessionUnknown, err))
	}

	err = tx.Rollback()
	require.NoError(t, err)

	_, err = s.Sessions(suite.ctx).Get(session.Token)
	require.True(t, errors.Is(errors.SessionUnknown, err))
}

func (suite *Suite) TestTransactionNested(t *testing.T) {
	s := suite.Storage(t)

	t.Run("fake commit into rollback", func(t *testing.T) {
		// create our actual transaction
		_, tx, err := s.SessionsTx(suite.ctx, nil)
		require.NoError(t, err)
		// now use it nested, this should make tx2 a "fake" that isn't
		// allowed to Commit
		_, tx2, err := s.SessionsTx(suite.ctx, tx)
		require.NoError(t, err)

		// test nested
		require.NoError(t, tx2.Commit())
		require.ErrorIs(t, tx2.Rollback(), sql.ErrTxDone, "rollback after commit should return ErrTxDone")
		// test actual
		require.NoError(t, tx.Rollback())
		require.ErrorIs(t, tx.Commit(), sql.ErrTxDone, "commit after rollback should return ErrTxDone")
	})

	t.Run("fake rollback into commit", func(t *testing.T) {
		// create our actual transaction
		_, tx, err := s.SessionsTx(suite.ctx, nil)
		require.NoError(t, err)
		// now use it nested, this should make tx2 a "fake" that isn't
		// allowed to Commit
		_, tx2, err := s.SessionsTx(suite.ctx, tx)
		require.NoError(t, err)

		// test nested
		require.NoError(t, tx2.Rollback(), "rollback should succeed from fake")
		// test actual
		require.ErrorIs(t, tx.Commit(), sql.ErrTxDone, "commit should error after rollback")
	})

	t.Run("commit", func(t *testing.T) {
		// create our actual transaction
		_, tx, err := s.SessionsTx(suite.ctx, nil)
		require.NoError(t, err)
		// now use it nested, this should make tx2 a "fake" that isn't
		// allowed to Commit
		_, tx2, err := s.SessionsTx(suite.ctx, tx)
		require.NoError(t, err)

		// test nested
		require.NoError(t, tx2.Commit())
		// test actual
		require.NoError(t, tx.Commit())
	})
}
