package telemetry

import (
	"context"
	"net/http"
	"reflect"
	"runtime"
	"strings"

	"github.com/go-chi/chi/v5"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"go.opentelemetry.io/otel/trace"
	"go.opentelemetry.io/otel/trace/noop"
)

func cleanFunctionName(str string) string {
	var index int

	strs := strings.Split(str, ".")
	for index = len(strs) - 1; index > 0; index-- {
		if !strings.HasPrefix(strs[index], "func") {
			break
		}
	}
	return strings.TrimSuffix(strs[index], "-fm")
}

func getFunctionName(temp any) string {
	name := runtime.FuncForPC(reflect.ValueOf(temp).Pointer()).Name()
	return cleanFunctionName(name)
}

type spanKey struct{}

type middlewareSpan struct {
	current   trace.Span
	parent    trace.Span
	parentCtx context.Context
}

func getSpan(ctx context.Context) *middlewareSpan {
	if v := ctx.Value(spanKey{}); v != nil {
		return v.(*middlewareSpan)
	}

	return &middlewareSpan{
		current:   trace.Span(noop.Span{}),
		parent:    trace.Span(noop.Span{}),
		parentCtx: context.Background(),
	}
}

func middlewareRecord(nextMiddleware func(http.Handler) http.Handler) func(http.Handler) http.Handler {
	name := getFunctionName(nextMiddleware)

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ms := getSpan(r.Context())
			// end the span of the previous middleware
			ms.current.End()

			// start our next span
			_, ms.current = ms.parent.TracerProvider().Tracer(name).Start(ms.parentCtx, name)

			// we're passing this span ourselves through ms.current so we don't modify the
			// requests context, otherwise we get all the middleware spans as childs of each other
			next.ServeHTTP(w, r)
		})
	}
}

func middlewareRecordInit(nextMiddleware func(http.Handler) http.Handler) func(http.Handler) http.Handler {
	name := getFunctionName(nextMiddleware)

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ctx := r.Context()
			parentSpan := trace.SpanFromContext(ctx)

			_, span := parentSpan.TracerProvider().Tracer(name).Start(ctx, name)

			ctx = context.WithValue(ctx, spanKey{}, &middlewareSpan{
				current:   span,
				parent:    parentSpan,
				parentCtx: ctx,
			})
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

func middlewareRecordFinish(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		ms := getSpan(ctx)
		ms.current.End()

		ms.parent.AddEvent("middleware_finished")

		next.ServeHTTP(w, r)
	})
}

func useMethodPath(operation string, r *http.Request) string {
	return r.Method + " " + r.URL.Path
}

// returns false if request path is /v1/sse, /admin/booth/sse or has the prefix /admin/telemetry
func filterBevin(r *http.Request) bool {
	input := r.URL.Path

	// 15  6  15
	var l = len(input)

	if l >= 1 && input[0] == '/' {
		input = input[1:]
		l -= 1
	}
	if l == 6 && input == "v1/sse" {
		return false
	}
	if l == 15 && input == "admin/booth/sse" {
		return false
	}
	if l >= 15 && input[:15] == "admin/telemetry" {
		return false
	}

	return true
}

func NewRouter() chi.Router {
	r := chi.NewRouter()
	r.Use(otelhttp.NewMiddleware("http_request",
		otelhttp.WithSpanNameFormatter(useMethodPath),
		otelhttp.WithFilter(filterBevin),
	))
	return &router{r, true}
}

type router struct {
	r     chi.Router
	first bool
}

func (r router) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	r.r.ServeHTTP(w, req)
}

// Routes returns the routing tree in an easily traversable structure.
func (r router) Routes() []chi.Route {
	return r.r.Routes()
}

// Middlewares returns the list of middlewares in use by the router.
func (r router) Middlewares() chi.Middlewares {
	return r.r.Middlewares()
}

// Match searches the routing tree for a handler that matches
// the method/path - similar to routing a http request, but without
// executing the handler thereafter.
func (r router) Match(rctx *chi.Context, method string, path string) bool {
	return r.r.Match(rctx, method, path)
}

// Use appends one or more middlewares onto the Router stack.
func (r *router) Use(middlewares ...func(http.Handler) http.Handler) {
	for _, m := range middlewares {
		if r.first {
			r.r.Use(middlewareRecordInit(m))
			r.first = false
		} else {
			r.r.Use(middlewareRecord(m))
		}
		r.r.Use(m)
	}
}

// With adds inline middlewares for an endpoint handler.
func (r *router) With(middlewares ...func(http.Handler) http.Handler) chi.Router {
	rx := r.r.With()
	r = &router{rx, false}
	r.Use(middlewares...)
	return r
}

// Group adds a new inline-Router along the current routing
// path, with a fresh middleware stack for the inline-Router.
func (r router) Group(fn func(r chi.Router)) chi.Router {
	im := r.With()
	if fn != nil {
		fn(im)
	}
	return im
}

// Route mounts a sub-Router along a `pattern“ string.
func (r router) Route(pattern string, fn func(r chi.Router)) chi.Router {
	sub := NewRouter()
	if fn != nil {
		fn(sub)
	}
	r.Mount(pattern, sub)
	return sub
}

// Mount attaches another http.Handler along ./pattern/*
func (r router) Mount(pattern string, h http.Handler) {
	r.r.Mount(pattern, h)
}

// Handle and HandleFunc adds routes for `pattern` that matches
// all HTTP methods.
func (r router) Handle(pattern string, h http.Handler) {
	r.r.Handle(pattern, middlewareRecordFinish(h))
}

func (r router) HandleFunc(pattern string, h http.HandlerFunc) {
	r.r.Handle(pattern, middlewareRecordFinish(h))
}

// Method and MethodFunc adds routes for `pattern` that matches
// the `method` HTTP method.
func (r router) Method(method string, pattern string, h http.Handler) {
	r.r.Method(method, pattern, middlewareRecordFinish(h))
}

func (r router) MethodFunc(method string, pattern string, h http.HandlerFunc) {
	r.r.MethodFunc(method, pattern, middlewareRecordFinish(h).ServeHTTP)
}

// HTTP-method routing along `pattern`
func (r router) Connect(pattern string, h http.HandlerFunc) {
	r.r.Connect(pattern, middlewareRecordFinish(h).ServeHTTP)
}

func (r router) Delete(pattern string, h http.HandlerFunc) {
	r.r.Delete(pattern, middlewareRecordFinish(h).ServeHTTP)
}

func (r router) Get(pattern string, h http.HandlerFunc) {
	r.r.Get(pattern, middlewareRecordFinish(h).ServeHTTP)
}

func (r router) Head(pattern string, h http.HandlerFunc) {
	r.r.Head(pattern, middlewareRecordFinish(h).ServeHTTP)
}

func (r router) Options(pattern string, h http.HandlerFunc) {
	r.r.Options(pattern, middlewareRecordFinish(h).ServeHTTP)
}

func (r router) Patch(pattern string, h http.HandlerFunc) {
	r.r.Patch(pattern, middlewareRecordFinish(h).ServeHTTP)
}

func (r router) Post(pattern string, h http.HandlerFunc) {
	r.r.Post(pattern, middlewareRecordFinish(h).ServeHTTP)
}

func (r router) Put(pattern string, h http.HandlerFunc) {
	r.r.Put(pattern, middlewareRecordFinish(h).ServeHTTP)
}

func (r router) Trace(pattern string, h http.HandlerFunc) {
	r.r.Trace(pattern, middlewareRecordFinish(h).ServeHTTP)
}

func (r router) Find(rctx *chi.Context, method string, path string) string {
	return r.r.Find(rctx, method, path)
}

// NotFound defines a handler to respond whenever a route could
// not be found.
func (r router) NotFound(h http.HandlerFunc) {
	r.r.NotFound(h)
}

// MethodNotAllowed defines a handler to respond whenever a method is
// not allowed.
func (r router) MethodNotAllowed(h http.HandlerFunc) {
	r.r.MethodNotAllowed(h)
}
