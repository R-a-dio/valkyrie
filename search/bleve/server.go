package bleve

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strconv"
	"time"
	"unsafe"

	radio "github.com/R-a-dio/valkyrie"
	"github.com/R-a-dio/valkyrie/errors"
	"github.com/R-a-dio/valkyrie/util/pool"
	"github.com/R-a-dio/valkyrie/website"
	"github.com/blevesearch/bleve/v2"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/hlog"
	"github.com/vmihailenco/msgpack/v4"
)

var (
	DefaultLimit  = 20
	DefaultOffset = 0
	DefaultSort   = []string{"-priority", "_score"}
	dataField     = []string{"data"}

	searchPath     = "/search"
	searchJSONPath = "/search_json"
	extendedPath   = "/search_extended"
	indexStatsPath = "/index_stats"
	updatePath     = "/update"
	deletePath     = "/delete"
)

var _cache = cache{
	enc: pool.NewPool(func() *msgpack.Encoder {
		return msgpack.NewEncoder(nil).UseJSONTag(true)
	}),
	dec: pool.NewPool(func() *msgpack.Decoder {
		return msgpack.NewDecoder(nil).UseJSONTag(true)
	}),
}

type cache struct {
	enc *pool.Pool[*msgpack.Encoder]
	dec *pool.Pool[*msgpack.Decoder]
}

func zerologLoggerFunc(r *http.Request, status, size int, duration time.Duration) {
	hlog.FromRequest(r).Info().
		Int("status_code", status).
		Int("response_size_bytes", size).
		Dur("elapsed_ms", duration).
		Msg("http request")
}

func NewServer(ctx context.Context, idx *index) (*http.Server, error) {
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

func DeleteHandler(idx *index) http.HandlerFunc {
	const op errors.Op = "search/bleve.DeleteHandler"

	return func(w http.ResponseWriter, r *http.Request) {
		var tids []radio.TrackID

		err := msgpack.NewDecoder(r.Body).Decode(&tids)
		if err != nil {
			err = errors.E(op, err)
			hlog.FromRequest(r).Error().Err(err).Msg("decode error")
			return
		}

		err = idx.Delete(r.Context(), tids)
		if err != nil {
			err = errors.E(op, err)
			hlog.FromRequest(r).Error().Err(err).Msg("delete error")
			return
		}
		w.WriteHeader(http.StatusOK)
	}
}

func UpdateHandler(idx *index) http.HandlerFunc {
	const op errors.Op = "search/bleve.UpdateHandler"

	return func(w http.ResponseWriter, r *http.Request) {
		var songs []radio.Song

		err := msgpack.NewDecoder(r.Body).Decode(&songs)
		if err != nil {
			err = errors.E(op, err)
			hlog.FromRequest(r).Error().Err(err).Msg("decode error")
			return
		}

		err = idx.Index(r.Context(), songs)
		if err != nil {
			err = errors.E(op, err)
			hlog.FromRequest(r).Error().Err(err).Msg("index error")
			return
		}
		w.WriteHeader(http.StatusOK)
	}
}

func AsIntOrDefault(s string, def int) int {
	i, err := strconv.Atoi(s)
	if err != nil {
		return def
	}
	return i
}

func IndexStatsHandler(idx *index) http.HandlerFunc {
	const op errors.Op = "search/bleve.IndexStatsHandler"

	return func(w http.ResponseWriter, r *http.Request) {
		stats := idx.index.Stats()
		enc := json.NewEncoder(w)
		enc.SetIndent("", "  ")
		enc.Encode(stats)
	}
}
func SearchHandler(idx *index) http.HandlerFunc {
	const op errors.Op = "search/bleve.SearchHandler"

	return func(w http.ResponseWriter, r *http.Request) {
		result, err := idx.SearchFromRequest(r)
		if err != nil {
			err = errors.E(op, err)
			hlog.FromRequest(r).Error().Err(err).Msg("failed to search")
			w.WriteHeader(http.StatusInternalServerError)
			_ = encodeError(w, err)
			return
		}

		err = encodeResult(w, result)
		if err != nil {
			err = errors.E(op, err)
			hlog.FromRequest(r).Error().Err(err).Msg("failed to encode")
			return
		}
	}
}

func SearchJSONHandler(idx *index) http.HandlerFunc {
	const op errors.Op = "search/bleve.SearchJSONHandler"

	return func(w http.ResponseWriter, r *http.Request) {
		result, err := idx.SearchFromRequest(r)
		if err != nil {
			err = errors.E(op, err)
			hlog.FromRequest(r).Error().Err(err).Msg("failed to search")
			w.WriteHeader(http.StatusInternalServerError)
			_ = json.NewEncoder(w).Encode(&SearchError{
				Err: err.Error(),
			})
			return
		}

		enc := json.NewEncoder(w)
		enc.SetIndent("", "  ")
		err = enc.Encode(result)
		if err != nil {
			err = errors.E(op, err)
			hlog.FromRequest(r).Error().Err(err).Msg("failed to encode")
			return
		}
	}
}

func encodeResult(dst io.Writer, result *bleve.SearchResult) error {
	const op errors.Op = "search/bleve.encodeResult"

	enc := _cache.enc.Get()
	enc.Reset(dst)
	defer _cache.enc.Put(enc)
	if err := enc.Encode(result); err != nil {
		return errors.E(op, err)
	}
	return nil
}

func decodeResult(src io.Reader, result *bleve.SearchResult) error {
	const op errors.Op = "search/bleve.decodeResult"

	dec := _cache.dec.Get()
	dec.Reset(src)
	defer _cache.dec.Put(dec)
	if err := dec.Decode(result); err != nil {
		return errors.E(op, err)
	}
	return nil
}

func encodeError(dst io.Writer, err error) error {
	const op errors.Op = "search/bleve.encodeError"

	var errString = "<nil>"
	if err != nil {
		errString = err.Error()
	}

	se := &SearchError{
		Err: errString,
	}

	enc := _cache.enc.Get()
	enc.Reset(dst)
	defer _cache.enc.Put(enc)
	if err := enc.Encode(se); err != nil {
		return errors.E(op, err)
	}
	return nil
}

func decodeError(src io.Reader) *SearchError {
	const op errors.Op = "search/bleve.decodeError"
	var se SearchError

	dec := _cache.dec.Get()
	dec.Reset(src)
	defer _cache.dec.Put(dec)
	if err := dec.Decode(&se); err != nil {
		return &SearchError{
			Err: errors.E(op, err).Error(),
		}
	}
	return &se
}

func bleveToRadio(result *bleve.SearchResult) (*radio.SearchResult, error) {
	const op errors.Op = "search/bleve.bleveToRadio"

	var res radio.SearchResult

	res.TotalHits = int(result.Total)
	res.Songs = make([]radio.Song, len(result.Hits))
	for i, hit := range result.Hits {
		tmp, ok := hit.Fields["data"].(string)
		if !ok {
			continue
		}
		data := unsafe.Slice(unsafe.StringData(tmp), len(tmp))
		err := msgpack.Unmarshal(data, &res.Songs[i])
		if err != nil {
			return nil, errors.E(op, err)
		}
	}

	return &res, nil
}

func ExtendedSearchHandler(idx *index) http.HandlerFunc {
	return nil
}
