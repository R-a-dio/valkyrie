package tracker

import (
	"context"
	"net"
	"net/http"
	"time"

	radio "github.com/R-a-dio/valkyrie"
	"github.com/R-a-dio/valkyrie/config"
	"github.com/R-a-dio/valkyrie/rpc"
	"github.com/R-a-dio/valkyrie/util"
	"github.com/R-a-dio/valkyrie/website"
	"github.com/Wessie/fdstore"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/hlog"
	"google.golang.org/grpc"
)

const (
	ICECAST_AUTH_HEADER         = "icecast-auth-user"
	ICECAST_CLIENTID_FIELD_NAME = "client"
)

type Server struct {
	cfg config.Config

	recorder *Recorder
	http     *http.Server
	grpc     *grpc.Server
	httpLn   net.Listener
	grpcLn   net.Listener
}

func (s *Server) Start(ctx context.Context, fds *fdstore.Store) error {
	var err error
	var state []byte

	logger := zerolog.Ctx(ctx)

	httpAddr := s.cfg.Conf().Tracker.ListenAddr.String()
	grpcAddr := s.cfg.Conf().Tracker.RPCAddr.String()

	s.httpLn, state, err = util.RestoreOrListen(fds, "tracker.http", "tcp", httpAddr)
	if err != nil {
		return err
	}

	s.grpcLn, state, err = util.RestoreOrListen(fds, "tracker.grpc", "tcp", grpcAddr)
	if err != nil {
		return err
	}

	_ = state

	logger.Info().Str("address", s.httpLn.Addr().String()).Msg("tracker http started listening")
	logger.Info().Str("address", s.grpcLn.Addr().String()).Msg("tracker grpc started listening")

	errCh := make(chan error, 2)
	go func() {
		errCh <- s.grpc.Serve(s.grpcLn)
	}()
	go func() {
		errCh <- s.http.Serve(s.httpLn)
	}()

	select {
	case <-ctx.Done():
		//TODO: do i run this in a goroutine?
		s.grpc.GracefulStop()

		return s.http.Shutdown(ctx)
	case err := <-errCh:
		return err
	}
}

func (s *Server) Close() error {
	//TODO: here too :3
	s.grpc.GracefulStop()

	return s.http.Close()
}

func NewServer(ctx context.Context, cfg config.Config) *Server {
	s := new(Server)
	s.recorder = NewRecorder(ctx, cfg)

	r := website.NewRouter()

	r.Use(
		hlog.NewHandler(*zerolog.Ctx(ctx)),
		hlog.RemoteAddrHandler("ip"),
		hlog.UserAgentHandler("user_agent"),
		hlog.RequestIDHandler("req_id", "Request-Id"),
		hlog.URLHandler("url"),
		hlog.MethodHandler("method"),
		hlog.ProtoHandler("protocol"),
		hlog.AccessHandler(zerologLoggerFunc),
	)
	r.Post("/listener_joined", ListenerAdd(ctx, s.recorder))
	r.Post("/listener_left", ListenerRemove(ctx, s.recorder))

	s.http = &http.Server{
		Addr:        cfg.Conf().Tracker.ListenAddr.String(),
		Handler:     r,
		BaseContext: func(l net.Listener) context.Context { return ctx },
	}
	gs := rpc.NewGrpcServer(ctx)
	rpc.RegisterListenerTrackerServer(gs, rpc.NewListenerTracker(s.recorder))

	s.grpc = gs

	return s
}
func zerologLoggerFunc(r *http.Request, status, size int, duration time.Duration) {
	hlog.FromRequest(r).Info().
		Int("status_code", status).
		Int("response_size_bytes", size).
		Dur("elapsed_ms", duration).
		Str("url", r.URL.String()).
		Msg("http request")
}

func ListenerAdd(ctx context.Context, recorder *Recorder) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()

		id := r.FormValue(ICECAST_CLIENTID_FIELD_NAME)
		if id == "" {
			// icecast send us no client id somehow, this is broken and
			// we can't record this listener
			hlog.FromRequest(r).WithLevel(zerolog.PanicLevel).Msg("received icecast client with no id")
			return
		}

		cid, err := radio.ParseListenerClientID(id)
		if err != nil {
			// icecast send us a client id that isn't an integer
			hlog.FromRequest(r).WithLevel(zerolog.PanicLevel).Msg("received icecast client with non-int id")
			return
		}

		// only return OK if we got the required ID from icecast
		w.Header().Set(ICECAST_AUTH_HEADER, "1")
		w.WriteHeader(http.StatusOK)

		go func() {
			recorder.ListenerAdd(ctx, NewListener(cid, r))
		}()
	}
}

func ListenerRemove(ctx context.Context, recorder *Recorder) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// always return OK because it doesn't really matter if the
		// rest of the request is broken
		w.WriteHeader(http.StatusOK)

		_ = r.ParseForm()

		id := r.FormValue(ICECAST_CLIENTID_FIELD_NAME)
		if id == "" {
			// icecast send us no client id somehow, this is broken and
			// we can't record this listener
			hlog.FromRequest(r).WithLevel(zerolog.PanicLevel).Msg("received icecast client with no id")
			return
		}

		cid, err := radio.ParseListenerClientID(id)
		if err != nil {
			// icecast send us a client id that isn't an integer
			hlog.FromRequest(r).WithLevel(zerolog.PanicLevel).Msg("received icecast client with non-int id")
			return
		}

		go recorder.ListenerRemove(ctx, cid)
	}
}
