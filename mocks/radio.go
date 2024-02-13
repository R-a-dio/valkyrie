package mocks

import (
	"testing"

	radio "github.com/R-a-dio/valkyrie"
	"github.com/R-a-dio/valkyrie/errors"
)

// RollbackTx is a helper function to create a mocked StorageTx
// that expects to be rolled back and not have Commit called
func RollbackTx(t *testing.T) radio.StorageTx {
	return &StorageTxMock{
		RollbackFunc: func() error { return nil },
	}
}

// CommitTx is a helper function to create a mocked StorageTx
// that expects to be committed, it errors if a Rollback occurs
// before a Commit
func CommitTx(t *testing.T) radio.StorageTx {
	var commitCalled bool

	return &StorageTxMock{
		RollbackFunc: func() error {
			if !commitCalled {
				t.Error("rollback called before commit")
			}
			return nil
		},
		CommitFunc: func() error {
			commitCalled = true
			return nil
		},
	}
}

// CommitErrTx is a helper function to create a mocked StorageTx
// that has the Commit return an error
func CommitErrTx(t *testing.T) radio.StorageTx {
	return &StorageTxMock{
		RollbackFunc: func() error {
			return nil
		},
		CommitFunc: func() error {
			return errors.E(errors.Testing)
		},
	}
}

// NotUsedTx is a mocked StorageTx that doesn't expect to be used at all
func NotUsedTx(t *testing.T) radio.StorageTx {
	return new(StorageTxMock)
}
