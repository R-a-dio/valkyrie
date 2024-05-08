package tracker

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"testing"
	"time"

	radio "github.com/R-a-dio/valkyrie"
	"github.com/leanovate/gopter"
	"github.com/leanovate/gopter/arbitrary"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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
		go r.ListenerAdd(ctx, NewListener(i, req))
	}

	assert.Eventually(t, func() bool {
		return int64(count) == r.ListenerAmount()
	}, eventuallyDelay, eventuallyTick)

	for i := range count {
		go r.ListenerRemove(ctx, i)
	}

	assert.Eventually(t, func() bool {
		return 0 == r.ListenerAmount()
	}, eventuallyDelay, eventuallyTick)

	testRecorderLengths(t, r, 0, 0)
}

func TestListenerMultiRemove(t *testing.T) {
	// test if multiple calls to ListenerRemove can desync our
	// internal state somehow
	fn := func(removalCount int) func(t *testing.T) {
		return func(t *testing.T) {
			t.Parallel()

			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			r := NewRecorder(ctx)

			req := httptest.NewRequest(http.MethodGet, "/test", nil)

			count := radio.ListenerClientID(200)
			for i := range count {
				go r.ListenerAdd(ctx, NewListener(i, req))
			}

			// wait for all the goroutines above to actually have added
			// their listener
			assert.Eventually(t, func() bool {
				return int64(count) == r.ListenerAmount()
			}, eventuallyDelay, eventuallyTick)

			for i := range count {
				// now remove listeners multiple times
				for range removalCount {
					go r.ListenerRemove(ctx, i)
				}
			}

			// wait for the goroutines to finish running
			ok := assert.Eventually(t, func() bool {
				return 0 == r.ListenerAmount()
			}, eventuallyDelay, eventuallyTick)
			if !ok {
				active, removed := getRecorderLength(r)
				t.Log("active", active, "removed", removed, "listener-count", r.ListenerAmount())
			}
			// remove all entries marked removed
			r.removeStalePending(0)
			// make sure we're back to 0 listeners and 0 removed entries
			testRecorderLengths(t, r, 0, 0)
		}
	}
	for removeCount := 1; removeCount < 12; removeCount++ {
		t.Run("MultiRemove"+strconv.Itoa(removeCount), fn(removeCount))
	}
}

func TestListenerMultiAdd(t *testing.T) {
	// test if multiple calls to ListenerRemove can desync our
	// internal state somehow
	fn := func(addCount int) func(t *testing.T) {
		return func(t *testing.T) {
			t.Parallel()

			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			r := NewRecorder(ctx)

			req := httptest.NewRequest(http.MethodGet, "/test", nil)

			count := radio.ListenerClientID(200)
			for i := range count {
				for range addCount {
					go r.ListenerAdd(ctx, NewListener(i, req))
				}
			}

			// wait for the goroutines to finish running
			ok := assert.Eventually(t, func() bool {
				return int64(count) == r.ListenerAmount()
			}, eventuallyDelay, eventuallyTick)
			if !ok {
				active, removed := getRecorderLength(r)
				t.Log("active", active, "removed", removed, "listener-count", r.ListenerAmount())
			}

			// make sure we're back to `count` listeners and 0 removed entries
			testRecorderLengths(t, r, int(count), 0)
		}
	}
	for addCount := 1; addCount < 12; addCount++ {
		t.Run("MultiAdd"+strconv.Itoa(addCount), fn(addCount))
	}
}

func BenchmarkRecorderAddAndRemove(b *testing.B) {
	ctx := context.Background()
	r := NewRecorder(ctx)
	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	const idle = 1500
	for i := range radio.ListenerClientID(idle) {
		r.ListenerAdd(ctx, NewListener(i, req))
	}

	b.ResetTimer()
	for n := 0; n < b.N; n++ {
		id := radio.ListenerClientID(n)
		r.ListenerAdd(ctx, NewListener(id+idle, req))
		r.ListenerRemove(ctx, id)
	}
	testRecorderLengths(b, r, idle, 0)
}

func TestListenerAddAndRemovalOutOfOrder(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	r := NewRecorder(ctx)

	req := httptest.NewRequest(http.MethodGet, "/test", nil)

	count := int64(200)
	for i := range radio.ListenerClientID(count) {
		if i%2 == 0 {
			go r.ListenerAdd(ctx, NewListener(i, req))
		} else {
			go r.ListenerRemove(ctx, i)
		}
	}

	assert.Eventually(t, func() bool {
		// half should have been added normally
		return assert.Equal(t, count/2, r.ListenerAmount()) &&
			testRecorderLengths(t, r, int(count/2), int(count/2))
	}, eventuallyDelay, eventuallyTick)

	// now do the inverse and we should end up with nothing
	for i := range radio.ListenerClientID(count) {
		if i%2 != 0 {
			go r.ListenerAdd(ctx, NewListener(i, req))
		} else {
			go r.ListenerRemove(ctx, i)
		}
	}

	assert.Eventually(t, func() bool {
		return assert.Zero(t, r.ListenerAmount()) &&
			testRecorderLengths(t, r, 0, 0)
	}, eventuallyDelay, eventuallyTick)
}

func testListenerLength(t testing.TB, r *Recorder, expected int) bool {
	active, _ := getRecorderLength(r)

	return assert.Equal(t, expected, active)
}

func testRecorderLengths(t testing.TB, r *Recorder, expectedActive, expectedRemoved int) bool {
	active, removed := getRecorderLength(r)

	return assert.Equal(t, expectedActive, active, "active mismatch") &&
		assert.Equal(t, expectedRemoved, removed, "removed mismatch")
}

func testPendingLength(t testing.TB, r *Recorder, expected int) bool {
	_, removed := getRecorderLength(r)

	return assert.Equal(t, expected, removed)
}

func getRecorderLength(r *Recorder) (active, removed int) {
	r.listeners.Range(func(key radio.ListenerClientID, value *Listener) bool {
		if value.Removed {
			removed++
		} else {
			active++
		}
		return true
	})
	return active, removed
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

		r.listeners.Store(id, &Listener{
			Removed:     true,
			RemovedTime: time.Now().Add(-RemoveStalePendingPeriod),
		})

		found := r.removeStalePending(RemoveStalePendingPeriod)
		assert.Equal(t, 1, found)

		testPendingLength(t, r, 0)
	})
	t.Run("many removal", func(t *testing.T) {
		ctx := testCtx(t)
		r := NewRecorder(ctx)

		count := RemoveStalePendingPeriod / time.Second * 2

		for i := range radio.ListenerClientID(count) {
			r.listeners.Store(i, &Listener{
				Removed:     true,
				RemovedTime: time.Now().Add(-time.Second * time.Duration(i)),
			})
		}

		testPendingLength(t, r, int(count))
		found := r.removeStalePending(RemoveStalePendingPeriod)
		testPendingLength(t, r, int(count/2))
		assert.Equal(t, int(count/2), found)
	})
	t.Run("removal by periodic goroutine", func(t *testing.T) {
		ctx := testCtx(t)
		r := NewRecorder(ctx)

		id := radio.ListenerClientID(10)

		r.listeners.Store(id, &Listener{
			Removed:     true,
			RemovedTime: time.Now().Add(-RemoveStalePendingPeriod),
		})

		// launch an extra period goroutine, since the one we create
		// in NewRecorder is very slow
		go r.PeriodicallyRemoveStalePending(ctx, eventuallyTick)

		assert.Eventually(t, func() bool {
			return testPendingLength(t, r, 0)
		}, eventuallyDelay, eventuallyTick*2)
	})
}

func TestIcecastRealIP(t *testing.T) {
	t.Run(xForwardedFor, func(t *testing.T) {
		id := radio.ListenerClientID(50)
		ip := "192.168.1.1"
		values := url.Values{}
		values.Add(xForwardedFor, ip)

		body := strings.NewReader(values.Encode())
		req := httptest.NewRequest(http.MethodPost, "/listener_add", body)
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

		ln := NewListener(id, req)
		assert.Equal(t, ip, ln.IP)
	})
	t.Run(xForwardedFor+"/multiple", func(t *testing.T) {
		id := radio.ListenerClientID(50)
		ip := "192.168.1.1, 203.0.113.195, 70.41.3.18, 150.172.238.178"
		values := url.Values{}
		values.Add(xForwardedFor, ip)

		body := strings.NewReader(values.Encode())
		req := httptest.NewRequest(http.MethodPost, "/listener_add", body)
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

		ln := NewListener(id, req)
		assert.Equal(t, "192.168.1.1", ln.IP)
	})
	t.Run(trueClientIP, func(t *testing.T) {
		id := radio.ListenerClientID(50)
		ip := "192.168.1.1"
		values := url.Values{}
		values.Add(trueClientIP, ip)

		body := strings.NewReader(values.Encode())
		req := httptest.NewRequest(http.MethodPost, "/listener_add", body)
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

		ln := NewListener(id, req)
		assert.Equal(t, ip, ln.IP)
	})
	t.Run(xRealIP, func(t *testing.T) {
		id := radio.ListenerClientID(50)
		ip := "192.168.1.1"
		values := url.Values{}
		values.Add(xRealIP, ip)

		body := strings.NewReader(values.Encode())
		req := httptest.NewRequest(http.MethodPost, "/listener_add", body)
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

		ln := NewListener(id, req)
		assert.Equal(t, ip, ln.IP)
	})
}

func TestRecorderListClients(t *testing.T) {
	a := arbitrary.DefaultArbitraries()
	a.RegisterGen(genListener(a))
	p := gopter.NewProperties(nil)

	ctx := context.Background()

	p.Property(t.Name(), a.ForAll(func(in []radio.Listener) bool {
		// sort entries, this is what we will expect later
		sortListeners(in)

		r := NewRecorder(ctx)
		// add our input by using a sync call
		r.Sync(ctx, in)
		// make sure they got added
		require.Equal(t, int64(len(in)), r.ListenerAmount(), "wrong length after sync")

		// then ask for the list of clients, these should be sorted with sortListeners
		// so we can then compare them easily
		out, err := r.ListClients(ctx)
		require.NoError(t, err)

		var ok = true
		for i := range in {
			ok = ok && assert.Equal(t, in[i].ID, out[i].ID, "mismatching ID")
			ok = ok && assert.Equal(t, in[i].UserAgent, out[i].UserAgent, "mismatching useragent")
			ok = ok && assert.Equal(t, in[i].IP, out[i].IP, "mismatching IP")
			ok = ok && assert.WithinDuration(t, in[i].Start, out[i].Start, time.Minute, "mismatching start time")
		}
		return ok
	}))
	p.TestingRun(t)
}
