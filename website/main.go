package website

import (
	"context"
	"io/fs"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	radio "github.com/R-a-dio/valkyrie"
	"github.com/R-a-dio/valkyrie/config"
	"github.com/R-a-dio/valkyrie/errors"
	"github.com/R-a-dio/valkyrie/search"
	"github.com/R-a-dio/valkyrie/storage"
	"github.com/R-a-dio/valkyrie/templates"
	"github.com/R-a-dio/valkyrie/util"
	"github.com/R-a-dio/valkyrie/util/secret"
	"github.com/R-a-dio/valkyrie/website/admin"
	phpapi "github.com/R-a-dio/valkyrie/website/api/php"
	v1 "github.com/R-a-dio/valkyrie/website/api/v1"
	vmiddleware "github.com/R-a-dio/valkyrie/website/middleware"
	"github.com/R-a-dio/valkyrie/website/public"
	"github.com/R-a-dio/valkyrie/website/shared"
	"github.com/Wessie/fdstore"
	"github.com/gorilla/csrf"
	"github.com/jxskiss/base62"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/hlog"
	"github.com/spf13/afero"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
)

var NewRouter = func() chi.Router { return chi.NewRouter() }

func zerologLoggerFunc(r *http.Request, status, size int, duration time.Duration) {
	hlog.FromRequest(r).Info().
		Int("status_code", status).
		Int("response_size_bytes", size).
		Dur("elapsed_ms", duration).
		Msg("http request")
}

// Execute runs a website instance with the configuration given
func Execute(ctx context.Context, cfg config.Config) error {
	const op errors.Op = "website/Execute"
	logger := zerolog.Ctx(ctx)

	if path := cfg.Conf().Website.DJImagePath; path == "" {
		return errors.E(op, "Website.DJImagePath is not configured")
	} else {
		// try and create the DJImagePath
		if err := os.MkdirAll(path, 0664); err != nil {
			return errors.E(op, "failed to create Website.DJImagePath", err)
		}
		// and the DJImagePath tmp dir where we store temporary uploads
		if err := os.MkdirAll(filepath.Join(path, os.TempDir()), 0664); err != nil {
			return errors.E(op, "failed to create Website.DJImagePath tmp dir", err)
		}
	}

	// database access
	storage, err := storage.Open(ctx, cfg)
	if err != nil {
		return errors.E(op, err)
	}

	// status RPC value
	statusValue := util.StreamValue(ctx, cfg.Manager.CurrentStatus)

	// templates
	// construct our stateful template functions, it uses the latest values from the manager
	templateFuncs := templates.NewStatefulFunctions(statusValue)
	// construct our templates from files on disk
	siteTemplates, err := templates.FromDirectory(
		cfg.Conf().TemplatePath,
		templateFuncs,
	)
	if err != nil {
		return errors.E(op, err)
	}
	executor := siteTemplates.Executor()

	// template value, for deciding what theme to use
	themeValues := templates.NewThemeValues(siteTemplates.ResolveThemeName)
	// user RPC value
	_ = util.StreamValue(ctx, cfg.Manager.CurrentUser, func(ctx context.Context, u *radio.User) {
		// if either no user, or no theme set, unset the DJ theme
		if u == nil || u.DJ.Theme == "" {
			themeValues.StoreDJ("")
			return
		}

		themeValues.StoreDJ(u.DJ.Theme)
	})

	// daypass generation
	dpass, err := secret.NewSecret(secret.DaypassLength)
	if err != nil {
		return errors.E(op, err)
	}
	// song download key generation
	songSecret, err := secret.NewSecret(secret.SongLength)
	if err != nil {
		return errors.E(op, err)
	}
	// search service
	searchService, err := search.Open(ctx, cfg)
	if err != nil {
		return errors.E(op, err)
	}
	// cache for news posts
	newsCache := shared.NewNewsCache()

	r := NewRouter()

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
		hlog.AccessHandler(zerologLoggerFunc),
	)
	// recover from panics and clean our IP of a port number
	r.Use(removePortFromAddress)
	r.Use(middleware.Recoverer)
	// session handling
	sessionManager := vmiddleware.NewSessionManager(ctx, storage)
	r.Use(sessionManager.LoadAndSave)
	// user handling
	authentication := vmiddleware.NewAuthentication(storage, executor, sessionManager)
	r.Use(authentication.UserMiddleware)
	// CSRF token handling
	// fixes a compatibility issue with the PHP api, see middleware documentation
	r.Use(phpapi.MoveTokenToHeaderForRequests)
	r.Use(skipCSRFProtection)
	csrfKey := []byte(cfg.Conf().Website.CSRFSecret)
	if len(csrfKey) == 0 {
		logger.Warn().Msg("CSRFSecret is empty, using random key")
		csrfKey, err = secret.NewKey(32)
		if err != nil {
			panic("CSRFSecret is empty and we couldn't generate a random key")
		}
	}
	r.Use(csrf.Protect(csrfKey,
		csrf.Secure(false),
		csrf.Encoding(base62.StdEncoding),
	))
	// shared input handling, stuff the base template needs
	r.Use(vmiddleware.InputMiddleware(cfg, statusValue))
	// theme state management
	r.Use(templates.ThemeCtx(themeValues))

	// legacy urls that once pointed to our stream, redirect them to the new url
	redirectHandler := RedirectLegacyStream(cfg)
	r.Get("/main.mp3", redirectHandler)
	r.Get("/main", redirectHandler)
	r.Get("/stream.mp3", redirectHandler)
	r.Get("/stream", redirectHandler)
	r.Get("/R-a-dio", redirectHandler)

	// serve assets from the assets directory
	r.Handle("/assets/*", http.StripPrefix("/assets/",
		AssetsHandler(cfg.Conf().AssetsPath, siteTemplates)),
	)
	r.Handle("/set-theme", templates.SetThemeHandler(templates.ThemeCookieName))

	// version 0 of the api (the legacy PHP version)
	// it's mostly self-contained to the /api/* route, except for /request that
	// leaked out at some point
	logger.Info().Str("event", "init").Str("part", "api_v0").Msg("")
	v0, err := phpapi.NewAPI(ctx, cfg, storage, statusValue)
	if err != nil {
		return errors.E(op, err)
	}
	r.Route("/api", v0.Route)
	r.Route(`/request/{TrackID:[0-9]+}`, v0.RequestRoute)

	// version 1 of the api
	logger.Info().Str("event", "init").Str("part", "api_v1").Msg("")
	v1, err := v1.NewAPI(ctx, cfg, executor, afero.NewReadOnlyFs(afero.NewOsFs()), songSecret)
	if err != nil {
		return errors.E(op, err)
	}
	defer v1.Shutdown()
	r.Route("/v1", v1.Route)

	// admin routes
	r.Get("/logout", authentication.LogoutHandler) // outside so it isn't login restricted
	r.Route("/admin", admin.Route(ctx, admin.NewState(
		ctx,
		cfg,
		dpass,
		songSecret,
		newsCache,
		storage,
		searchService,
		siteTemplates,
		executor,
		sessionManager,
		authentication,
		afero.NewOsFs(),
	)))

	// public routes
	r.Route("/", public.Route(ctx, public.NewState(
		ctx,
		cfg,
		dpass,
		newsCache,
		executor,
		storage,
		searchService,
	)))

	// setup the http server
	conf := cfg.Conf()
	server := &http.Server{
		Addr:              conf.Website.WebsiteAddr.String(),
		Handler:           r,
		ReadHeaderTimeout: time.Second * 5,
	}

	// read any FDs from a previous process
	fdstorage := fdstore.NewStoreListenFDs()

	// get a listener for the website server
	ln, _, err := util.RestoreOrListen(fdstorage, "website", "tcp", server.Addr)
	if err != nil {
		return err
	}

	zerolog.Ctx(ctx).Info().Str("address", server.Addr).Msg("website started listening")

	// add the listener to the storage again for the next time we restart
	_ = fdstorage.AddListener(ln, "website", []byte(server.Addr))

	errCh := make(chan error, 1)
	go func() {
		errCh <- server.Serve(ln)
	}()

	select {
	case <-ctx.Done():
		return server.Close()
	case <-util.Signal(syscall.SIGUSR2):
		if err := fdstorage.Send(); err != nil {
			zerolog.Ctx(ctx).Error().Err(err).Msg("failed to send store")
		}
		return server.Close()
	case err = <-errCh:
		return err
	}
}

// RedirectLegacyStream redirects a request to the (new) icecast stream url
func RedirectLegacyStream(cfg config.Config) http.HandlerFunc {
	redirectUrl := config.Value(cfg, func(c config.Config) string {
		url := c.Conf().Website.PublicStreamURL
		url = strings.TrimPrefix(url, "https:")
		url = strings.TrimPrefix(url, "http:")
		return url
	})
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Location", redirectUrl())
		w.WriteHeader(http.StatusMovedPermanently)
	}
}

// removePortFromAddress is a middleware that should be behind RealIP to
// remove any potential port number that is present on r.RemoteAddr, since
// we use it to identify users and don't want the port to be used in those
// cases.
//
// This middleware will panic if the address is unparseable by net.SplitHostPort
func removePortFromAddress(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.RemoteAddr == "" {
			next.ServeHTTP(w, r)
			return
		}

		host, _, err := net.SplitHostPort(r.RemoteAddr)
		if err == nil {
			r.RemoteAddr = host
		}
		next.ServeHTTP(w, r)
	})
}

func skipCSRFProtection(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/admin/telemetry") {
			r = csrf.UnsafeSkipCheck(r)
		}
		next.ServeHTTP(w, r)
	})
}

func AssetsHandler(assetsPath string, site *templates.Site) http.Handler {
	base := os.DirFS(assetsPath)

	return http.FileServer(http.FS(assetsFS{base, site}))
}

type assetsFS struct {
	base fs.FS
	site interface {
		Theme(name string) templates.ThemeBundle
	}
}

func (fsys assetsFS) Open(name string) (fs.File, error) {
	// try our valkyrie assets first
	f, err := fsys.base.Open(name)
	if err == nil {
		return f, nil
	}

	// if it errored but wasn't a file not existing error we just
	// return the error as is
	if !errors.IsE(err, fs.ErrNotExist) {
		return nil, err
	}

	// otherwise, no file exists in the base assets, so try and find it
	// in theme specific assets
	theme, rest, found := strings.Cut(name, "/")
	if !found {
		// if there was no cut it means we don't have anything behind the separator so
		// there is no name to even try, just NotExist.
		return nil, fs.ErrNotExist
	}

	// find theme and pass through the assets fs
	return fsys.site.Theme(theme).Assets().Open(rest)
}
