package tracker

import (
	"context"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/R-a-dio/valkyrie/telemetry"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

type Listener struct {
	span trace.Span

	// ID is the identifier icecast is using for this client
	ID string
	// Start is the time this listener started listening
	Start time.Time
	// Info is the information icecast sends us through the POST form values
	Info url.Values
}

func NewRecorder() *Recorder {
	return &Recorder{
		Listeners: make(map[string]*Listener),
	}
}

type Recorder struct {
	mu             sync.Mutex
	Listeners      map[string]*Listener
	ListenerAmount atomic.Int64
}

func (r *Recorder) ListenerAdd(ctx context.Context, id string, req *http.Request) {
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

	r.mu.Lock()
	r.Listeners[listener.ID] = &listener
	r.mu.Unlock()

	r.ListenerAmount.Add(1)
}

func (r *Recorder) ListenerRemove(ctx context.Context, id string, req *http.Request) {
	r.mu.Lock()
	listener, ok := r.Listeners[id]
	delete(r.Listeners, id)
	r.mu.Unlock()

	if ok {
		listener.span.End()
		r.ListenerAmount.Add(-1)
	}
}

func requestToOtelAttributes(req *http.Request) []attribute.KeyValue {
	res := telemetry.HeadersToAttributes(req.Header)
	for name, value := range req.PostForm {
		res = append(res, attribute.StringSlice(strings.ToLower(name), value))
	}
	return res
}
