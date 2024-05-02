package tracker

import (
	"context"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/R-a-dio/valkyrie/telemetry"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

type ClientID uint64

func ParseClientID(s string) (ClientID, error) {
	id, err := strconv.ParseUint(s, 10, 64)
	return ClientID(id), err
}

func (c ClientID) String() string {
	return strconv.FormatUint(uint64(c), 10)
}

type Listener struct {
	span trace.Span

	// ID is the identifier icecast is using for this client
	ID ClientID
	// Start is the time this listener started listening
	Start time.Time
	// Info is the information icecast sends us through the POST form values
	Info url.Values
}

func NewRecorder() *Recorder {
	return &Recorder{
		pendingRemoval: make(map[ClientID]time.Time),
		listeners:      make(map[ClientID]*Listener),
	}
}

type Recorder struct {
	mu             sync.Mutex
	pendingRemoval map[ClientID]time.Time
	listeners      map[ClientID]*Listener
	listenerAmount atomic.Int64
}

func (r *Recorder) ListenerAmount() int64 {
	return r.listenerAmount.Load()
}

func (r *Recorder) ListenerAdd(ctx context.Context, id ClientID, req *http.Request) {
	_, span := otel.Tracer("listener-tracker").Start(ctx, "listener",
		trace.WithNewRoot(),
		trace.WithAttributes(requestToOtelAttributes(req)...),
	)

	listener := Listener{
		ID:    id,
		span:  span,
		Start: time.Now(),
		Info:  req.PostForm,
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

func (r *Recorder) ListenerRemove(ctx context.Context, id ClientID, req *http.Request) {
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

func requestToOtelAttributes(req *http.Request) []attribute.KeyValue {
	res := telemetry.HeadersToAttributes(req.Header)
	for name, value := range req.PostForm {
		res = append(res, attribute.StringSlice(strings.ToLower(name), value))
	}
	return res
}
