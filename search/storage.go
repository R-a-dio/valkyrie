package search

import (
	"context"
	"time"

	radio "github.com/R-a-dio/valkyrie"
	"github.com/R-a-dio/valkyrie/errors"
)

// WrapStorageService wraps around a StorageService and intercepts calls to methods
// that mutate tracks in the TrackStorage interface. When such a mutation happens
// a background task is launched to update the SearchService given
func WrapStorageService(search radio.SearchService, storage radio.StorageService) radio.StorageService {
	return storageService{search, storage, storage}
}

// partialStorage is an interface containing all the methods we are NOT interested in.
type partialStorage interface {
	radio.SessionStorageService
	radio.RelayStorageService
	radio.QueueStorageService
	radio.SongStorageService
	radio.RequestStorageService
	radio.UserStorageService
	radio.StatusStorageService
	radio.SubmissionStorageService
	radio.NewsStorageService
	radio.ScheduleStorageService
}

type storageService struct {
	// search service to update
	search radio.SearchService
	// only embed the partial interface, if we embed the full StorageService interface
	// the compiler can't warn us about new methods that get added to it, this way there
	// is human interaction required with this code if a new method is introduced
	partialStorage
	// wrapped is the full TrackStorageService interface such that we can access the
	// methods we are wrapping in this implementation
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

// partialTrackStorage is an interface containing all the methods of radio.TrackStorage
// that we are NOT interested in.
type partialTrackStorage interface {
	Get(radio.TrackID) (*radio.Song, error)
	All() ([]radio.Song, error)
	Unusable() ([]radio.Song, error)
	BeforeLastRequested(before time.Time) ([]radio.Song, error)
	QueueCandidates() ([]radio.TrackID, error)
}

type trackStorage struct {
	ctx context.Context
	// search service to update
	search radio.SearchService
	// only embed the partial interface, if we embed the full TrackStorage interface
	// the compiler can't warn us about new methods that get added to it, this way there
	// is human interaction required with this code if a new method is introduced
	partialTrackStorage
	// wrapped is the full TrackStorage interface such that we can access the methods
	// we are wrapping in this implementation
	wrapped radio.TrackStorage
}

func (ts trackStorage) Insert(song radio.Song) (radio.TrackID, error) {
	const op errors.Op = "search/trackStorage.Insert"

	new, err := ts.wrapped.Insert(song)
	if err != nil {
		return new, errors.E(op, err)
	}

	err = ts.search.Update(ts.ctx, song)
	if err != nil {
		return new, errors.E(op, err)
	}

	return new, nil
}

func (ts trackStorage) UpdateMetadata(song radio.Song) error {
	const op errors.Op = "search/trackStorage.UpdateMetadata"

	err := ts.wrapped.UpdateMetadata(song)
	if err != nil {
		return errors.E(op, err)
	}

	err = ts.search.Update(ts.ctx, song)
	if err != nil {
		return errors.E(op, err)
	}
	return nil
}

func (ts trackStorage) UpdateUsable(song radio.Song, state radio.TrackState) error {
	const op errors.Op = "search/trackStorage.UpdateUsable"

	err := ts.wrapped.UpdateUsable(song, state)
	if err != nil {
		return errors.E(op, err)
	}

	new, err := ts.wrapped.Get(song.TrackID)
	if err != nil {
		return errors.E(op, err)
	}

	err = ts.search.Update(ts.ctx, *new)
	if err != nil {
		return errors.E(op, err)
	}
	return nil
}

func (ts trackStorage) UpdateRequestInfo(id radio.TrackID) error {
	const op errors.Op = "search/trackStorage.UpdateRequestInfo"

	err := ts.wrapped.UpdateRequestInfo(id)
	if err != nil {
		return errors.E(op, err)
	}

	new, err := ts.wrapped.Get(id)
	if err != nil {
		return errors.E(op, err)
	}

	err = ts.search.Update(ts.ctx, *new)
	if err != nil {
		return errors.E(op, err)
	}
	return nil
}

func (ts trackStorage) UpdateLastPlayed(id radio.TrackID) error {
	const op errors.Op = "search/trackStorage.UpdateLastPlayed"

	err := ts.wrapped.UpdateLastPlayed(id)
	if err != nil {
		return errors.E(op, err)
	}

	new, err := ts.wrapped.Get(id)
	if err != nil {
		return errors.E(op, err)
	}

	err = ts.search.Update(ts.ctx, *new)
	if err != nil {
		return errors.E(op, err)
	}
	return nil
}

func (ts trackStorage) UpdateLastRequested(id radio.TrackID) error {
	const op errors.Op = "search/trackStorage.UpdateLastRequested"

	err := ts.wrapped.UpdateLastRequested(id)
	if err != nil {
		return errors.E(op, err)
	}

	new, err := ts.wrapped.Get(id)
	if err != nil {
		return errors.E(op, err)
	}

	err = ts.search.Update(ts.ctx, *new)
	if err != nil {
		return errors.E(op, err)
	}
	return nil
}

func (ts trackStorage) DecrementRequestCount(before time.Time) error {
	const op errors.Op = "search/trackStorage.DecrementRequestCount"

	songs, err := ts.wrapped.BeforeLastRequested(before)
	if err != nil {
		return errors.E(op, err)
	}

	err = ts.wrapped.DecrementRequestCount(before)
	if err != nil {
		return errors.E(op, err)
	}

	// TODO(wessie): make this refresh from the actual storage
	for i := range songs {
		if songs[i].RequestCount > 0 {
			songs[i].RequestCount--
		}
	}

	err = ts.search.Update(ts.ctx, songs...)
	if err != nil {
		return errors.E(op, err)
	}
	return nil
}

func (ts trackStorage) Delete(id radio.TrackID) error {
	const op errors.Op = "search/trackStorage.Delete"

	err := ts.wrapped.Delete(id)
	if err != nil {
		return errors.E(op, err)
	}

	err = ts.search.Delete(ts.ctx, id)
	if err != nil {
		return errors.E(op, err)
	}
	return nil
}
