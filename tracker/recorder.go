package tracker

import (
	"context"
	"net"
	"net/http"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	radio "github.com/R-a-dio/valkyrie"
	"github.com/R-a-dio/valkyrie/telemetry"
	"github.com/rs/zerolog"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

type Listener struct {
	span trace.Span

	radio.Listener
}

func NewRecorder(ctx context.Context) *Recorder {
	r := &Recorder{
		pendingRemoval: make(map[radio.ListenerClientID]time.Time),
		listeners:      make(map[radio.ListenerClientID]*Listener),
	}
	go r.PeriodicallyRemoveStalePending(ctx, RemoveStalePendingTickrate)
	return r
}

type Recorder struct {
	mu             sync.Mutex
	pendingRemoval map[radio.ListenerClientID]time.Time
	listeners      map[radio.ListenerClientID]*Listener
	listenerAmount atomic.Int64
}

func (r *Recorder) PeriodicallyRemoveStalePending(ctx context.Context, tickrate time.Duration) {
	ticker := time.NewTicker(tickrate)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			stale := r.removeStalePending()
			if stale > 0 {
				zerolog.Ctx(ctx).Error().Int("amount", stale).Msg("found stale pending removals")
			}
		}
	}
}

func (r *Recorder) removeStalePending() (found_stale int) {
	deadline := time.Now().Add(-RemoveStalePendingPeriod)

	r.mu.Lock()
	defer r.mu.Unlock()
	for k, v := range r.pendingRemoval {
		if v.Before(deadline) {
			delete(r.pendingRemoval, k)
			found_stale++
		}
	}
	return found_stale
}

func (r *Recorder) ListenerAmount() int64 {
	return r.listenerAmount.Load()
}

func (r *Recorder) ListenerAdd(ctx context.Context, id radio.ListenerClientID, req *http.Request) {
	_, span := otel.Tracer("listener-tracker").Start(ctx, "listener",
		trace.WithNewRoot(),
		trace.WithAttributes(requestToOtelAttributes(req)...),
	)

	listener := Listener{
		span: span,
		Listener: radio.Listener{
			ID:        id,
			UserAgent: req.PostFormValue("agent"), // passed by icecast
			Start:     time.Now(),
			IP:        IcecastRealIP(req),
		},
	}

	var ok bool
	r.mu.Lock()
	if _, ok = r.pendingRemoval[listener.ID]; !ok {
		r.listeners[listener.ID] = &listener
	} else {
		delete(r.pendingRemoval, listener.ID)
	}
	r.mu.Unlock()

	if !ok {
		r.listenerAmount.Add(1)
	}
}

func (r *Recorder) ListenerRemove(ctx context.Context, id radio.ListenerClientID, req *http.Request) {
	var listener *Listener
	var ok bool

	r.mu.Lock()
	if listener, ok = r.listeners[id]; ok {
		delete(r.listeners, id)
	} else {
		r.pendingRemoval[id] = time.Now()
	}
	r.mu.Unlock()

	if ok {
		listener.span.End()
		r.listenerAmount.Add(-1)
	}
}

func (r *Recorder) ListClients(ctx context.Context) ([]radio.Listener, error) {
	r.mu.Lock()
	res := make([]radio.Listener, 0, len(r.listeners))
	for _, v := range r.listeners {
		res = append(res, v.Listener)
	}
	r.mu.Unlock()

	slices.SortFunc(res, func(a, b radio.Listener) int {
		return a.Start.Compare(b.Start)
	})
	return res, nil
}

func (r *Recorder) RemoveClient(ctx context.Context, id radio.ListenerClientID) error {
	return nil
}

func requestToOtelAttributes(req *http.Request) []attribute.KeyValue {
	res := telemetry.HeadersToAttributes(req.Header)
	for name, value := range req.PostForm {
		res = append(res, attribute.StringSlice(strings.ToLower(name), value))
	}
	return res
}

const prefix = "client."

var xForwardedFor = strings.ToLower(prefix + "X-Forwarded-For")
var xRealIP = strings.ToLower(prefix + "X-Real-IP")
var trueClientIP = strings.ToLower(prefix + "True-Client-IP")

// icecastRealIP recovers the clients real ip address from the request
//
// This looks for X-Forwarded-For, X-Real-IP and True-Client-IP
func IcecastRealIP(r *http.Request) string {
	var ip string

	if tcip := r.PostForm.Get(trueClientIP); tcip != "" {
		ip = tcip
	} else if xrip := r.PostForm.Get(xRealIP); xrip != "" {
		ip = xrip
	} else if xff := r.PostForm.Get(xForwardedFor); xff != "" {
		i := strings.Index(xff, ",")
		if i == -1 {
			i = len(xff)
		}
		ip = xff[:i]
	}
	if ip == "" || net.ParseIP(ip) == nil {
		return ""
	}
	return ip

}
