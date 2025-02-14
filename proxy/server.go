package proxy

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"sync"
	"time"

	radio "github.com/R-a-dio/valkyrie"
	"github.com/R-a-dio/valkyrie/config"
	"github.com/R-a-dio/valkyrie/errors"
	"github.com/R-a-dio/valkyrie/proxy/compat"
	"github.com/R-a-dio/valkyrie/util"
	"github.com/R-a-dio/valkyrie/website"
	"github.com/R-a-dio/valkyrie/website/middleware"
	"github.com/Wessie/fdstore"
	"github.com/go-chi/chi/v5"
	chiware "github.com/go-chi/chi/v5/middleware"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/hlog"
)

type Server struct {
	cfg        config.Config
	listenerMu sync.Mutex
	listener   net.Listener
	proxy      *ProxyManager
	storage    radio.UserStorageService
	manager    radio.ManagerService
	http       *http.Server
	events     *EventHandler
}

func NewServer(ctx context.Context, cfg config.Config, manager radio.ManagerService, uss radio.UserStorageService) (*Server, error) {
	const op errors.Op = "proxy.NewServer"

	eh := NewEventHandler(ctx, cfg)
	pm, err := NewProxyManager(ctx, cfg, uss, eh)
	if err != nil {
		return nil, errors.E(op, err)
	}
	var srv = &Server{
		cfg:     cfg,
		proxy:   pm,
		manager: manager,
		storage: uss,
		events:  eh,
	}

	// older icecast source clients still use the SOURCE method instead of PUT
	chi.RegisterMethod("SOURCE")

	logger := zerolog.Ctx(ctx)
	// setup our routes
	r := website.NewRouter()
	// setup zerolog
	r.Use(
		util.NewZerologAttributes(*logger),
		hlog.RequestIDHandler("req_id", "Request-Id"),
		hlog.AccessHandler(util.ZerologLoggerFunc),
	)
	r.Use(chiware.Recoverer)
	// handle basic authentication
	r.Use(middleware.BasicAuth(uss))
	// and generate an identifier for the user
	r.Use(IdentifierMiddleware)
	// metadata route used to update mp3 metadata out-of-bound
	r.Get("/admin/metadata", middleware.RequirePermission(radio.PermDJ, srv.GetMetadata))
	// listclients would normally listen all listener clients but since we
	// don't have any of those we could normally just not implement this. But
	// some streaming software assumes this endpoint exists to display a listener
	// count, so we just return a static 0;
	r.Get("/admin/listclients", srv.GetListClients)
	r.HandleFunc("/*", middleware.RequirePermission(radio.PermDJ, srv.PutSource))

	srv.http = &http.Server{
		Handler:      r,
		ReadTimeout:  time.Second * 10,
		WriteTimeout: time.Second * 5,
	}

	return srv, nil
}

func (s *Server) Close() error {
	return s.http.Close()
}

func (srv *Server) Start(ctx context.Context, fdstorage *fdstore.Store) error {
	logger := zerolog.Ctx(ctx)

	addr := srv.cfg.Conf().Proxy.ListenAddr.String()

	ln, state, err := util.RestoreOrListen(fdstorage, fdstoreHTTPName, "tcp", addr)
	if err != nil {
		return err
	}
	ln = compat.Wrap(logger, ln)

	if len(state) > 0 {
		err = json.Unmarshal(state, srv.proxy)
		if err != nil {
			// not a critical error, log it and continue
			logger.Error().Ctx(ctx).Err(err).Str("json", string(state)).Msg("failed to unmarshal state")
		}
	}

	srv.listenerMu.Lock()
	srv.listener = ln
	srv.listenerMu.Unlock()

	// before we actually start serving we give the manager a chance to recover
	// its mounts from the fdstorage if any exist
	srv.proxy.restoreMounts(ctx, fdstorage)

	logger.Info().Ctx(ctx).Str("address", ln.Addr().String()).Msg("proxy started listening")
	return srv.http.Serve(ln)
}
