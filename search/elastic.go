package search

import (
	"context"
	"encoding/json"
	"log"

	radio "github.com/R-a-dio/valkyrie"
	"github.com/R-a-dio/valkyrie/config"
	"github.com/R-a-dio/valkyrie/errors"

	"github.com/olivere/elastic"
)

const (
	songSearchIndex = "song-database"
	songSearchType  = "track"
	songMapping     = `
{
	"mappings": {
		"track": {
			"properties": {
				  "track_id": {
					"type": "keyword"
				  },
				  "title": {
					"type": "text",
					"norms": false
				  },
				  "album": {
					"type": "text",
					"norms": false
				  },
				  "artist": {
					"type": "text",
					"norms": false
				  },
				  "tags": {
					"type": "text",
					"norms": false
				  },
				  "hash": {
					"type": "keyword"
				  },
				  "last_editor": {
					"type": "keyword"
				  },
				  "last_played": {
					"type": "date"
				  },
				  "last_requested": {
					"type": "date"
				  },
				  "length": {
					"type": "long"
				  },
				  "need_reupload": {
					"type": "boolean"
				  },
				  "priority": {
					"type": "long"
				  },
				  "request_count": {
					"type": "long"
				  },
				  "request_delay": {
					"type": "long"
				  },
				  "acceptor": {
					"type": "keyword"
				  },
				  "usable": {
					"type": "boolean"
				  }
			}
		}
	}
}
	`
)

// NewElasticSearchService returns a new radio.SearchService that calls into
// an elasticsearch instance for the implementation
func NewElasticSearchService(ctx context.Context, cfg config.Config) (*ElasticService, error) {
	const op errors.Op = "search/NewElasticSearchService"

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
	return &ElasticService{
		es: client,
	}, nil
}

var _ radio.SearchService = &ElasticService{}

// ElasticService implements radio.SearchService
type ElasticService struct {
	es *elastic.Client
}

// CreateIndex creates all indices used by the service, it returns an error if the indices
// already exist
func (es *ElasticService) CreateIndex(ctx context.Context) error {
	const op errors.Op = "search/ElasticService.CreateIndex"
	exists, err := es.es.IndexExists(songSearchIndex).Do(ctx)
	if err != nil {
		return errors.E(op, err)
	}
	if exists {
		return errors.E(errors.SearchIndexExists, op)
	}

	create, err := es.es.CreateIndex(songSearchIndex).BodyString(songMapping).Do(ctx)
	if err != nil {
		return errors.E(op, err)
	}
	if !create.Acknowledged {
		return errors.E(op, "index creation not acknowledged")
	}

	return nil
}

// DeleteIndex deletes all indices created by CreateIndex
func (es *ElasticService) DeleteIndex(ctx context.Context) error {
	const op errors.Op = "search/ElasticService.DeleteIndex"

	del, err := es.es.DeleteIndex(songSearchIndex).Do(ctx)
	if err != nil {
		return errors.E(op, err)
	}
	if !del.Acknowledged {
		return errors.E(op, "index deletion not acknowledged")
	}

	return nil
}

// Search implements radio.SearchService
func (es *ElasticService) Search(ctx context.Context, query string, limit int, offset int) ([]radio.Song, error) {
	const op errors.Op = "search/ElasticService.Search"
	esQuery := es.createSearchQuery(query)

	action := es.es.Search().Index(songSearchIndex).
		Query(esQuery).
		Sort("priority", false). // sort by our custom priority field
		From(offset).Size(limit).
		Pretty(true)

	usableOnly := true
	if usableOnly {
		action = action.PostFilter(
			elastic.NewBoolQuery().Must(
				elastic.NewTermQuery("usable", true),
			),
		)
	}

	res, err := action.Do(ctx)
	if err != nil {
		return nil, errors.E(op, err)
	}

	if res.Hits == nil || len(res.Hits.Hits) == 0 {
		return []radio.Song{}, errors.E(op, errors.SearchNoResults, errors.Info(query))
	}

	songs := make([]radio.Song, len(res.Hits.Hits))
	for i, hit := range res.Hits.Hits {
		err := json.Unmarshal(*hit.Source, &songs[i])
		if err != nil {
			return nil, errors.E(op, err)
		}
		songs[i].FillMetadata()
	}
	return songs, nil
}

func (es *ElasticService) createSearchQuery(query string) elastic.Query {
	/* query_string version, this is what has been done historically
	return elastic.NewQueryStringQuery(query).
		Field("title").Field("artist").Field("album").Field("tags").Field("track_id").
		DefaultOperator("AND") */
	return elastic.NewMultiMatchQuery(query, "title", "artist", "album", "tags", "track_id").
		Type("cross_fields").Operator("AND")
}

// Update implements radio.SearchService
func (es *ElasticService) Update(ctx context.Context, songs ...radio.Song) error {
	const op errors.Op = "search/ElasticService.Update"
	bulk := es.es.Bulk()

	for _, song := range songs {
		if !song.HasTrack() {
			return errors.E(op, errors.SongWithoutTrack, song)
		}

		bulk = bulk.Add(es.createUpsertRequest(song))
	}

	resp, err := bulk.Do(ctx)
	if err != nil {
		return errors.E(op, err)
	}

	log.Printf("search: elastic: indexed %d songs", len(resp.Items))
	return nil
}

func (es *ElasticService) createUpsertRequest(song radio.Song) elastic.BulkableRequest {
	return elastic.NewBulkUpdateRequest().
		Index(songSearchIndex).
		Type(songSearchType).
		Id(song.TrackID.String()).
		Doc(song).DocAsUpsert(true)
}

// Delete implements radio.SearchService
func (es *ElasticService) Delete(ctx context.Context, songs ...radio.Song) error {
	const op errors.Op = "search/ElasticService.Delete"
	bulk := es.es.Bulk()

	for _, song := range songs {
		if !song.HasTrack() {
			return errors.E(op, errors.SongWithoutTrack, song)
		}
		bulk = bulk.Add(es.createDeleteRequest(song))
	}

	resp, err := bulk.Do(ctx)
	if err != nil {
		return errors.E(op, err)
	}

	log.Printf("search: elastic: deleted %d songs", len(resp.Items))
	return nil
}

func (es *ElasticService) createDeleteRequest(song radio.Song) elastic.BulkableRequest {
	return elastic.NewBulkDeleteRequest().
		Index(songSearchIndex).
		Type(songSearchType).
		Id(song.TrackID.String())
}
