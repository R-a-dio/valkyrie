package tracker

import (
	"context"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/R-a-dio/valkyrie/telemetry"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

type Listener struct {
	id   string
	span trace.Span
}

func NewRecorder() *Recorder {
	return &Recorder{
		Listeners: make(map[string]Listener),
	}
}

type Recorder struct {
	mu             sync.Mutex
	Listeners      map[string]Listener
	ListenerAmount atomic.Int64
}

func (r *Recorder) ListenerAdd(ctx context.Context, req *http.Request) {
	defer r.ListenerAmount.Add(1)
	id := req.FormValue("client")

	_, span := otel.Tracer("listener-tracker").Start(ctx, "listener",
		trace.WithNewRoot(),
		trace.WithAttributes(requestToOtelAttributes(req)...),
	)

	listener := Listener{
		id:   id,
		span: span,
	}

	r.mu.Lock()
	r.Listeners[listener.id] = listener
	r.mu.Unlock()
}

func (r *Recorder) ListenerRemove(ctx context.Context, req *http.Request) {
	defer r.ListenerAmount.Add(-1)
	id := req.FormValue("client")

	r.mu.Lock()
	listener, ok := r.Listeners[id]
	delete(r.Listeners, id)
	r.mu.Unlock()

	if ok {
		listener.span.End()
	}
}

func requestToOtelAttributes(req *http.Request) []attribute.KeyValue {
	res := telemetry.HeadersToAttributes(req.Header)
	for name, value := range req.PostForm {
		res = append(res, attribute.StringSlice(strings.ToLower(name), value))
	}
	return res
}
