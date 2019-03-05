package search

import (
	"context"
	"time"

	radio "github.com/R-a-dio/valkyrie"
)

// WrapStorageService wraps around a StorageService and intercepts calls to methods
// that mutate tracks in the TrackStorage interface. When such a mutation happens
// a background task is launched to update the SearchService given
func WrapStorageService(search radio.SearchService, storage radio.StorageService) radio.StorageService {
	return storageService{search, storage, storage}
}

type partialStorage interface {
	radio.QueueStorageService
	radio.SongStorageService
	radio.RequestStorageService
	radio.UserStorageService
	radio.StatusStorageService
}

type storageService struct {
	search radio.SearchService
	partialStorage
	wrapped radio.TrackStorageService
}

func (ss storageService) Track(ctx context.Context) radio.TrackStorage {
	ts := ss.wrapped.Track(ctx)
	return trackStorage{ctx, ss.search, ts, ts}
}

func (ss storageService) TrackTx(ctx context.Context, tx radio.StorageTx) (radio.TrackStorage, radio.StorageTx, error) {
	ts, tx, err := ss.wrapped.TrackTx(ctx, tx)
	return trackStorage{ctx, ss.search, ts, ts}, tx, err
}

type partialTrackStorage interface {
	Get(radio.TrackID) (*radio.Song, error)
	All() ([]radio.Song, error)
	Unusable() ([]radio.Song, error)
	BeforeLastRequested(before time.Time) ([]radio.Song, error)
	QueueCandidates() ([]radio.TrackID, error)
}

type trackStorage struct {
	ctx    context.Context
	search radio.SearchService
	partialTrackStorage
	wrapped radio.TrackStorage
}

func (ts trackStorage) UpdateUsable(song radio.Song, state int) error {
	err := ts.wrapped.UpdateUsable(song, state)
	if err != nil {
		return err
	}

	new, err := ts.wrapped.Get(song.TrackID)
	if err != nil {
		return err
	}

	return ts.search.Update(ts.ctx, *new)
}

func (ts trackStorage) UpdateRequestInfo(id radio.TrackID) error {
	err := ts.wrapped.UpdateRequestInfo(id)
	if err != nil {
		return err
	}

	new, err := ts.wrapped.Get(id)
	if err != nil {
		return err
	}

	return ts.search.Update(ts.ctx, *new)
}

func (ts trackStorage) UpdateLastPlayed(id radio.TrackID) error {
	err := ts.wrapped.UpdateLastPlayed(id)
	if err != nil {
		return err
	}

	new, err := ts.wrapped.Get(id)
	if err != nil {
		return err
	}

	return ts.search.Update(ts.ctx, *new)
}

func (ts trackStorage) UpdateLastRequested(id radio.TrackID) error {
	err := ts.wrapped.UpdateLastRequested(id)
	if err != nil {
		return err
	}

	new, err := ts.wrapped.Get(id)
	if err != nil {
		return err
	}

	return ts.search.Update(ts.ctx, *new)
}

func (ts trackStorage) DecrementRequestCount(before time.Time) error {
	songs, err := ts.wrapped.BeforeLastRequested(before)
	if err != nil {
		return err
	}

	err = ts.wrapped.DecrementRequestCount(before)
	if err != nil {
		return err
	}

	// TODO(wessie): make this refresh from the actual storage
	for i := range songs {
		if songs[i].RequestCount > 0 {
			songs[i].RequestCount--
		}
	}

	return ts.search.Update(ts.ctx, songs...)
}
