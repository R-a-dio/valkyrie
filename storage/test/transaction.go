package storagetest

import (
	"time"

	radio "github.com/R-a-dio/valkyrie"
	"github.com/R-a-dio/valkyrie/errors"
)

func (suite *Suite) TestTransactionCommit() {
	ss, tx, err := suite.Storage.SessionsTx(suite.ctx, nil)
	suite.NoError(err)

	session := radio.Session{
		Token:  "transaction commit test token",
		Expiry: time.Now(),
		Data:   []byte("transaction commit test data"),
	}

	err = ss.Save(session)
	suite.NoError(err)

	err = tx.Commit()
	suite.NoError(err)

	got, err := suite.Storage.Sessions(suite.ctx).Get(session.Token)
	if suite.NoError(err) {
		suite.Equal(session.Token, got.Token)
		suite.WithinDuration(session.Expiry, got.Expiry, time.Second)
		suite.Equal(session.Data, got.Data)
	}
}

func (suite *Suite) TestTransactionRollback() {
	ss, tx, err := suite.Storage.SessionsTx(suite.ctx, nil)
	suite.NoError(err)

	session := radio.Session{
		Token:  "transaction rollback test token",
		Expiry: time.Now(),
		Data:   []byte("transaction rollback test data"),
	}

	err = ss.Save(session)
	suite.NoError(err)

	err = tx.Rollback()
	suite.NoError(err)

	_, err = suite.Storage.Sessions(suite.ctx).Get(session.Token)
	suite.True(errors.Is(errors.SessionUnknown, err))
}
