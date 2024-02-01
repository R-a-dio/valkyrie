package streamer

import (
	"context"
	"fmt"
	"math/rand"
	"path/filepath"
	"sync"
	"time"

	radio "github.com/R-a-dio/valkyrie"
	"github.com/R-a-dio/valkyrie/config"
	"github.com/R-a-dio/valkyrie/errors"
	"github.com/R-a-dio/valkyrie/streamer/audio"
	"github.com/rs/zerolog"
)

// queueMinimumLength is the minimum amount of songs required to be
// in the queue. If less than queueMinimumLength is in the queue after a call
// to Queue.Populate an error is returned.
const queueMinimumLength = queueRequestThreshold / 2

// queueRequestThreshold is the amount of requests that should be in the queue
// before random songs should stop being added to it.
const queueRequestThreshold = 10

const queueName = "default"

// NewQueueService returns you a new QueueService with the configuration given
func NewQueueService(ctx context.Context, cfg config.Config, storage radio.StorageService) (*QueueService, error) {
	const op errors.Op = "streamer/NewQueueService"

	ctx, cancel := context.WithTimeout(ctx, time.Second*5)
	defer cancel()

	queue, err := storage.Queue(ctx).Load(queueName)
	if err != nil {
		return nil, errors.E(op, err)
	}

	qs := &QueueService{
		Config:  cfg,
		logger:  zerolog.Ctx(ctx),
		Storage: storage,
		queue:   queue,
		rand:    config.NewRand(true),
	}

	if err = qs.populate(ctx); err != nil {
		return nil, errors.E(op, err)
	}

	if err = storage.Queue(ctx).Store(queueName, qs.queue); err != nil {
		return nil, errors.E(op, err)
	}

	return qs, nil
}

// QueueService implements radio.QueueService that uses a random population algorithm
type QueueService struct {
	config.Config
	logger *zerolog.Logger

	Storage radio.StorageService
	rand    *rand.Rand

	// mu protects the fields below
	mu    sync.Mutex
	queue []radio.QueueEntry
	// reservedIndex indicates what part of the queue has been reserved by calls
	// to ReserveNext, it's the index of the first un-reserved entry
	reservedIndex int
}

// append appends the entry given to the queue, it tries to probe a more accurate
// song length with ffprobe and calculates the ExpectedStartTime on the entry
func (qs *QueueService) append(ctx context.Context, entry radio.QueueEntry) {
	// try running an ffprobe to get a more accurate song length
	ctx, cancel := context.WithTimeout(ctx, time.Second)
	defer cancel()

	path := filepath.Join(qs.Conf().MusicPath, entry.FilePath)
	length, err := audio.ProbeDuration(ctx, path)
	if err == nil {
		// only use the probe value if it didn't error
		entry.Length = length
	}

	if len(qs.queue) == 0 {
		entry.ExpectedStartTime = time.Now()
	} else {
		last := qs.queue[len(qs.queue)-1]
		entry.ExpectedStartTime = last.ExpectedStartTime.Add(last.Length)
	}

	qs.logger.Info().Str("entry", entry.String()).Msg("appending to queue")
	qs.queue = append(qs.queue, entry)
}

// calculateExpectedStartTime calculates the ExpectedStartTime fields of all entries
// based on the first entries ExpectedStartTime; This will generate incorrect times
// if the first entry has a wrong time.
func (qs *QueueService) calculateExpectedStartTime() {
	var length = qs.queue[0].Length
	for i := 1; i < len(qs.queue); i++ {
		qs.queue[i].ExpectedStartTime = qs.queue[i-1].ExpectedStartTime.Add(length)
	}
}

// AddRequest implements radio.QueueService
func (qs *QueueService) AddRequest(ctx context.Context, song radio.Song, identifier string) error {
	const op errors.Op = "streamer/QueueService.AddRequest"

	qs.mu.Lock()
	defer qs.mu.Unlock()

	qs.append(ctx, radio.QueueEntry{
		Song:           song,
		IsUserRequest:  true,
		UserIdentifier: identifier,
	})

	err := qs.Storage.Queue(ctx).Store(queueName, qs.queue)
	if err != nil {
		return errors.E(op, err)
	}
	return nil
}

// ReserveNext implements radio.QueueService
func (qs *QueueService) ReserveNext(ctx context.Context) (*radio.QueueEntry, error) {
	const op errors.Op = "streamer/QueueService.ReserveNext"

	qs.mu.Lock()
	defer qs.mu.Unlock()

	if len(qs.queue) == 0 {
		return nil, errors.E(op, errors.QueueEmpty)
	}

	if qs.reservedIndex == len(qs.queue) {
		return nil, errors.E(op, errors.QueueExhausted)
	}

	entry := qs.queue[qs.reservedIndex]
	qs.reservedIndex++
	qs.logger.Info().Str("entry", entry.String()).Msg("reserve in queue")

	return &entry, nil
}

// ResetReserved implements radio.QueueService
func (qs *QueueService) ResetReserved(ctx context.Context) error {
	qs.mu.Lock()
	defer qs.mu.Unlock()

	qs.logger.Info().Int("index", qs.reservedIndex).Msg("reset reserve in queue")
	qs.reservedIndex = 0
	return nil
}

// Remove removes the song given from the queue
func (qs *QueueService) Remove(ctx context.Context, entry radio.QueueEntry) (bool, error) {
	const op errors.Op = "streamer/QueueService.Remove"

	qs.mu.Lock()
	defer qs.mu.Unlock()

	size := len(qs.queue)
	for i, e := range qs.queue {
		if !e.EqualTo(entry) {
			continue
		}

		qs.logger.Info().Str("entry", e.String()).Msg("removing from queue")

		qs.queue = append(qs.queue[:i], qs.queue[i+1:]...)
		if i < qs.reservedIndex {
			qs.reservedIndex--
		}

		// we've removed the first song so assume it just started playing; now we update
		// the front of our queue with the current time and the songs length and
		// recalculate the rest from there
		if len(qs.queue) > 0 && i == 0 {
			qs.queue[0].ExpectedStartTime = time.Now().Add(e.Length)
			qs.calculateExpectedStartTime()
		}
		break
	}

	go func() {
		qs.mu.Lock()
		defer qs.mu.Unlock()

		ctx, cancel := context.WithTimeout(context.Background(), time.Second*5)
		defer cancel()

		err := qs.populate(ctx)
		if err != nil {
			qs.logger.Error().Err(err).Msg("failed to populate queue")
		}

		err = qs.Storage.Queue(ctx).Store(queueName, qs.queue)
		if err != nil {
			qs.logger.Error().Err(err).Msg("failed to store queue")
		}
	}()

	err := qs.Storage.Queue(ctx).Store(queueName, qs.queue)
	if err != nil {
		return false, errors.E(op, err)
	}

	return size != len(qs.queue), nil
}

// Entries returns all entries in the queue
func (qs *QueueService) Entries(ctx context.Context) ([]radio.QueueEntry, error) {
	qs.mu.Lock()
	defer qs.mu.Unlock()

	all := make([]radio.QueueEntry, len(qs.queue))
	copy(all, qs.queue)
	return all, nil
}

func (qs *QueueService) populate(ctx context.Context) error {
	const op errors.Op = "streamer/QueueService.populate"

	ts, tx, err := qs.Storage.TrackTx(ctx, nil)
	if err != nil {
		return errors.E(op, err)
	}
	defer tx.Rollback()

	// figure out what the queue consists of right now
	var randomEntries, requestEntries int
	for i := range qs.queue {
		if qs.queue[i].IsUserRequest {
			requestEntries++
		} else {
			randomEntries++
		}
	}

	if requestEntries > queueRequestThreshold {
		requestEntries = queueRequestThreshold
	}

	// total amount of random songs we want in the queue
	randomThreshold := (queueRequestThreshold - requestEntries) / 2
	if randomEntries >= randomThreshold {
		// we already have enough random songs
		return nil
	}
	// wanted final length of the queue
	wantedLength := len(qs.queue) + (randomThreshold - randomEntries)

	candidates, err := ts.QueueCandidates()
	if err != nil {
		return errors.E(op, err)
	}

	if len(candidates) == 0 {
		return errors.E(op, errors.QueueShort)
	}

	// bookmarking so we can tell what happens here
	var candidateCount = len(candidates)
	var skipReasons []error

outer:
	for len(qs.queue) < wantedLength {
		// we've run out of candidates
		if len(candidates) == 0 {
			break
		}

		// grab a candidate at random
		n := qs.rand.Intn(len(candidates))
		id := candidates[n]

		candidates[n] = candidates[len(candidates)-1]
		candidates = candidates[:len(candidates)-1]

		// check if our candidate might already be in the queue
		for i := range qs.queue {
			// and skip it if it is already there
			if qs.queue[i].TrackID == id {
				skipReasons = append(skipReasons, skipped{
					TrackID: id,
					Reason:  "duplicate entry",
				})
				continue outer
			}
		}

		song, err := ts.Get(id)
		if err != nil {
			skipReasons = append(skipReasons, skipped{
				TrackID: id,
				Err:     err,
			})
			continue
		}

		if err = ts.UpdateLastRequested(id); err != nil {
			skipReasons = append(skipReasons, skipped{
				TrackID: id,
				Err:     err,
			})
			continue
		}

		qs.append(ctx, radio.QueueEntry{
			Song: *song,
		})
	}

	// we always want to commit because we might have added songs to the queue but it
	// ended up still not being enough. So we do need to commit the track updates
	err = tx.Commit()
	if err != nil {
		return errors.E(op, err)
	}

	// we all okay so we can return, otherwise we want to log the reason we failed here
	if len(qs.queue) >= queueMinimumLength {
		return nil
	}

	if candidateCount == 0 {
		qs.logger.Info().Str("reason", "empty candidate list").Msg("failed to populate queue above minimum")
	}
	if len(skipReasons) > 0 {
		qs.logger.Info().
			Str("reason", "skipped all candidates").
			Errs("candidates", skipReasons).
			Msg("failed to populate queue above minimum")
	}

	return errors.E(op, errors.QueueShort)
}

type skipped struct {
	TrackID radio.TrackID
	Reason  string
	Err     error
}

func (s skipped) Error() string {
	if s.Err == nil {
		return fmt.Sprintf("<%d> %s", s.TrackID, s.Reason)
	}
	return fmt.Sprintf("<%d> %s", s.TrackID, s.Err)
}
