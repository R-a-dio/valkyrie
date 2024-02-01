package mariadb

import (
	"context"

	radio "github.com/R-a-dio/valkyrie"
	"github.com/R-a-dio/valkyrie/config"
	"github.com/R-a-dio/valkyrie/errors"
	"github.com/R-a-dio/valkyrie/search"
	"github.com/R-a-dio/valkyrie/storage"
)

func init() {
	search.Register("storage", false, NewStorageSearchService)
}

type StorageSearchService interface {
	Search() radio.SearchService
}

func NewStorageSearchService(ctx context.Context, cfg config.Config) (radio.SearchService, error) {
	const op errors.Op = "search/storage.NewStorageSearchService"

	s, err := storage.Open(ctx, cfg)
	if err != nil {
		return nil, errors.E(op, err)
	}

	if ss, ok := s.(StorageSearchService); ok {
		return ss.Search(), nil
	}

	return nil, errors.E(op, errors.NotImplemented)
}
