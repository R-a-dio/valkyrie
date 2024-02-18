package proxy

import (
	"context"
	"net"
	"net/http"
	"net/url"
	"time"

	radio "github.com/R-a-dio/valkyrie"
	"github.com/R-a-dio/valkyrie/website/middleware"
	"github.com/go-chi/chi/v5"
	chiware "github.com/go-chi/chi/v5/middleware"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/hlog"
)

type Server struct {
	storage radio.UserStorageService
	manager radio.ManagerService
	http    *http.Server
}

func zerologLoggerFunc(r *http.Request, status, size int, duration time.Duration) {
	hlog.FromRequest(r).Info().
		Int("status_code", status).
		Int("response_size_bytes", size).
		Dur("elapsed_ms", duration).
		Msg("http request")
}

func NewServer(ctx context.Context, manager radio.ManagerService, storage radio.UserStorageService) (*Server, error) {
	var srv = &Server{
		manager: manager,
		storage: storage,
	}

	logger := zerolog.Ctx(ctx)
	// setup our routes
	r := chi.NewRouter()
	// setup zerolog
	r.Use(
		hlog.NewHandler(*logger),
		hlog.RemoteAddrHandler("ip"),
		hlog.UserAgentHandler("user_agent"),
		hlog.RequestIDHandler("req_id", "Request-Id"), // TODO: check if we want to return the header
		hlog.URLHandler("url"),
		hlog.MethodHandler("method"),
		hlog.ProtoHandler("protocol"),
		hlog.CustomHeaderHandler("is_htmx", "Hx-Request"),
		hlog.AccessHandler(zerologLoggerFunc),
	)
	r.Use(chiware.Recoverer)
	// handle basic authentication
	r.Use(middleware.BasicAuth(storage))
	// and generate an identifier for the user
	r.Use(IdentifierMiddleware)
	// metadata route used to update mp3 metadata out-of-bound
	r.Get("/admin/metadata", srv.GetMetadata)
	// listclients would normally listen all listener clients but since we
	// don't have any of those we could normally just not implement this. But
	// some streaming software assumes this endpoint exists to display a listener
	// count, so we just return a static 0;
	r.Get("/admin/listclients", srv.GetListClients)
	r.MethodFunc(http.MethodPut, "/", srv.PutSource)
	r.MethodFunc("SOURCE", "/", srv.PutSource)

	srv.http = &http.Server{
		Handler:      r,
		ReadTimeout:  time.Second * 10,
		WriteTimeout: time.Second * 5,
	}

	return srv, nil
}

func (s *Server) Serve(l net.Listener) error {
	return s.http.Serve(l)
}

func (s *Server) Close() error {
	return s.http.Close()
}

func NewSourceConn(ctx context.Context, uri *url.URL) (net.Conn, error) {
	return nil, nil
}
