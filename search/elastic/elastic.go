package elastic

import (
	"context"
	"encoding/json"
	"log"

	radio "github.com/R-a-dio/valkyrie"
	"github.com/R-a-dio/valkyrie/config"
	"github.com/R-a-dio/valkyrie/errors"
	"github.com/R-a-dio/valkyrie/search"

	"github.com/olivere/elastic"
)

func init() {
	search.Register("elastic", NewSearchService)
}

const (
	songSearchIndex = "song-database"
	songSearchType  = "track"
	songMapping     = `
{
	"mappings": {
		"track": {
			"properties": {
				  "TrackID": {
					"type": "keyword"
				  },
				  "Title": {
					"type": "text",
					"norms": false
				  },
				  "Album": {
					"type": "text",
					"norms": false
				  },
				  "Artist": {
					"type": "text",
					"norms": false
				  },
				  "Tags": {
					"type": "text",
					"norms": false
				  },
				  "Hash": {
					"type": "keyword"
				  },
				  "LastEditor": {
					"type": "keyword"
				  },
				  "LastPlayed": {
					"type": "date"
				  },
				  "LastRequested": {
					"type": "date"
				  },
				  "Length": {
					"type": "long"
				  },
				  "NeedReupload": {
					"type": "boolean"
				  },
				  "Priority": {
					"type": "long"
				  },
				  "RequestCount": {
					"type": "long"
				  },
				  "RequestDelay": {
					"type": "long"
				  },
				  "Acceptor": {
					"type": "keyword"
				  },
				  "Usable": {
					"type": "boolean"
				  }
			}
		}
	}
}
	`
)

// NewSearchService is a wrapper around NewElasticSearchService to return a
// radio.SearchService type instead
func NewSearchService(cfg config.Config) (radio.SearchService, error) {
	return NewElasticSearchService(cfg)
}

// NewElasticSearchService returns a new radio.SearchService that calls into
// an elasticsearch instance for the implementation
func NewElasticSearchService(cfg config.Config) (*SearchService, error) {
	const op errors.Op = "elastic/NewElasticSearchService"

	conf := cfg.Conf()

	log.Printf("search: elastic: trying to connect to %s", conf.Elastic.URL)
	client, err := elastic.NewClient(
		elastic.SetURL(conf.Elastic.URL),
		elastic.SetSniff(false),
	)
	if err != nil {
		return nil, errors.E(op, err)
	}

	version, err := client.ElasticsearchVersion(conf.Elastic.URL)
	if err != nil {
		return nil, errors.E(op, err)
	}

	log.Printf("search: elastic: using elasticsearch on %s with version %s", conf.Elastic.URL, version)
	return &SearchService{
		es: client,
	}, nil
}

var _ radio.SearchService = &SearchService{}

// SearchService implements radio.SearchService with an elasticsearch backend
type SearchService struct {
	es *elastic.Client
}

// CreateIndex creates all indices used by the service, it returns an error if the indices
// already exist
func (ss *SearchService) CreateIndex(ctx context.Context) error {
	const op errors.Op = "elastic/ElasticService.CreateIndex"
	exists, err := ss.es.IndexExists(songSearchIndex).Do(ctx)
	if err != nil {
		return errors.E(op, err)
	}
	if exists {
		return errors.E(errors.SearchIndexExists, op)
	}

	create, err := ss.es.CreateIndex(songSearchIndex).BodyString(songMapping).Do(ctx)
	if err != nil {
		return errors.E(op, err)
	}
	if !create.Acknowledged {
		return errors.E(op, "index creation not acknowledged")
	}

	return nil
}

// DeleteIndex deletes all indices created by CreateIndex
func (ss *SearchService) DeleteIndex(ctx context.Context) error {
	const op errors.Op = "elastic/SearchService.DeleteIndex"

	del, err := ss.es.DeleteIndex(songSearchIndex).Do(ctx)
	if err != nil {
		return errors.E(op, err)
	}
	if !del.Acknowledged {
		return errors.E(op, "index deletion not acknowledged")
	}

	return nil
}

// Search implements radio.SearchService
func (ss *SearchService) Search(ctx context.Context, query string,
	limit int, offset int) (*radio.SearchResult, error) {
	const op errors.Op = "elastic/SearchService.Search"
	esQuery := ss.createSearchQuery(query)

	action := ss.es.Search().Index(songSearchIndex).
		Query(esQuery).
		Sort("Priority", false). // sort by our custom priority field
		From(offset).Size(limit).
		Pretty(true)

	usableOnly := true
	if usableOnly {
		action = action.PostFilter(
			elastic.NewBoolQuery().Must(
				elastic.NewTermQuery("Usable", true),
			),
		)
	}

	res, err := action.Do(ctx)
	if err != nil {
		return nil, errors.E(op, err)
	}

	if res.Hits == nil || len(res.Hits.Hits) == 0 {
		return nil, errors.E(op, errors.SearchNoResults, errors.Info(query))
	}

	songs := make([]radio.Song, len(res.Hits.Hits))
	for i, hit := range res.Hits.Hits {
		err := json.Unmarshal(*hit.Source, &songs[i])
		if err != nil {
			return nil, errors.E(op, err)
		}
		songs[i].FillMetadata()
	}

	result := &radio.SearchResult{
		Songs:     songs,
		TotalHits: int(res.TotalHits()),
	}
	return result, nil
}

func (ss *SearchService) createSearchQuery(query string) elastic.Query {
	/* query_string version, this is what has been done historically
	return elastic.NewQueryStringQuery(query).
		Field("title").Field("artist").Field("album").Field("tags").Field("track_id").
		DefaultOperator("AND") */
	return elastic.NewMultiMatchQuery(query, "Title", "Artist", "Album", "Tags", "TrackID").
		Type("cross_fields").Operator("AND")
}

// Update implements radio.SearchService
func (ss *SearchService) Update(ctx context.Context, songs ...radio.Song) error {
	const op errors.Op = "elastic/SearchService.Update"

	// if we only get a single argument, don't do a bulk update
	if len(songs) == 1 {
		song := songs[0]

		_, err := ss.es.Update().
			Index(songSearchIndex).
			Type(songSearchType).
			Id(song.TrackID.String()).
			Doc(song).DocAsUpsert(true).
			Do(ctx)
		if err != nil {
			return errors.E(op, err)
		}

		log.Printf("%s: indexed 1 song", op)
		return nil
	}

	// if more than one, we do a bulk update
	bulk := ss.es.Bulk()

	for _, song := range songs {
		if !song.HasTrack() {
			return errors.E(op, errors.SongWithoutTrack, song)
		}

		bulk = bulk.Add(ss.createUpsertRequest(song))
	}

	resp, err := bulk.Do(ctx)
	if err != nil {
		return errors.E(op, err)
	}

	log.Printf("search: elastic: indexed %d songs", len(resp.Items))
	return nil
}

func (ss *SearchService) createUpsertRequest(song radio.Song) elastic.BulkableRequest {
	return elastic.NewBulkUpdateRequest().
		Index(songSearchIndex).
		Type(songSearchType).
		Id(song.TrackID.String()).
		Doc(song).DocAsUpsert(true)
}

// Delete implements radio.SearchService
func (ss *SearchService) Delete(ctx context.Context, songs ...radio.Song) error {
	const op errors.Op = "elastic/SearchService.Delete"
	bulk := ss.es.Bulk()

	for _, song := range songs {
		if !song.HasTrack() {
			return errors.E(op, errors.SongWithoutTrack, song)
		}
		bulk = bulk.Add(ss.createDeleteRequest(song))
	}

	resp, err := bulk.Do(ctx)
	if err != nil {
		return errors.E(op, err)
	}

	log.Printf("search: elastic: deleted %d songs", len(resp.Items))
	return nil
}

func (ss *SearchService) createDeleteRequest(song radio.Song) elastic.BulkableRequest {
	return elastic.NewBulkDeleteRequest().
		Index(songSearchIndex).
		Type(songSearchType).
		Id(song.TrackID.String())
}
