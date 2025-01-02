package tracker

import (
	"context"
	"net"
	"net/http"
	"time"

	radio "github.com/R-a-dio/valkyrie"
	"github.com/R-a-dio/valkyrie/config"
	"github.com/R-a-dio/valkyrie/errors"
	"github.com/R-a-dio/valkyrie/rpc"
	"github.com/R-a-dio/valkyrie/util"
	"github.com/R-a-dio/valkyrie/website"
	"github.com/Wessie/fdstore"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/hlog"
	"google.golang.org/grpc"
)

const (
	// UpdateListenersTickrate is the period between two UpdateListeners
	// calls done to the manager
	UpdateListenersTickrate = time.Second * 10
	// SyncListenersTickrate is the period between two sync operations
	SyncListenersTickrate = time.Minute * 10

	RemoveStaleTickrate = time.Hour * 24
	RemoveStalePeriod   = time.Minute * 5

	HttpLn      = "tracker.http"
	GrpcLn      = "tracker.grpc"
	TrackerFile = "tracker.listeners"

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

	h http.Handler
}

func cloneListener(ln net.Listener) (net.Listener, error) {
	const op errors.Op = "tracker/cloneListener"

	clone, err := ln.(fdstore.Filer).File()
	if err != nil {
		return nil, errors.E(op, err)
	}

	ln, err = net.FileListener(clone)
	if err != nil {
		return nil, errors.E(op, err)
	}

	return ln, nil
}

func (s *Server) Start(ctx context.Context, fds *fdstore.Store) error {
	const op errors.Op = "tracker/Server.Start"
	logger := zerolog.Ctx(ctx)

	httpAddr := s.cfg.Conf().Tracker.ListenAddr.String()
	grpcAddr := s.cfg.Conf().Tracker.RPCAddr.String()

	// get hold of our http listener
	httpLn, recorderState, err := util.RestoreOrListen(fds, HttpLn, "tcp", httpAddr)
	if err != nil {
		return errors.E(op, err)
	}

	s.httpLn, err = cloneListener(httpLn)
	if err != nil {
		// log the error but other than that just continue
		logger.Error().Err(err).Msg("failed to clone http listener")
	}

	grpcLn, _, err := util.RestoreOrListen(fds, GrpcLn, "tcp", grpcAddr)
	if err != nil {
		return errors.E(op, err)
	}

	s.grpcLn, err = cloneListener(grpcLn)
	if err != nil {
		// log the error but other than that just continue, since the rest will still function
		logger.Error().Err(err).Msg("failed to clone grpc listener")
	}

	if len(recorderState) > 0 {
		s.recorder.UnmarshalJSON(recorderState)
	}

	logger.Info().Str("address", s.httpLn.Addr().String()).Msg("tracker http started listening")
	logger.Info().Str("address", s.grpcLn.Addr().String()).Msg("tracker grpc started listening")

	// setup periodic task to update the manager of our listener count
	go s.periodicallyUpdateListeners(ctx, UpdateListenersTickrate)
	// setup periodic task to keep recorder state in sync with icecast
	go s.periodicallySyncListeners(ctx, SyncListenersTickrate)

	errCh := make(chan error, 2)
	go func() {
		errCh <- s.grpc.Serve(grpcLn)
	}()
	go func() {
		errCh <- s.http.Serve(httpLn)
	}()

	select {
	case <-ctx.Done():
		return s.Close()
	case err := <-errCh:
		return err
	}
}

func (s *Server) Close() error {
	s.grpc.Stop()
	return s.http.Close()
}

func (s *Server) Shutdown(ctx context.Context) error {
	const op errors.Op = "tracker/Server.Shutdown"
	// stop the grpc server, this one can't mutate any extra state so a normal stop
	// should be fine
	s.grpc.Stop()

	// stutdown the http server, this one does mutate state, so we need a "graceful"
	// shutdown that waits for in-flight requests to finish
	err := s.http.Shutdown(ctx)
	if err != nil {
		return errors.E(op, err)
	}

	// TODO: wait for the recorder state to actually handle all in-flight requests
	return nil
}

func NewServer(ctx context.Context, cfg config.Config) *Server {
	s := new(Server)
	s.recorder = NewRecorder(ctx, cfg)

	s.cfg = cfg

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
	s.h = r

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

		// use an extra indirect function such that NewListener is executed in the goroutine instead
		// of the handler
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

func (s *Server) periodicallyUpdateListeners(ctx context.Context,
	tickrate time.Duration,
) {
	ticker := time.NewTicker(tickrate)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			err := s.cfg.Manager.UpdateListeners(ctx, s.recorder.ListenerAmount())
			if err != nil {
				zerolog.Ctx(ctx).Error().Err(err).Msg("failed update listeners")
			}
		}
	}
}

func (s *Server) periodicallySyncListeners(ctx context.Context,
	tickrate time.Duration,
) {
	ticker := time.NewTicker(tickrate)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			err := s.syncListeners(ctx)
			if err != nil {
				zerolog.Ctx(ctx).Error().Err(err).Msg("failed sync listeners")
			}
		}
	}
}

func (s *Server) syncListeners(ctx context.Context) error {
	const op errors.Op = "tracker/syncListeners"

	s.recorder.syncing.Store(true)
	defer s.recorder.syncing.Store(false)

	ctx, cancel := context.WithTimeout(ctx, time.Second*5)
	defer cancel()

	list, err := GetIcecastListClients(ctx, s.cfg)
	if err != nil {
		return errors.E(op, err)
	}

	s.recorder.Sync(ctx, list)
	return nil
}
