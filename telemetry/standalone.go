package telemetry

import (
	"context"
	"net/http"
	"time"

	radio "github.com/R-a-dio/valkyrie"
	"github.com/R-a-dio/valkyrie/config"
	"github.com/R-a-dio/valkyrie/errors"
	"github.com/R-a-dio/valkyrie/storage"
	"github.com/R-a-dio/valkyrie/telemetry/reverseproxy"
	"github.com/R-a-dio/valkyrie/templates"
	"github.com/R-a-dio/valkyrie/templates/functions"
	"github.com/R-a-dio/valkyrie/util"
	vmiddleware "github.com/R-a-dio/valkyrie/website/middleware"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/hlog"
)

func ExecuteStandalone(ctx context.Context, cfg config.Config) error {
	const op errors.Op = "telemetry/reverseproxy.ExecuteStandalone"
	logger := zerolog.Ctx(ctx)

	// database access
	storage, err := storage.Open(ctx, cfg)
	if err != nil {
		return errors.E(op, err)
	}

	// construct our stateful template functions, it uses the latest values from the manager
	templateFuncs := functions.NewStatefulFunctions(cfg, nil)
	// construct our templates from files on disk
	siteTemplates, err := templates.FromDirectory(
		cfg.Conf().TemplatePath,
		templateFuncs,
	)
	if err != nil {
		return errors.E(op, err)
	}
	siteTemplates.Production = true
	executor := siteTemplates.Executor()

	r := NewRouter()
	r.Use(middleware.RealIP)
	// setup zerolog details
	r.Use(
		util.NewZerologAttributes(*logger),
		hlog.RequestIDHandler("req_id", "Request-Id"),
		hlog.AccessHandler(util.ZerologLoggerFunc),
	)
	// recover from panics
	r.Use(vmiddleware.Recoverer)
	// session handling
	sessionManager := vmiddleware.NewSessionManager(ctx, storage, true)
	r.Use(sessionManager.LoadAndSave)
	// user handling
	authentication := vmiddleware.NewAuthentication(storage, executor, sessionManager)
	r.Use(authentication.UserMiddleware)
	// theme handling, not really needed but the login middleware wants it
	r.Use(templates.ThemeCtxSimple(templates.ThemeAdminDefault))

	rvp := reverseproxy.New(ctx, cfg)

	r.Get("/logout", authentication.LogoutHandler)
	r.Route("/", func(r chi.Router) {
		r = r.With(
			authentication.LoginMiddleware,
		)

		r.Handle("/*", vmiddleware.RequirePermission(radio.PermTelemetryView, rvp.ServeHTTP))
	})

	conf := cfg.Conf()

	server := &http.Server{
		Addr:              conf.Telemetry.StandaloneProxy.ListenAddr.String(),
		Handler:           r,
		ReadHeaderTimeout: time.Second * 5,
	}

	errCh := make(chan error, 1)
	go func() {
		errCh <- server.ListenAndServe()
	}()

	select {
	case <-ctx.Done():
		return server.Close()
	case err = <-errCh:
		return err
	}
}
