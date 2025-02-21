package bleve

import (
	"context"
	"net/http"
	"strings"
	"time"

	radio "github.com/R-a-dio/valkyrie"
	"github.com/R-a-dio/valkyrie/config"
	"github.com/R-a-dio/valkyrie/errors"
	"github.com/blevesearch/bleve/v2"
	"github.com/blevesearch/bleve/v2/document"
	"github.com/blevesearch/bleve/v2/mapping"
	index "github.com/blevesearch/bleve_index_api"
	"github.com/vmihailenco/msgpack/v4"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
)

const (
	radioCompositeField = "_radio"
	exactCompositeField = "_exact"
)

// indexText holds fields we index multiple times with different analyzers
type indexText struct {
	// main fields we're searching through
	Title  string `bleve:"title"`
	Artist string `bleve:"artist"`
	Album  string `bleve:"album"`
	Tags   string `bleve:"tags"`
}

// indexSort holds fields we want to sort on, but have specific requirements
// for that differ from standard bleve behavior
type indexSort struct {
	Title  string `bleve:"title"`
	Artist string `bleve:"artist"`
	Album  string `bleve:"album"`
	Tags   string `bleve:"tags"`
	ID     int    `bleve:"id"`
}

// indexSong is the structure of the bleve document
type indexSong struct {
	// fields to index with radio analyzer
	Radio indexText `bleve:"radio"`
	// fields to index with exact analyzer
	Exact indexText `bleve:"exact"`

	// time fields
	LastRequested time.Time `bleve:"lr"`
	LastPlayed    time.Time `bleve:"lp"`
	// keyword fields
	ID       string `bleve:"id"`
	Acceptor string `bleve:"acceptor"`
	Editor   string `bleve:"editor"`
	// sorting fields
	Sort         indexSort `bleve:"sort"`
	Priority     int       `bleve:"priority"`
	RequestCount int       `bleve:"rc"`
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

	return &indexSong{
		Radio:         text,
		Exact:         text,
		LastRequested: s.LastRequested,
		LastPlayed:    s.LastPlayed,
		ID:            s.TrackID.String(),
		Sort: indexSort{
			Title:  s.Title,
			Artist: s.Artist,
			Album:  s.Album,
			Tags:   s.Tags,
			ID:     int(s.TrackID),
		},
		Acceptor:     s.Acceptor,
		Editor:       s.LastEditor,
		Priority:     s.Priority,
		RequestCount: s.RequestCount,
		Data:         string(data),
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

	req := NewSearchRequest(query, limit, offset)

	result, err := b.index.SearchInContext(ctx, req)
	if err != nil {
		return nil, errors.E(op, err)
	}
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
		doc, err := b.createDocument(song)
		if err != nil {
			return errors.E(op, err)
		}

		err = batch.IndexAdvanced(doc)
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

func (b *indexWrap) createDocument(song radio.Song) (*document.Document, error) {
	doc := document.NewDocument(song.TrackID.String())
	// first run us through the normal mapping, this will generate the usual bleve document
	err := b.index.Mapping().MapDocument(doc, toIndexSong(song))
	if err != nil {
		return nil, err
	}

	// now add our own special sauce fields

	// first we need to collect what fields we're including
	radioInclude := getFieldsWithPrefix("radio.", doc)
	exactInclude := getFieldsWithPrefix("exact.", doc)

	radioField := document.NewCompositeFieldWithIndexingOptions(
		radioCompositeField,
		false,
		radioInclude,
		[]string{},
		index.IndexField|index.IncludeTermVectors,
	)
	exactField := document.NewCompositeFieldWithIndexingOptions(
		exactCompositeField,
		false,
		exactInclude,
		[]string{},
		index.IndexField|index.IncludeTermVectors,
	)

	doc.AddField(radioField)
	doc.AddField(exactField)
	return doc, nil
}

// getFieldsWithPrefix returns all the fields that have the prefix given
func getFieldsWithPrefix(prefix string, doc *document.Document) []string {
	var res []string
	for _, f := range doc.Fields {
		if strings.HasPrefix(f.Name(), prefix) {
			res = append(res, f.Name())
		}
	}
	return res
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

func newTextMapping() *mapping.FieldMapping {
	fm := bleve.NewTextFieldMapping()
	fm.Store = false
	return fm
}

func newSortMapping() *mapping.FieldMapping {
	fm := bleve.NewTextFieldMapping()
	fm.Store = false
	return fm
}

func constructIndexMapping() (mapping.IndexMapping, error) {
	im := bleve.NewIndexMapping()
	im.DefaultType = "song"

	// create a mapping for our radio.Song type
	sm := bleve.NewDocumentStaticMapping()
	sm.StructTagKey = "bleve"
	sm.DefaultAnalyzer = radioAnalyzerName
	// disable the default _all field
	sm.AddSubDocumentMapping("_all", bleve.NewDocumentDisabledMapping())

	// create the radio submapping
	rm := bleve.NewDocumentStaticMapping()
	rm.StructTagKey = "bleve"
	rm.DefaultAnalyzer = radioAnalyzerName
	rm.AddFieldMappingsAt("title", newTextMapping())
	rm.AddFieldMappingsAt("artist", newTextMapping())
	rm.AddFieldMappingsAt("album", newTextMapping())
	rm.AddFieldMappingsAt("tags", newTextMapping())

	sm.AddSubDocumentMapping("radio", rm)

	// create the exact submapping
	exact := bleve.NewDocumentStaticMapping()
	exact.StructTagKey = "bleve"
	exact.DefaultAnalyzer = exactAnalyzerName
	exact.AddFieldMappingsAt("title", newTextMapping())
	exact.AddFieldMappingsAt("artist", newTextMapping())
	exact.AddFieldMappingsAt("album", newTextMapping())
	exact.AddFieldMappingsAt("tags", newTextMapping())

	sm.AddSubDocumentMapping("exact", exact)

	// create the sort submapping
	sort := bleve.NewDocumentStaticMapping()
	sort.StructTagKey = "bleve"
	sort.DefaultAnalyzer = sortAnalyzerName
	sort.AddFieldMappingsAt("title", newSortMapping())
	sort.AddFieldMappingsAt("artist", newSortMapping())
	sort.AddFieldMappingsAt("album", newSortMapping())
	sort.AddFieldMappingsAt("tags", newSortMapping())
	sortId := bleve.NewNumericFieldMapping()
	sortId.Index = true
	sortId.Store = false
	sort.AddFieldMappingsAt("id", sortId)

	sm.AddSubDocumentMapping("sort", sort)

	// create the rest of the normal mappings
	acceptor := bleve.NewKeywordFieldMapping()
	acceptor.Index = true
	acceptor.Store = false
	acceptor.IncludeTermVectors = false
	sm.AddFieldMappingsAt("acceptor", acceptor)

	editor := bleve.NewKeywordFieldMapping()
	editor.Index = true
	editor.Store = false
	editor.IncludeTermVectors = false
	sm.AddFieldMappingsAt("editor", editor)

	priority := bleve.NewNumericFieldMapping()
	priority.Index = true
	priority.Store = false
	priority.Analyzer = "keyword"
	sm.AddFieldMappingsAt("priority", priority)

	rc := bleve.NewNumericFieldMapping()
	rc.Index = true
	rc.Store = false
	rc.Analyzer = "keyword"
	sm.AddFieldMappingsAt("rc", priority)

	id := bleve.NewKeywordFieldMapping()
	id.Index = true
	id.Store = false
	id.IncludeTermVectors = false
	sm.AddFieldMappingsAt("id", id)

	lr := bleve.NewDateTimeFieldMapping()
	lr.Index = true
	lr.Store = false
	sm.AddFieldMappingsAt("lr", lr)

	lp := bleve.NewDateTimeFieldMapping()
	lp.Index = true
	lp.Store = false
	sm.AddFieldMappingsAt("lp", lp)

	data := bleve.NewKeywordFieldMapping()
	data.Index = false
	data.Store = true
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
