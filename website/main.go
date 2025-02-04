package website

import (
	"context"
	"encoding/json"
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
	"github.com/R-a-dio/valkyrie/templates/functions"
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

// Execute runs a website instance with the configuration given
func Execute(ctx context.Context, cfg config.Config) error {
	const op errors.Op = "website/Execute"
	logger := zerolog.Ctx(ctx)

	// check directory existences
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
	if path := cfg.Conf().MusicPath; path == "" {
		return errors.E(op, "MusicPath is not configured")
	} else {
		// try and create MusicPath
		if err := os.MkdirAll(path, 0664); err != nil {
			return errors.E(op, "failed to create MusicPath", err)
		}
		// and the pending dir where we store pending songs
		if err := os.MkdirAll(filepath.Join(path, "pending"), 0664); err != nil {
			return errors.E(op, "failed to create MusicPath pending dir", err)
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
	templateFuncs := functions.NewStatefulFunctions(cfg, statusValue)
	// construct our templates from files on disk
	siteTemplates, err := templates.FromDirectory(
		cfg.Conf().TemplatePath,
		templateFuncs,
	)
	if err != nil {
		return errors.E(op, err)
	}
	// templates reload on every render in development mode
	siteTemplates.Production = !cfg.Conf().DevelopmentMode
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
		hlog.AccessHandler(util.ZerologLoggerFunc),
	)
	// clean our IP of a port number
	r.Use(removePortFromAddress)
	// recover from panics
	r.Use(vmiddleware.Recoverer)
	// session handling
	sessionManager := vmiddleware.NewSessionManager(ctx, storage)
	r.Use(sessionManager.LoadAndSave)
	// user handling
	authentication := vmiddleware.NewAuthentication(storage, executor, sessionManager)
	r.Use(authentication.UserMiddleware)
	// CSRF token handling
	// fixes a compatibility issue with the PHP api, see middleware documentation
	r.Use(phpapi.MoveTokenToHeaderForRequests)
	// skip CSRF protection for our telemetry backend
	r.Use(skipCSRFProtection)
	// CSRF secret for generating tokens
	csrfKey := []byte(cfg.Conf().Website.CSRFSecret)
	if len(csrfKey) == 0 {
		// no key is a critical error if this is a production environment
		if cfg.Conf().DevelopmentMode {
			logger.Warn().Msg("CSRFSecret is empty, using random key")
			csrfKey, err = secret.NewKey(32)
			if err != nil {
				panic("CSRFSecret is empty and we couldn't generate a random key")
			}
		} else {
			panic("CSRFSecret is not configured and DevelopmentMode==false")
		}
	}
	r.Use(csrf.Protect(csrfKey,
		csrf.Path("/"),
		csrf.Secure(!cfg.Conf().DevelopmentMode), // disable https requirement under dev mode
		csrf.Encoding(base62.StdEncoding),
	))
	// shared input handling, stuff the base template needs
	r.Use(vmiddleware.InputMiddleware(cfg, statusValue, public.NavBar, admin.NavBar))
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
	logger.Info().Ctx(ctx).Str("event", "init").Str("part", "api_v0").Msg("")
	v0, err := phpapi.NewAPI(ctx, cfg, storage, statusValue)
	if err != nil {
		return errors.E(op, err)
	}
	r.Route("/api", v0.Route)
	r.Route(`/request/{TrackID:[0-9]+}`, v0.RequestRoute)

	// version 1 of the api
	logger.Info().Ctx(ctx).Str("event", "init").Str("part", "api_v1").Msg("")
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
		themeValues,
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
	ln, state, err := util.RestoreOrListen(fdstorage, "website", "tcp", server.Addr)
	if err != nil {
		return err
	}
	zerolog.Ctx(ctx).Info().Ctx(ctx).Str("address", server.Addr).Msg("website started listening")

	// restore the state from the previous process
	var ws websiteStorage
	_ = json.Unmarshal(state, &ws)
	themeValues.StoreHoliday(ws.HolidayTheme)

	errCh := make(chan error, 1)
	go func() {
		errCh <- server.Serve(ln)
	}()

	select {
	case <-ctx.Done():
		return server.Close()
	case <-util.Signal(syscall.SIGUSR2):
		// store our "not really persistent" state
		state, _ := json.Marshal(websiteStorage{
			HolidayTheme: themeValues.LoadHoliday(),
		})

		if err := fdstorage.AddListener(ln, "website", state); err != nil {
			zerolog.Ctx(ctx).Error().Ctx(ctx).Err(err).Msg("failed to store listener")
		}
		if err := fdstorage.Send(); err != nil {
			zerolog.Ctx(ctx).Error().Ctx(ctx).Err(err).Msg("failed to send store")
		}
		return server.Close()
	case err = <-errCh:
		return err
	}
}

type websiteStorage struct {
	HolidayTheme radio.ThemeName
}

// RedirectLegacyStream redirects a request to the (new) icecast stream url
func RedirectLegacyStream(cfg config.Config) http.HandlerFunc {
	redirectUrl := config.Value(cfg, func(cfg config.Config) string {
		url := cfg.Conf().Website.PublicStreamURL
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

// skipCSRFProtection removes the requirement of the CSRF token to the paths configured
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
		Theme(name radio.ThemeName) templates.ThemeBundle
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
	return fsys.site.Theme(radio.ThemeName(theme)).Assets().Open(rest)
}
