package graceful

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestFinishSync(t *testing.T) {
	ctx, _ := TestGraceful(context.Background(), false)

	// call Finish on it and see if we have Sync working
	Finish(ctx)

	select {
	case <-Sync(ctx):
	default:
		t.Error("Sync should return a closed channel after Finish call")
	}
}

func TestSyncNoGraceful(t *testing.T) {
	// new context with no graceful
	ctx := context.Background()

	select {
	case <-Sync(ctx):
	default:
		t.Error("Sync should return closed channel if no graceful exists")
	}
}

func TestGracefulSignal(t *testing.T) {
	ctx, g := TestGraceful(context.Background(), false)

	assert.Equal(t, g.Signal, Signal(ctx),
		"channel returned by Signal should be equal to g.Signal")
	assert.Nil(t, Signal(context.Background()),
		"Signal with empty context should return nil")
}

func TestParentTestParent(t *testing.T) {
	ctx, g := TestGraceful(context.Background(), true)

	conn, err := Parent(ctx)
	assert.NoError(t, err)
	assert.NotNil(t, conn)
	assert.Equal(t, g.Parent, conn)
}

func TestParentNoChild(t *testing.T) {
	ctx := context.Background()
	ctx, _ = TestGraceful(ctx, false)

	conn, err := Parent(ctx)
	assert.Error(t, err)
	assert.Nil(t, conn)
}

func TestParentNoGraceful(t *testing.T) {
	ctx := context.Background()

	conn, err := Parent(ctx)
	assert.Error(t, err)
	assert.Nil(t, conn)
}
