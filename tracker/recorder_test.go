package tracker

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	radio "github.com/R-a-dio/valkyrie"
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

	count := radio.ListenerClientID(200)
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
	for i := range radio.ListenerClientID(count) {
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
	for i := range radio.ListenerClientID(count) {
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

		id := radio.ListenerClientID(10)
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
		for i := range radio.ListenerClientID(count) {
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

		id := radio.ListenerClientID(10)
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

func TestIcecastRealIP(t *testing.T) {
	t.Run(xForwardedFor, func(t *testing.T) {
		ctx := testCtx(t)
		r := NewRecorder(ctx)
		id := radio.ListenerClientID(50)
		ip := "192.168.1.1"
		values := url.Values{}
		values.Add(xForwardedFor, ip)

		body := strings.NewReader(values.Encode())
		req := httptest.NewRequest(http.MethodPost, "/listener_add", body)
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

		r.ListenerAdd(ctx, id, req)

		r.mu.Lock()
		assert.Equal(t, ip, r.listeners[id].IP)
		r.mu.Unlock()
	})
	t.Run(xForwardedFor+"/multiple", func(t *testing.T) {
		ctx := testCtx(t)
		r := NewRecorder(ctx)
		id := radio.ListenerClientID(50)
		ip := "192.168.1.1, 203.0.113.195, 70.41.3.18, 150.172.238.178"
		values := url.Values{}
		values.Add(xForwardedFor, ip)

		body := strings.NewReader(values.Encode())
		req := httptest.NewRequest(http.MethodPost, "/listener_add", body)
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

		r.ListenerAdd(ctx, id, req)

		r.mu.Lock()
		assert.Equal(t, "192.168.1.1", r.listeners[id].IP)
		r.mu.Unlock()
	})
	t.Run(trueClientIP, func(t *testing.T) {
		ctx := testCtx(t)
		r := NewRecorder(ctx)
		id := radio.ListenerClientID(50)
		ip := "192.168.1.1"
		values := url.Values{}
		values.Add(trueClientIP, ip)

		body := strings.NewReader(values.Encode())
		req := httptest.NewRequest(http.MethodPost, "/listener_add", body)
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

		r.ListenerAdd(ctx, id, req)

		r.mu.Lock()
		assert.Equal(t, ip, r.listeners[id].IP)
		r.mu.Unlock()
	})
	t.Run(xRealIP, func(t *testing.T) {
		ctx := testCtx(t)
		r := NewRecorder(ctx)
		id := radio.ListenerClientID(50)
		ip := "192.168.1.1"
		values := url.Values{}
		values.Add(xRealIP, ip)

		body := strings.NewReader(values.Encode())
		req := httptest.NewRequest(http.MethodPost, "/listener_add", body)
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

		r.ListenerAdd(ctx, id, req)

		r.mu.Lock()
		assert.Equal(t, ip, r.listeners[id].IP)
		r.mu.Unlock()
	})
}
