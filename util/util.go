package util

import (
	"context"
	"fmt"
	"mime"
	"net"
	"net/http"
	"net/url"
	"path/filepath"
	"runtime/debug"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	radio "github.com/R-a-dio/valkyrie"
	"github.com/R-a-dio/valkyrie/errors"
	"github.com/R-a-dio/valkyrie/util/eventstream"
	"github.com/Wessie/fdstore"
	"github.com/go-chi/chi/v5"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/hlog"
	"golang.org/x/time/rate"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func init() {
	must := func(err error) {
		if err != nil {
			panic(err)
		}
	}
	must(mime.AddExtensionType(".opus", "audio/ogg"))
	must(mime.AddExtensionType(".mp3", "audio/mpeg"))
	must(mime.AddExtensionType(".flac", "audio/flac"))
	must(mime.AddExtensionType(".m4a", "audio/mp4"))
}

func ZerologLoggerFunc(r *http.Request, status, size int, duration time.Duration) {
	hlog.FromRequest(r).Info().Ctx(r.Context()).
		Int("status_code", status).
		Int("response_size_bytes", size).
		Dur("elapsed_ms", duration).
		Msg("http request")
}

// NewZerologAttributes aggregates a bunch of zerolog/hlog middleware into a single one
//
//	hlog.NewHandler(*logger)
//	hlog.RemoteAddrHandler("ip")
//	hlog.UserAgentHandler("user_agent")
//	hlog.URLHandler("url")
//	hlog.MethodHandler("method")
//	hlog.ProtoHandler("protocol")
//	hlog.CustomHeaderHandler("is_htmx", "Hx-Request")
//	hlog.RefererHandler("referer")
func NewZerologAttributes(logger zerolog.Logger) func(next http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			zctx := logger.With().
				Str("url", r.URL.String()).
				Str("method", r.Method).
				Str("ip", r.RemoteAddr).
				Bool("is_htmx", IsHTMX(r)).
				Str("req_id", "").
				Str("protocol", r.Proto)

			if ua := r.Header.Get("User-Agent"); ua != "" {
				zctx = zctx.Str("user_agent", ua)
			}
			if ref := r.Header.Get("Referer"); ref != "" {
				zctx = zctx.Str("referer", ref)
			}

			l := zctx.Logger()
			r = r.WithContext(l.WithContext(r.Context()))
			next.ServeHTTP(w, r)
		})
	}
}

// IsHTMX checks if a request was made by HTMX through the Hx-Request header
func IsHTMX(r *http.Request) bool {
	return r.Header.Get("Hx-Request") == "true" && r.Header.Get("Hx-History-Restore-Request") != "true"
}

func RedirectBack(r *http.Request) (nr *http.Request, success bool) {
	var changed bool

	if hxHeader := r.Header.Get("Hx-Current-Url"); hxHeader != "" {
		current, err := url.Parse(hxHeader)
		if err == nil {
			r.URL = current
			changed = true
		}
	}

	if !changed {
		current, err := url.Parse(r.Referer())
		if err == nil {
			r.URL = current
			changed = true
		}
	}

	r.RequestURI = r.URL.RequestURI()
	// chi uses some internal routing context that holds state even after we
	// redirect with the above method, so we empty the RoutePath in it so that
	// chi will fill it back in
	rCtx := r.Context().Value(chi.RouteCtxKey)
	if rCtx != nil {
		if chiCtx, ok := rCtx.(*chi.Context); ok {
			chiCtx.RoutePath = ""
		}
	}
	return r, changed
}

// RedirectPath modifies r's path to the newpath given
func RedirectPath(r *http.Request, newpath string) *http.Request {
	r.URL.Path = newpath
	r.RequestURI = r.URL.RequestURI()
	return r
}

func ChangeRequestMethod(r *http.Request, method string) *http.Request {
	r.Method = method

	rCtx := r.Context().Value(chi.RouteCtxKey)
	if rCtx != nil {
		if chiCtx, ok := rCtx.(*chi.Context); ok {
			chiCtx.RouteMethod = method
		}
	}

	return r
}

type alreadyRedirectedKey struct{}

// RedirectToServer looks up the http.Server associated with this request
// and calls ServeHTTP again
func RedirectToServer(w http.ResponseWriter, r *http.Request) error {
	const op errors.Op = "util.RedirectToServer"
	ctx := r.Context()

	alreadyRedirected := ctx.Value(alreadyRedirectedKey{})
	if alreadyRedirected != nil {
		return errors.E(op, "request was already redirected once")
	}

	srv := ctx.Value(http.ServerContextKey)
	if srv == nil {
		return errors.E(op, "no server context key found")
	}

	httpSrv, ok := srv.(*http.Server)
	if !ok {
		return errors.E(op, "server context key did not contain *http.Server")
	}

	// add a context value so we know we've redirected internally
	ctx = context.WithValue(r.Context(), alreadyRedirectedKey{}, struct{}{})

	// and then send them off to be handled again
	httpSrv.Handler.ServeHTTP(w, r.WithContext(ctx))
	return nil
}

func AbsolutePath(dir string, path string) string {
	if filepath.IsAbs(path) {
		return path
	}
	return filepath.Join(dir, path)
}

const HeaderContentDisposition = "Content-Disposition"

func AddContentDispositionSong(w http.ResponseWriter, metadata, filename string) {
	filename = metadata + filepath.Ext(filename)
	AddContentDisposition(w, filename)
}

var headerReplacer = strings.NewReplacer(
	"\r", "", "\n", "", // newlines
	"+", "%20", // spaces from the query escape
)

var rfc2616 = strings.NewReplacer(
	`\`, `\\`, // escape character
	`"`, `\"`, // quotes
)

func AddContentDisposition(w http.ResponseWriter, filename string) {
	disposition := "attachment; " + makeHeader(filename)
	w.Header().Set(HeaderContentDisposition, disposition)
	// also add a content-type header if we can get a mimetype
	ct := mime.TypeByExtension(filepath.Ext(filename))
	if ct != "" {
		w.Header().Set("Content-Type", ct)
	}
}

func makeHeader(filename string) string {
	// For some reason Go doesn't provide access to the internal percent
	// encoding routines, meaning we have to do this to get a fully
	// percent-encoded string including spaces as %20.
	encoded := url.QueryEscape(filename)
	encoded = headerReplacer.Replace(encoded)
	// RFC2616 quoted string encoded
	escaped := rfc2616.Replace(filename)
	// RFC5987 regular and extended header value encoding
	disposition := fmt.Sprintf(`filename="%s"; filename*=UTF-8''%s`, escaped, encoded)
	return disposition
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

// NewStatusValue is a silly special cased *Value that updates the statuses listener
// count before returning the status, since just listening to CurrentStatus by itself
// will only have listeners update when a song change occurs
func NewStatusValue(ctx context.Context, m radio.ManagerService) StreamValuer[radio.Status] {
	return &statusValue{
		sv: StreamValue(ctx, m.CurrentStatus),
		lv: StreamValue(ctx, m.CurrentListeners),
	}
}

type StreamValuer[T any] interface {
	Latest() T
}

type statusValue struct {
	sv StreamValuer[radio.Status]
	lv StreamValuer[radio.Listeners]
}

func (sv *statusValue) Latest() radio.Status {
	status := sv.sv.Latest()
	status.Listeners = sv.lv.Latest()
	return status
}

// StreamValue opens the stream created by StreamFn and calls any callbackFn given everytime a new
// value is returned by the stream. StreamValue also stores the last received value, accessible by
// calling .Latest
func StreamValue[T any](ctx context.Context, fn StreamFn[T], callbackFn ...StreamCallbackFn[T]) *Value[T] {
	var value Value[T]
	value.last.Store(new(T))

	go func() {
		var stream eventstream.Stream[T]
		var err error
		defer func() {
			if stream != nil {
				stream.Close()
			}
		}()

		rater := rate.NewLimiter(rate.Every(time.Second), 1)
		for {
			// make sure we don't spam this bit of code by rate limiting ourselves
			if err = rater.Wait(ctx); err != nil {
				zerolog.Ctx(ctx).Debug().Ctx(ctx).Err(err).Msg("stream-value: ctx canceled")
				return
			}

			stream, err = fn(ctx)
			if err != nil {
				if status.Code(err) == codes.Canceled {
					// in case of cancel just exit quietly
					zerolog.Ctx(ctx).Debug().Ctx(ctx).Err(err).Msg("stream-value: ctx canceled")
					return
				}

				// stream creation error most likely means the service
				// is down or unavailable for some reason so retry in
				// a little bit and stay alive
				zerolog.Ctx(ctx).Error().Ctx(ctx).Err(err).Msg("stream-value: stream error")
				continue
			}

			zerolog.Ctx(ctx).Info().Ctx(ctx).Msg("stream-value: connected")

			for {
				v, err := stream.Next()
				if err != nil {
					// we either got context canceled or received some
					// stream error that indicates we need a new stream,
					// try and get one from the outer loop.
					if status.Code(err) == codes.Canceled {
						// in case of cancel just exit quietly
						zerolog.Ctx(ctx).Debug().Ctx(ctx).Err(err).Msg("stream-value: ctx canceled")
						return
					}
					zerolog.Ctx(ctx).Error().Ctx(ctx).Err(err).Msg("stream-value: next error")
					break
				}

				value.last.Store(&v)

				for _, callback := range callbackFn {
					func() {
						defer recoverPanicLogger(ctx)
						callback(ctx, v)
					}()
				}
			}
			stream.Close()
		}
	}()

	return &value
}

func recoverPanicLogger(ctx context.Context) {
	rvr := recover()
	if rvr == nil {
		return
	}
	if err, ok := rvr.(error); ok && err != nil {
		zerolog.Ctx(ctx).WithLevel(zerolog.PanicLevel).Str("stack", string(debug.Stack())).Err(err).Msg("panic in StreamValue callback")
		return
	}
	zerolog.Ctx(ctx).WithLevel(zerolog.PanicLevel).Str("stack", string(debug.Stack())).Any("recover", rvr).Msg("panic in StreamValue callback")
}

type Value[T any] struct {
	last atomic.Pointer[T]
}

func (v *Value[T]) Latest() T {
	return *v.last.Load()
}

// NewStaticValue returns a Value that stores the static variable, should really
// only be used in testing
func NewStaticValue[T any](static T) *Value[T] {
	var value Value[T]
	value.last.Store(&static)
	return &value
}

type CallbackTimer struct {
	fn func()

	mu    sync.Mutex
	timer *time.Timer
}

func NewCallbackTimer(callback func()) *CallbackTimer {
	return &CallbackTimer{
		fn: callback,
	}
}

// Start starts a timer with the timeout given, if a timer
// is already running it is stopped and a new timer is created
func (tc *CallbackTimer) Start(timeout time.Duration) {
	tc.mu.Lock()
	defer tc.mu.Unlock()
	if tc.timer != nil {
		tc.timer.Stop()
	}
	tc.timer = time.AfterFunc(timeout, tc.fn)
}

// Stop stops the current timer if one exists
func (tc *CallbackTimer) Stop() bool {
	if tc == nil {
		return true
	}
	tc.mu.Lock()
	defer tc.mu.Unlock()
	if tc.timer != nil {
		return tc.timer.Stop()
	}
	return true
}

// RestoreOrListen tries to restore a listener with the name given from
// the store given. If any error occurs it instead just calls net.Listen
// with the provided arguments network and addr
func RestoreOrListen(store *fdstore.Store, name string, network, addr string) (net.Listener, []byte, error) {
	lns, err := store.RemoveListener(name)
	if err != nil || len(lns) == 0 {
		ln, err := net.Listen(network, addr)
		return ln, nil, err
	}

	return lns[0].Listener, lns[0].Data, nil
}

func ReduceWithStep[T any](s []T, step int) []T {
	if step < 1 {
		// set the step to 1 if it's lower than that, this to
		// avoid a panic below, also zero or negative step is
		// undefined behavior for this function
		step = 1
	}

	var res []T

	for i := step - 1; i < len(s); i += step {
		res = append(res, s[i])
	}

	return res
}

func ReduceHasLeftover[T any](s []T, step int) bool {
	if step > 0 {
		return len(s)%step > 0
	}
	return false
}
