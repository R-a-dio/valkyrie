package bleve

import (
	"context"
	"net/http"
	"syscall"

	"github.com/R-a-dio/valkyrie/config"
	"github.com/R-a-dio/valkyrie/errors"
	"github.com/R-a-dio/valkyrie/search"
	"github.com/R-a-dio/valkyrie/util"
	"github.com/R-a-dio/valkyrie/website"
	"github.com/Wessie/fdstore"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/hlog"
)

func Execute(ctx context.Context, cfg config.Config) error {
	const op errors.Op = "search/bleve.Execute"

	idx, err := NewIndex(cfg.Conf().Search.IndexPath)
	if err != nil {
		return errors.E(op, err)
	}
	defer idx.index.Close()

	srv, err := NewServer(ctx, idx)
	if err != nil {
		return errors.E(op, err)
	}
	defer srv.Close()

	fdstorage := fdstore.NewStoreListenFDs()

	endpoint := cfg.Conf().Search.Endpoint.URL()
	ln, _, err := util.RestoreOrListen(fdstorage, "bleve", "tcp", endpoint.Host)
	if err != nil {
		return errors.E(op, err)
	}

	errCh := make(chan error, 1)
	go func() {
		errCh <- srv.Serve(ln)
	}()

	select {
	case <-ctx.Done():
		return srv.Close()
	case <-util.Signal(syscall.SIGUSR2):
		zerolog.Ctx(ctx).Info().Ctx(ctx).Msg("SIGUSR2 received")
		err := fdstorage.AddListener(ln, "bleve", nil)
		if err != nil {
			zerolog.Ctx(ctx).Error().Ctx(ctx).Err(err).Msg("failed to store listener")
		}
		if err = fdstorage.Send(); err != nil {
			zerolog.Ctx(ctx).Error().Ctx(ctx).Err(err).Msg("failed to send store")
		}
		return srv.Close()
	case err := <-errCh:
		return err
	}
}

func init() {
	search.Register("bleve", true, Open)
}

func NewServer(ctx context.Context, idx *indexWrap) (*http.Server, error) {
	logger := zerolog.Ctx(ctx)
	r := website.NewRouter()
	r.Use(middleware.Recoverer)
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

	r.Get(searchPath, SearchHandler(idx))
	r.Get(searchJSONPath, SearchJSONHandler(idx))
	r.Get(extendedPath, ExtendedSearchHandler(idx))
	r.Get(indexStatsPath, IndexStatsHandler(idx))
	r.Post(deletePath, DeleteHandler(idx))
	r.Post(updatePath, UpdateHandler(idx))

	srv := &http.Server{
		Handler: r,
	}
	return srv, nil
}
