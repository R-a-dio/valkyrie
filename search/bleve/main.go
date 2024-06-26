package bleve

import (
	"context"
	"net/http"
	"syscall"
	"time"

	radio "github.com/R-a-dio/valkyrie"
	"github.com/R-a-dio/valkyrie/config"
	"github.com/R-a-dio/valkyrie/errors"
	"github.com/R-a-dio/valkyrie/search"
	"github.com/R-a-dio/valkyrie/util"
	"github.com/Wessie/fdstore"
	"github.com/blevesearch/bleve/v2"
	"github.com/blevesearch/bleve/v2/mapping"
	"github.com/blevesearch/bleve/v2/search/query"
	"github.com/rs/zerolog"
	"github.com/vmihailenco/msgpack/v4"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
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
	case <-util.Signal(syscall.SIGHUP):
		err := fdstorage.AddListener(ln, "bleve", nil)
		if err != nil {
			zerolog.Ctx(ctx).Error().Err(err).Msg("failed to store listener")
		}
		if err = fdstorage.Send(); err != nil {
			zerolog.Ctx(ctx).Error().Err(err).Msg("failed to send store")
		}
		return srv.Close()
	case err := <-errCh:
		return err
	}
}

type indexSong struct {
	// main fields we're searching through
	Title  string `bleve:"title"`
	Artist string `bleve:"artist"`
	Album  string `bleve:"album"`
	Tags   string `bleve:"tags"`
	// time fields
	LastRequested time.Time `bleve:"lastrequested"`
	LastPlayed    time.Time `bleve:"lastplayed"`
	// keyword fields
	ID       int    `bleve:"id"`
	Acceptor string `bleve:"acceptor"`
	Editor   string `bleve:"editor"`
	// sorting fields
	Priority     int `bleve:"priority"`
	RequestCount int `bleve:"requestcount"`
	// actual song data
	Data string `bleve:"data"`
}

func (is *indexSong) BleveType() string {
	return "song"
}

func toIndexSong(s radio.Song) *indexSong {
	data, _ := msgpack.Marshal(s)

	return &indexSong{
		Title:         s.Title,
		Artist:        s.Artist,
		Album:         s.Album,
		Tags:          s.Tags,
		LastRequested: s.LastRequested,
		LastPlayed:    s.LastPlayed,
		ID:            int(s.TrackID),
		Acceptor:      s.Acceptor,
		Editor:        s.LastEditor,
		Priority:      s.Priority,
		RequestCount:  s.RequestCount,
		Data:          string(data),
	}
}

type index struct {
	index bleve.Index
}

func (b *index) SearchFromRequest(r *http.Request) (*bleve.SearchResult, error) {
	const op errors.Op = "search/bleve.SearchFromRequest"

	raw := r.FormValue("q")
	limit := AsIntOrDefault(r.FormValue("limit"), DefaultLimit)
	offset := AsIntOrDefault(r.FormValue("offset"), DefaultOffset)

	res, err := b.Search(r.Context(), raw, limit, offset)
	if err != nil {
		return nil, errors.E(op, err)
	}
	return res, nil
}

func (b *index) Search(ctx context.Context, raw string, limit, offset int) (*bleve.SearchResult, error) {
	const op errors.Op = "search/bleve.Search"
	ctx, span := otel.Tracer("").Start(ctx, string(op))
	defer span.End()

	query, err := NewQuery(ctx, raw)
	if err != nil {
		return nil, errors.E(op, err)
	}

	req := bleve.NewSearchRequestOptions(query, limit, offset, false)
	req.SortBy(DefaultSort)
	req.Fields = dataField

	result, err := b.index.SearchInContext(ctx, req)
	if err != nil {
		return nil, errors.E(op, err)
	}
	return result, nil
}

func (b *index) Index(ctx context.Context, songs []radio.Song) error {
	const op errors.Op = "search/bleve.Index"
	ctx, span := otel.Tracer("").Start(ctx, string(op))
	defer span.End()

	span.SetAttributes(attribute.KeyValue{
		Key:   "song_count",
		Value: attribute.IntValue(len(songs)),
	})

	batch := b.index.NewBatch()
	for _, song := range songs {
		isong := toIndexSong(song)
		err := batch.Index(song.TrackID.String(), isong)
		if err != nil {
			return errors.E(op, err)
		}
	}
	err := b.index.Batch(batch)
	if err != nil {
		return errors.E(op, err)
	}
	return nil
}

func (b *index) Delete(ctx context.Context, tids []radio.TrackID) error {
	const op errors.Op = "search/bleve.Delete"
	ctx, span := otel.Tracer("").Start(ctx, string(op))
	defer span.End()

	batch := b.index.NewBatch()
	for _, tid := range tids {
		batch.Delete(tid.String())
	}
	err := b.index.Batch(batch)
	if err != nil {
		return errors.E(op, err)
	}
	return nil
}

func init() {
	search.Register("bleve", true, Open)
}

func mixedTextMapping() *mapping.FieldMapping {
	m := bleve.NewTextFieldMapping()
	m.Analyzer = "radio"
	m.Store = false
	m.Index = true
	return m
}

func constructIndexMapping() (mapping.IndexMapping, error) {
	im := bleve.NewIndexMapping()
	im.DefaultType = "song"

	// create a mapping for our radio.Song type
	sm := bleve.NewDocumentStaticMapping()
	sm.StructTagKey = "bleve"
	sm.DefaultAnalyzer = "radio-query"

	title := mixedTextMapping()
	sm.AddFieldMappingsAt("title", title)
	artist := mixedTextMapping()
	sm.AddFieldMappingsAt("artist", artist)
	album := mixedTextMapping()
	sm.AddFieldMappingsAt("album", album)
	tags := mixedTextMapping()
	sm.AddFieldMappingsAt("tags", tags)

	acceptor := bleve.NewKeywordFieldMapping()
	acceptor.Index = true
	acceptor.Store = false
	acceptor.IncludeTermVectors = false
	acceptor.IncludeInAll = false
	sm.AddFieldMappingsAt("acceptor", acceptor)

	editor := bleve.NewKeywordFieldMapping()
	editor.Index = true
	editor.Store = false
	editor.IncludeTermVectors = false
	editor.IncludeInAll = false
	sm.AddFieldMappingsAt("editor", editor)

	priority := bleve.NewNumericFieldMapping()
	priority.Index = true
	priority.Store = false
	sm.AddFieldMappingsAt("priority", priority)

	id := bleve.NewNumericFieldMapping()
	id.Index = true
	id.Store = false
	sm.AddFieldMappingsAt("id", id)

	lr := bleve.NewDateTimeFieldMapping()
	lr.Index = true
	lr.Store = false
	sm.AddFieldMappingsAt("lastrequested", lr)

	lp := bleve.NewDateTimeFieldMapping()
	lp.Index = true
	lp.Store = false
	sm.AddFieldMappingsAt("lastplayed", lp)

	data := bleve.NewTextFieldMapping()
	data.Index = false
	data.Store = true
	data.Analyzer = "keyword"
	sm.AddFieldMappingsAt("data", data)

	// register the song mapping
	im.AddDocumentMapping("song", sm)
	return im, im.Validate()
}

func Open(ctx context.Context, cfg config.Config) (radio.SearchService, error) {
	const op errors.Op = "bleve/Open"

	return NewClient(cfg.Conf().Search.Endpoint.URL()), nil
}

func NewIndex(indexPath string) (*index, error) {
	const op errors.Op = "bleve.NewIndex"

	idx, err := bleve.Open(indexPath)
	if err == nil {
		// happy path, we have an index and opened it
		return &index{idx}, nil
	}

	// check if error was not-exist
	if !errors.IsE(err, bleve.ErrorIndexPathDoesNotExist) {
		return nil, errors.E(op, err)
	}

	// if it was, create a new index instead
	mapping, err := constructIndexMapping()
	if err != nil {
		return nil, errors.E(op, err)
	}

	idx, err = bleve.New(indexPath, mapping)
	if err != nil {
		return nil, errors.E(op, err)
	}
	return &index{idx}, nil
}

func NewQuery(ctx context.Context, s string) (query.Query, error) {
	const op errors.Op = "search/bleve.NewQuery"
	ctx, span := otel.Tracer("").Start(ctx, string(op))
	defer span.End()

	qsq := bleve.NewQueryStringQuery(s)
	q, err := qsq.Parse()
	if err != nil {
		return nil, err
	}

	bq, ok := q.(*query.BooleanQuery)
	if !ok {
		return q, nil
	}

	dq, ok := bq.Should.(*query.DisjunctionQuery)
	if !ok {
		return q, nil
	}

	cq, ok := bq.Must.(*query.ConjunctionQuery)
	if !ok {
		return q, nil
	}
	// move the disjuncts (OR) into the conjuncts (AND) query set
	cq.AddQuery(dq.Disjuncts...)
	// add a bit of fuzziness to queries that support it
	//addFuzzy(cq.Conjuncts)

	bq.Should = nil
	return bq, nil
}

func addFuzzy(qs []query.Query) {
	var fuzzyMin = 3
	for _, q := range qs {
		switch fq := q.(type) {
		case *query.MatchQuery:
			if len(fq.Match) > fuzzyMin {
				fq.SetFuzziness(1)
			}
		case *query.FuzzyQuery:
			if len(fq.Term) > fuzzyMin && fq.Fuzziness == 0 {
				fq.SetFuzziness(1)
			}
		case *query.MatchPhraseQuery:
			if len(fq.MatchPhrase) > fuzzyMin {
				fq.SetFuzziness(1)
			}
		}
	}
}
