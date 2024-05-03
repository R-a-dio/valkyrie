package tracker

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

const (
	eventuallyTick  = time.Millisecond * 150
	eventuallyDelay = time.Second * 5
)

func TestListenerAddAndRemoval(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	r := NewRecorder(ctx)

	req := httptest.NewRequest(http.MethodGet, "/test", nil)

	count := ClientID(200)
	for i := range count {
		go r.ListenerAdd(ctx, i, req)
	}

	assert.Eventually(t, func() bool {
		return int64(count) == r.ListenerAmount()
	}, eventuallyDelay, eventuallyTick)

	for i := range count {
		go r.ListenerRemove(ctx, i, req)
	}

	assert.Eventually(t, func() bool {
		return 0 == r.ListenerAmount()
	}, eventuallyDelay, eventuallyTick)

	assert.Len(t, r.listeners, 0)
	assert.Len(t, r.pendingRemoval, 0)
}

func TestListenerAddAndRemovalOutOfOrder(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	r := NewRecorder(ctx)

	req := httptest.NewRequest(http.MethodGet, "/test", nil)

	count := int64(200)
	for i := range ClientID(count) {
		if i%2 == 0 {
			go r.ListenerAdd(ctx, i, req)
		} else {
			go r.ListenerRemove(ctx, i, req)
		}
	}

	assert.Eventually(t, func() bool {
		// half should have been added normally
		return assert.Equal(t, count/2, r.ListenerAmount()) &&
			testListenerLength(t, r, int(count/2))
	}, eventuallyDelay, eventuallyTick)
	assert.Eventually(t, func() bool {
		// half should have been removed early
		r.mu.Lock()
		defer r.mu.Unlock()
		return assert.Len(t, r.pendingRemoval, int(count/2))
	}, eventuallyDelay, eventuallyTick)

	// now do the inverse and we should end up with nothing
	for i := range ClientID(count) {
		if i%2 != 0 {
			go r.ListenerAdd(ctx, i, req)
		} else {
			go r.ListenerRemove(ctx, i, req)
		}
	}

	assert.Eventually(t, func() bool {
		return assert.Zero(t, r.ListenerAmount()) &&
			testListenerLength(t, r, 0)
	}, eventuallyDelay, eventuallyTick)

	assert.Eventually(t, func() bool {
		return testPendingLength(t, r, 0)
	}, eventuallyDelay, eventuallyTick)
}

func testListenerLength(t *testing.T, r *Recorder, expected int) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	return assert.Len(t, r.listeners, expected)
}

func testPendingLength(t *testing.T, r *Recorder, expected int) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	return assert.Len(t, r.pendingRemoval, expected)
}

func testCtx(t *testing.T, ctxx ...context.Context) (ctx context.Context) {
	if len(ctxx) == 0 {
		ctx = context.Background()
	} else {
		ctx = ctxx[0]
	}

	ctx, cancel := context.WithCancel(ctx)
	t.Cleanup(cancel)
	return ctx
}

func TestRecorderRemoveStalePending(t *testing.T) {

	t.Run("simple removal", func(t *testing.T) {
		ctx := testCtx(t)
		r := NewRecorder(ctx)

		id := ClientID(10)
		r.mu.Lock()
		r.pendingRemoval[id] = time.Now().Add(-RemoveStalePendingPeriod)
		r.mu.Unlock()

		found := r.removeStalePending()
		assert.Equal(t, 1, found)

		testPendingLength(t, r, 0)
	})
	t.Run("many removal", func(t *testing.T) {
		ctx := testCtx(t)
		r := NewRecorder(ctx)

		count := RemoveStalePendingPeriod / time.Second * 2
		r.mu.Lock()
		for i := range ClientID(count) {
			r.pendingRemoval[i] = time.Now().Add(-time.Second * time.Duration(i))
		}
		r.mu.Unlock()

		testPendingLength(t, r, int(count))
		found := r.removeStalePending()
		testPendingLength(t, r, int(count/2))
		assert.Equal(t, int(count/2), found)
	})
	t.Run("removal by periodic goroutine", func(t *testing.T) {
		ctx := testCtx(t)
		r := NewRecorder(ctx)

		id := ClientID(10)
		r.mu.Lock()
		r.pendingRemoval[id] = time.Now().Add(-RemoveStalePendingPeriod)
		r.mu.Unlock()

		// launch an extra period goroutine, since the one we create
		// in NewRecorder is very slow
		go r.PeriodicallyRemoveStalePending(ctx, eventuallyTick)

		assert.Eventually(t, func() bool {
			return testPendingLength(t, r, 0)
		}, eventuallyDelay, eventuallyTick)
	})
}
