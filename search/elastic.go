package search

import (
	"context"
	"encoding/json"
	"errors"
	"log"

	radio "github.com/R-a-dio/valkyrie"
	"github.com/R-a-dio/valkyrie/config"

	"github.com/olivere/elastic"
)

const (
	songSearchIndex = "song-database"
	songSearchType  = "track"
)

// NewElasticSearchService returns a new radio.SearchService that calls into
// an elasticsearch instance for the implementation
func NewElasticSearchService(ctx context.Context, cfg config.Config) (radio.SearchService, error) {
	conf := cfg.Conf()

	log.Printf("search: elastic: trying to connect to %s", conf.Elastic.URL)
	client, err := elastic.NewClient(
		elastic.SetURL(conf.Elastic.URL),
		elastic.SetSniff(false),
	)
	if err != nil {
		return nil, err
	}

	version, err := client.ElasticsearchVersion(conf.Elastic.URL)
	if err != nil {
		return nil, err
	}

	log.Printf("search: elastic: using elasticsearch on %s with version %s", conf.Elastic.URL, version)
	return &ElasticService{
		es: client,
	}, nil
}

// ElasticService implements radio.SearchService
type ElasticService struct {
	es *elastic.Client
}

func (es *ElasticService) Search(ctx context.Context, query string, limit int, offset int) ([]radio.Song, error) {
	esQuery := es.createSearchQuery2(query)

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
		return nil, err
	}

	if res.Hits == nil || len(res.Hits.Hits) == 0 {
		return []radio.Song{}, nil
	}

	songs := make([]radio.Song, len(res.Hits.Hits))
	for i, hit := range res.Hits.Hits {
		err := json.Unmarshal(*hit.Source, &songs[i])
		if err != nil {
			return nil, err
		}
		songs[i].FillMetadata()
	}
	return songs, nil
}

func (es *ElasticService) createSearchQuery(query string) elastic.Query {
	return elastic.NewQueryStringQuery(query).
		Field("title").Field("artist").Field("album").Field("tags").Field("id").
		DefaultOperator("AND")
}

func (es *ElasticService) createSearchQuery2(query string) elastic.Query {
	return elastic.NewMultiMatchQuery(query, "title", "artist", "album", "tags", "id").
		Type("cross_fields").Operator("AND")
}

func (es *ElasticService) Update(ctx context.Context, songs ...radio.Song) error {
	bulk := es.es.Bulk()

	for _, song := range songs {
		if !song.HasTrack() {
			return errors.New("received song with no track")
			//return ErrInvalidSong
		}

		bulk = bulk.Add(es.createUpdateRequest(song))
	}

	resp, err := bulk.Do(ctx)
	if err != nil {
		return err
	}

	log.Printf("search: elastic: indexed %d songs", len(resp.Items))
	return nil
}

func (es *ElasticService) createUpdateRequest(song radio.Song) elastic.BulkableRequest {
	return elastic.NewBulkUpdateRequest().
		Index(songSearchIndex).
		Type(songSearchType).
		Id(song.TrackID.String()).
		Doc(song).DocAsUpsert(true)
}

func (es *ElasticService) Delete(ctx context.Context, song radio.Song) error {
	if !song.HasTrack() {
		return errors.New("received song with no track")
		//return ErrInvalidSong
	}

	action := es.es.Index().Index(songSearchIndex).
		Type(songSearchType).
		OpType("delete").
		Id(song.TrackID.String())

	put, err := action.Do(ctx)
	if err != nil {
		return err
	}

	log.Printf("search: elastic: deleted song %s", put.Id)
	return nil
}
