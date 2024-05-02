package tracker

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestListenerAddAndRemoval(t *testing.T) {
	r := NewRecorder()
	ctx := context.Background()
	req := httptest.NewRequest(http.MethodGet, "/test", nil)

	count := ClientID(200)
	for i := range count {
		go r.ListenerAdd(ctx, i, req)
	}

	assert.Eventually(t, func() bool {
		return int64(count) == r.ListenerAmount()
	}, time.Second*5, time.Millisecond*50)

	for i := range count {
		go r.ListenerRemove(ctx, i, req)
	}

	assert.Eventually(t, func() bool {
		return 0 == r.ListenerAmount()
	}, time.Second*5, time.Millisecond*50)

	assert.Len(t, r.listeners, 0)
	assert.Len(t, r.pendingRemoval, 0)
}

func TestListenerAddAndRemovalOutOfOrder(t *testing.T) {
	eventuallyTick := time.Millisecond * 150
	eventuallyDelay := time.Second * 5

	r := NewRecorder()
	ctx := context.Background()
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
			assert.Len(t, r.listeners, int(count/2))
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
		r.mu.Lock()
		defer r.mu.Unlock()
		return assert.Zero(t, r.ListenerAmount()) &&
			assert.Len(t, r.listeners, 0)
	}, eventuallyDelay, eventuallyTick)

	assert.Eventually(t, func() bool {
		r.mu.Lock()
		defer r.mu.Unlock()
		return assert.Len(t, r.pendingRemoval, 0)
	}, eventuallyDelay, eventuallyTick)
}
