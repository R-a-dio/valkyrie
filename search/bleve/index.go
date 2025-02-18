package bleve

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	radio "github.com/R-a-dio/valkyrie"
	"github.com/R-a-dio/valkyrie/config"
	"github.com/R-a-dio/valkyrie/errors"
	"github.com/blevesearch/bleve/v2"
	"github.com/blevesearch/bleve/v2/analysis/analyzer/keyword"
	"github.com/blevesearch/bleve/v2/mapping"
	"github.com/blevesearch/bleve/v2/numeric"
	"github.com/blevesearch/bleve/v2/search"
	"github.com/vmihailenco/msgpack/v4"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
)

// prioScoreSort sorts the documents by their priority and score
type prioScoreSort struct {
	prio float64
}

func (s *prioScoreSort) UpdateVisitor(field string, term []byte) {
	if field != "priority" {
		return
	}
	valid, shift := numeric.ValidPrefixCodedTermBytes(term)
	if !valid || shift != 0 {
		return
	}
	prio, _ := numeric.PrefixCoded(term).Int64()
	s.prio = numeric.Int64ToFloat64(prio)
}

func (s *prioScoreSort) Value(a *search.DocumentMatch) string {
	prio := s.prio
	score := a.Score

	// boost sort score if we had a large match score; this means that
	// there were exact matches
	if score > 0.5 {
		prio += 1000
	}

	return fmt.Sprintf("%010d", int(prio))
}

func (s *prioScoreSort) Descending() bool {
	return true
}

func (s *prioScoreSort) RequiresDocID() bool {
	return false
}

func (s *prioScoreSort) RequiresScoring() bool {
	return false
}

func (s *prioScoreSort) RequiresFields() []string {
	return []string{"priority"}
}

func (s *prioScoreSort) Reverse() {
}

func (s prioScoreSort) Copy() search.SearchSort {
	return &s
}

type indexText struct {
	// main fields we're searching through
	Title  string `bleve:"title"`
	Artist string `bleve:"artist"`
	Album  string `bleve:"album"`
	Tags   string `bleve:"tags"`
}

type indexSong struct {
	Ngram  indexText `bleve:"ngram"`
	Ngram_ string    `bleve:"ngram_"`
	Exact  indexText `bleve:"exact"`
	Exact_ string    `bleve:"exact_"`
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

	text := indexText{
		Title:  s.Title,
		Artist: s.Artist,
		Album:  s.Album,
		Tags:   s.Tags,
	}

	all := strings.Join([]string{s.Title, s.Artist, s.Album, s.Tags}, " ")

	return &indexSong{
		Ngram:         text,
		Ngram_:        all,
		Exact:         text,
		Exact_:        all,
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

type indexWrap struct {
	index bleve.Index
}

func (b *indexWrap) Close() error {
	return b.index.Close()
}

func (b *indexWrap) SearchFromRequest(r *http.Request) (*bleve.SearchResult, error) {
	const op errors.Op = "search/bleve.SearchFromRequest"

	raw := r.FormValue("q")
	limit := AsIntOrDefault(r.FormValue("limit"), DefaultLimit)
	offset := AsIntOrDefault(r.FormValue("offset"), DefaultOffset)
	exact := r.FormValue("exact") == "true"

	res, err := b.Search(r.Context(), raw, exact, limit, offset)
	if err != nil {
		return nil, errors.E(op, err)
	}
	return res, nil
}

func (b *indexWrap) Search(ctx context.Context, raw string, exactOnly bool, limit, offset int) (*bleve.SearchResult, error) {
	const op errors.Op = "search/bleve.Search"
	ctx, span := otel.Tracer("bleve").Start(ctx, string(op))
	defer span.End()

	query, err := NewQuery(ctx, raw, exactOnly)
	if err != nil {
		return nil, errors.E(op, err)
	}

	req := bleve.NewSearchRequestOptions(query, limit, offset, false)
	if query.SortField != "" {
		field := query.SortField
		if query.Descending {
			field = "-" + field
		}
		req.SortBy([]string{field})
	} else {
		req.SortByCustom(search.SortOrder{&prioScoreSort{}})
	}
	req.Fields = dataField

	result, err := b.index.SearchInContext(ctx, req)
	if err != nil {
		return nil, errors.E(op, err)
	}
	fmt.Println(result.Hits)
	return result, nil
}

func (b *indexWrap) Index(ctx context.Context, songs []radio.Song) error {
	const op errors.Op = "search/bleve.Index"
	ctx, span := otel.Tracer("bleve").Start(ctx, string(op))
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

func (b *indexWrap) Delete(ctx context.Context, tids []radio.TrackID) error {
	const op errors.Op = "search/bleve.Delete"
	ctx, span := otel.Tracer("bleve").Start(ctx, string(op))
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

func mixedTextMapping(analyzerName string) *mapping.FieldMapping {
	m := bleve.NewTextFieldMapping()
	m.Analyzer = analyzerName
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
	sm.DefaultAnalyzer = radioAnalyzerName

	ngram := bleve.NewDocumentStaticMapping()
	ngram.StructTagKey = "bleve"
	ngram.DefaultAnalyzer = radioAnalyzerName

	title := mixedTextMapping(radioAnalyzerName)
	ngram.AddFieldMappingsAt("title", title)
	artist := mixedTextMapping(radioAnalyzerName)
	ngram.AddFieldMappingsAt("artist", artist)
	album := mixedTextMapping(radioAnalyzerName)
	ngram.AddFieldMappingsAt("album", album)
	tags := mixedTextMapping(radioAnalyzerName)
	ngram.AddFieldMappingsAt("tags", tags)

	sm.AddSubDocumentMapping("ngram", ngram)

	exact := bleve.NewDocumentStaticMapping()
	exact.StructTagKey = "bleve"
	exact.DefaultAnalyzer = exactAnalyzerName

	title_exact := mixedTextMapping(exactAnalyzerName)
	exact.AddFieldMappingsAt("title", title_exact)
	artist_exact := mixedTextMapping(exactAnalyzerName)
	exact.AddFieldMappingsAt("artist", artist_exact)
	album_exact := mixedTextMapping(exactAnalyzerName)
	exact.AddFieldMappingsAt("album", album_exact)
	tags_exact := mixedTextMapping(exactAnalyzerName)
	exact.AddFieldMappingsAt("tags", tags_exact)

	sm.AddSubDocumentMapping("exact", exact)

	ngram_ := mixedTextMapping(radioAnalyzerName)
	sm.AddFieldMappingsAt("ngram_", ngram_)

	exact_ := mixedTextMapping(exactAnalyzerName)
	sm.AddFieldMappingsAt("exact_", exact_)

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
	priority.Analyzer = "keyword"
	sm.AddFieldMappingsAt("priority", priority)

	id := bleve.NewNumericFieldMapping()
	id.Index = true
	id.Store = false
	id.IncludeTermVectors = false
	id.IncludeInAll = true
	id.Analyzer = "keyword"
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
	data.IncludeInAll = false
	data.Analyzer = keyword.Name
	sm.AddFieldMappingsAt("data", data)

	// register the song mapping
	im.AddDocumentMapping("song", sm)
	return im, im.Validate()
}

func Open(ctx context.Context, cfg config.Config) (radio.SearchService, error) {
	return NewClient(cfg.Conf().Search.Endpoint.URL()), nil
}

func NewIndex(indexPath string) (*indexWrap, error) {
	const op errors.Op = "bleve.NewIndex"

	idx, err := bleve.Open(indexPath)
	if err == nil {
		// happy path, we have an index and opened it
		return &indexWrap{idx}, nil
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

	if indexPath == ":memory:" { // support memory-only index for testing purposes
		idx, err = bleve.NewMemOnly(mapping)
	} else {
		idx, err = bleve.New(indexPath, mapping)
	}
	if err != nil {
		return nil, errors.E(op, err)
	}
	return &indexWrap{idx}, nil
}
