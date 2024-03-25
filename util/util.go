package util

import (
	"context"
	"net/http"
	"net/url"
	"sync/atomic"
	"time"

	"github.com/R-a-dio/valkyrie/util/eventstream"
	"github.com/rs/zerolog"
)

// IsHTMX checks if a request was made by HTMX through the Hx-Request header
func IsHTMX(r *http.Request) bool {
	return r.Header.Get("Hx-Request") == "true"
}

func RedirectBack(r *http.Request) *http.Request {
	current, err := url.Parse(r.Header.Get("Hx-Current-Url"))
	if err != nil {
		r.URL = current
	} else {
		current, err = url.Parse(r.Header.Get("Referer"))
		if err == nil {
			r.URL = current
		}
	}
	return r
}

type StreamFn[T any] func(context.Context) (eventstream.Stream[T], error)

type StreamCallbackFn[T any] func(context.Context, T)

// OneOff creates a stream through fn and returns the first value received after which
// it closes the stream. Should be used where you only need a very sporadic value that is
// supplied by a streaming API.
func OneOff[T any](ctx context.Context, fn StreamFn[T]) (T, error) {
	s, err := fn(ctx)
	if err != nil {
		return *new(T), err
	}
	defer s.Close()

	return s.Next()
}

// StreamValue opens the stream created by StreamFn and calls any callbackFn given everytime a new
// value is returned by the stream. StreamValue also stores the last received value, accessable by
// calling .Latest
func StreamValue[T any](ctx context.Context, fn StreamFn[T], callbackFn ...StreamCallbackFn[T]) *Value[T] {
	var value Value[T]
	value.last.Store(new(T))

	go func() {
		for {
			stream, err := fn(ctx)
			if err != nil {
				// stream creation error most likely means the service
				// is down or unavailable for some reason so retry in
				// a little bit and stay alive
				zerolog.Ctx(ctx).Error().Err(err).Msg("stream-value: stream error")
				select {
				case <-ctx.Done():
					// context was canceled, either while we were waiting on
					// retrying, or that was our original error and we exit
					return
				case <-time.After(time.Second):
				}
				continue
			}

			for {
				v, err := stream.Next()
				if err != nil {
					// we either got context canceled or received some
					// stream error that indicates we need a new stream,
					// try and get one from the outer loop.
					zerolog.Ctx(ctx).Error().Err(err).Msg("stream-value: next error")
					break
				}

				value.last.Store(&v)

				for _, callback := range callbackFn {
					callback(ctx, v)
				}
			}
			stream.Close()
		}
	}()

	return &value
}

type Value[T any] struct {
	last atomic.Pointer[T]
}

func (v *Value[T]) Latest() T {
	return *v.last.Load()
}
