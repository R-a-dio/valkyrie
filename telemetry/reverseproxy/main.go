package reverseproxy

import (
	"context"
	"net/http/httputil"
	"net/url"

	radio "github.com/R-a-dio/valkyrie"
	"github.com/R-a-dio/valkyrie/config"
	"github.com/R-a-dio/valkyrie/errors"
	"github.com/R-a-dio/valkyrie/storage"
	"github.com/R-a-dio/valkyrie/util"
	"github.com/R-a-dio/valkyrie/website"
	vmiddleware "github.com/R-a-dio/valkyrie/website/middleware"
	"github.com/go-chi/chi/middleware"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/hlog"
)

func New(ctx context.Context, cfg config.Config) *httputil.ReverseProxy {
	AdminMonitoringURL := config.Value(cfg, func(cfg config.Config) *url.URL {
		return cfg.Conf().Website.AdminMonitoringURL.URL()
	})
	AdminMonitoringUserHeader := config.Value(cfg, func(cfg config.Config) string {
		return cfg.Conf().Website.AdminMonitoringUserHeader
	})
	AdminMonitoringRoleHeader := config.Value(cfg, func(cfg config.Config) string {
		return cfg.Conf().Website.AdminMonitoringRoleHeader
	})

	return &httputil.ReverseProxy{
		Rewrite: func(pr *httputil.ProxyRequest) {
			pr.SetURL(AdminMonitoringURL())
			pr.SetXForwarded()
			pr.Out.Host = pr.In.Host

			u := vmiddleware.UserFromContext(pr.In.Context())
			pr.Out.Header.Add(AdminMonitoringUserHeader(), u.Username)
			if u.UserPermissions.Has(radio.PermAdmin) {
				pr.Out.Header.Add(AdminMonitoringRoleHeader(), "Admin")
			}
		},
	}
}

func ExecuteStandalone(ctx context.Context, cfg config.Config) error {
	const op errors.Op = "telemetry/reverseproxy.ExecuteStandalone"
	logger := zerolog.Ctx(ctx)

	// database access
	storage, err := storage.Open(ctx, cfg)
	if err != nil {
		return errors.E(op, err)
	}

	r := website.NewRouter()
	r.Use(middleware.RealIP)
	// setup zerolog details
	r.Use(
		hlog.NewHandler(*logger),
		hlog.RemoteAddrHandler("ip"),
		hlog.UserAgentHandler("user_agent"),
		hlog.RequestIDHandler("req_id", "Request-Id"),
		hlog.URLHandler("url"),
		hlog.MethodHandler("method"),
		hlog.ProtoHandler("protocol"),
		hlog.CustomHeaderHandler("is_htmx", "Hx-Request"),
		hlog.AccessHandler(util.ZerologLoggerFunc),
	)
	// recover from panics
	r.Use(vmiddleware.Recoverer)
	// session handling
	sessionManager := vmiddleware.NewSessionManager(ctx, storage)
	r.Use(sessionManager.LoadAndSave)
	// user handling
	authentication := vmiddleware.NewAuthentication(storage, executor, sessionManager)
	r.Use(authentication.UserMiddleware)
	rvp := New(ctx, cfg)

	srv
	return nil
}
