package streamer

import (
	"context"
	"fmt"
	"math/rand/v2"
	"sync"
	"time"

	radio "github.com/R-a-dio/valkyrie"
	"github.com/R-a-dio/valkyrie/config"
	"github.com/R-a-dio/valkyrie/errors"
	"github.com/R-a-dio/valkyrie/streamer/audio"
	"github.com/rs/zerolog"
	"go.opentelemetry.io/otel"
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

	// quickly make sure all entries have an QueueID associated with them
	for i, e := range queue {
		if e.QueueID.IsZero() {
			queue[i].QueueID = radio.NewQueueID()
		}
	}

	qs := &QueueService{
		logger:  zerolog.Ctx(ctx),
		Storage: storage,
		prober:  audio.NewProber(cfg, time.Second*2), // wait 2 seconds at most for ffprobe to run
		queue:   queue,
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
	logger *zerolog.Logger

	Storage radio.StorageService
	prober  audio.Prober

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
	const op errors.Op = "streamer/QueueService.append"
	ctx, span := otel.Tracer("queue").Start(ctx, string(op))
	defer span.End()

	// try running an ffprobe to get a more accurate song length
	length, err := qs.prober(ctx, entry.Song)
	if err != nil {
		// log any error, but it isn't critical so just continue
		qs.logger.Error().Ctx(ctx).Err(err).Msg("duration probe failure")
	}

	if length > 0 { // only change the length if we actually got one
		entry.Length = length
	}

	if len(qs.queue) == 0 {
		entry.ExpectedStartTime = time.Now()
	} else {
		last := qs.queue[len(qs.queue)-1]
		entry.ExpectedStartTime = last.ExpectedStartTime.Add(last.Length)
	}
	entry.QueueID = radio.NewQueueID()

	qs.logger.Info().Ctx(ctx).Str("entry", entry.String()).Msg("appending to queue")
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
	ctx, span := otel.Tracer("queue").Start(ctx, string(op))
	defer span.End()

	qs.mu.Lock()
	defer qs.mu.Unlock()

	// there is a small edge-case where we might've populated the queue with a new song that also just
	// got requested (see https://github.com/R-a-dio/valkyrie/issues/269) so to avoid that we check if
	// a song is already in the queue here, and if it is we just update that entry instead of adding
	// a new one.
	addNew := true
	for i := range qs.queue {
		if qs.queue[i].TrackID != song.TrackID {
			continue
		}

		addNew = false
		qs.queue[i].IsUserRequest = true
		qs.queue[i].UserIdentifier = identifier
	}

	if addNew {
		qs.append(ctx, radio.QueueEntry{
			Song:           song.Copy(),
			IsUserRequest:  true,
			UserIdentifier: identifier,
		})
	}

	err := qs.Storage.Queue(ctx).Store(queueName, qs.queue)
	if err != nil {
		return errors.E(op, err)
	}
	return nil
}

// ReserveNext implements radio.QueueService
func (qs *QueueService) ReserveNext(ctx context.Context) (*radio.QueueEntry, error) {
	const op errors.Op = "streamer/QueueService.ReserveNext"
	ctx, span := otel.Tracer("queue").Start(ctx, string(op))
	defer span.End()

	qs.mu.Lock()
	defer qs.mu.Unlock()

	if len(qs.queue) == 0 {
		return nil, errors.E(op, errors.QueueEmpty)
	}

	if qs.reservedIndex == len(qs.queue) {
		return nil, errors.E(op, errors.QueueExhausted)
	}

	entry := qs.queue[qs.reservedIndex].Copy()
	qs.reservedIndex++
	qs.logger.Info().Ctx(ctx).Str("entry", entry.String()).Msg("reserve in queue")

	return &entry, nil
}

// ResetReserved implements radio.QueueService
func (qs *QueueService) ResetReserved(ctx context.Context) error {
	const op errors.Op = "streamer/QueueService.ResetReserved"
	ctx, span := otel.Tracer("queue").Start(ctx, string(op))
	defer span.End()

	qs.mu.Lock()
	defer qs.mu.Unlock()

	qs.logger.Info().Ctx(ctx).Int("index", qs.reservedIndex).Msg("reset reserve in queue")
	qs.reservedIndex = 0
	return nil
}

// Remove removes the song given from the queue
func (qs *QueueService) Remove(ctx context.Context, id radio.QueueID) (bool, error) {
	const op errors.Op = "streamer/QueueService.Remove"
	ctx, span := otel.Tracer("queue").Start(ctx, string(op))
	defer span.End()

	qs.mu.Lock()
	defer qs.mu.Unlock()

	size := len(qs.queue)
	for i, e := range qs.queue {
		if e.QueueID != id {
			continue
		}

		qs.logger.Info().Ctx(ctx).Str("entry", e.String()).Msg("removing from queue")

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
			qs.logger.Error().Ctx(ctx).Err(err).Msg("failed to populate queue")
		}

		err = qs.Storage.Queue(ctx).Store(queueName, qs.queue)
		if err != nil {
			qs.logger.Error().Ctx(ctx).Err(err).Msg("failed to store queue")
		}
	}()

	err := qs.Storage.Queue(ctx).Store(queueName, qs.queue)
	if err != nil {
		return false, errors.E(op, err)
	}

	return size != len(qs.queue), nil
}

// Entries returns all entries in the queue
func (qs *QueueService) Entries(ctx context.Context) (radio.Queue, error) {
	const op errors.Op = "streamer/QueueService.Entries"
	ctx, span := otel.Tracer("queue").Start(ctx, string(op))
	defer span.End()

	qs.mu.Lock()
	defer qs.mu.Unlock()

	all := make([]radio.QueueEntry, 0, len(qs.queue))
	for _, e := range qs.queue {
		all = append(all, e.Copy())
	}
	return all, nil
}

func (qs *QueueService) populate(ctx context.Context) error {
	const op errors.Op = "streamer/QueueService.populate"
	ctx, span := otel.Tracer("queue").Start(ctx, string(op))
	defer span.End()

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
		n := rand.IntN(len(candidates))
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
		qs.logger.Info().Ctx(ctx).Str("reason", "empty candidate list").Msg("failed to populate queue above minimum")
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
