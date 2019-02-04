package streamer

import (
	"context"
	"errors"
	"fmt"
	"log"
	"math/rand"
	"path/filepath"
	"sync"
	"time"

	radio "github.com/R-a-dio/valkyrie"
	"github.com/R-a-dio/valkyrie/config"
	"github.com/R-a-dio/valkyrie/database"
	"github.com/R-a-dio/valkyrie/streamer/audio"
	"github.com/jmoiron/sqlx"
)

// queueMinimumLength is the minimum amount of songs required to be
// in the queue. If less than queueMinimumLength is in the queue after a call
// to Queue.Populate an error is returned.
const queueMinimumLength = queueRequestThreshold / 2

// queueRequestThreshold is the amount of requests that should be in the queue
// before random songs should stop being added to it.
const queueRequestThreshold = 10

const queueName = "default"

// ErrEmptyQueue is returned when the queue is empty
var ErrEmptyQueue = errors.New("queue: empty")

// ErrExhaustedQueue is returned when there aren't enough songs to return
var ErrExhaustedQueue = errors.New("queue: exhausted")

// ErrShortQueue is returned by population if it found less candidates than required
var ErrShortQueue = errors.New("queue: not enough population candidates")

// NewQueueService returns you a new QueueService with the configuration given
func NewQueueService(ctx context.Context, cfg config.Config, db *sqlx.DB) (*QueueService, error) {
	ctx, cancel := context.WithTimeout(ctx, time.Second*5)
	defer cancel()

	storage := database.NewQueueStorage(db)
	queue, err := storage.Load(ctx, queueName)
	if err != nil {
		return nil, err
	}

	qs := &QueueService{
		Config:  cfg,
		db:      db,
		queue:   queue,
		storage: storage,
	}

	if err = qs.populate(ctx); err != nil {
		return nil, err
	}

	return qs, storage.Store(ctx, queueName, qs.queue)
}

// QueueService implements radio.QueueService that uses a random population algorithm
type QueueService struct {
	config.Config
	db      *sqlx.DB
	storage radio.QueueStorage

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

	log.Printf("queue:   adding entry: %s", entry)
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
	qs.mu.Lock()
	defer qs.mu.Unlock()

	qs.append(ctx, radio.QueueEntry{
		Song:           song,
		IsUserRequest:  true,
		UserIdentifier: identifier,
	})

	return qs.storage.Store(ctx, queueName, qs.queue)
}

// ReserveNext implements radio.QueueService
func (qs *QueueService) ReserveNext(ctx context.Context) (*radio.QueueEntry, error) {
	qs.mu.Lock()
	defer qs.mu.Unlock()

	if len(qs.queue) == 0 {
		return nil, ErrEmptyQueue
	}

	if qs.reservedIndex == len(qs.queue) {
		return nil, ErrExhaustedQueue
	}

	entry := qs.queue[qs.reservedIndex]
	qs.reservedIndex++
	log.Printf("queue: reserved entry: %s", entry)

	return &entry, nil
}

// ResetReserved implements radio.QueueService
func (qs *QueueService) ResetReserved(ctx context.Context) error {
	qs.mu.Lock()
	defer qs.mu.Unlock()

	log.Printf("queue: resetting reserved index from %d", qs.reservedIndex)
	qs.reservedIndex = 0
	return nil
}

// Remove removes the song given from the queue
func (qs *QueueService) Remove(ctx context.Context, entry radio.QueueEntry) (bool, error) {
	qs.mu.Lock()
	defer qs.mu.Unlock()

	size := len(qs.queue)
	for i, e := range qs.queue {
		if !e.EqualTo(entry) {
			continue
		}

		log.Printf("queue: removing entry: %s", e)

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
			log.Println(err)
		}

		err = qs.storage.Store(ctx, queueName, qs.queue)
		if err != nil {
			log.Println(err)
		}
	}()

	err := qs.storage.Store(ctx, queueName, qs.queue)
	if err != nil {
		return false, err
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
	tx, err := database.HandleTx(ctx, qs.db)
	if err != nil {
		return err
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

	candidates, err := database.QueuePopulate(tx)
	if err != nil {
		return err
	}

	if len(candidates) == 0 {
		return ErrShortQueue
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
		n := rand.Intn(len(candidates))
		id := candidates[n]

		candidates[n] = candidates[len(candidates)-1]
		candidates = candidates[:len(candidates)-1]

		// check if our candidate might already be in the queue
		for i := range qs.queue {
			// and skip it if it is already there
			if qs.queue[i].TrackID == id {
				skipReasons = append(skipReasons, skipped{
					trackID: id,
					reason:  "duplicate entry",
				})
				continue outer
			}
		}

		song, err := database.GetTrack(tx, id)
		if err != nil {
			skipReasons = append(skipReasons, skipped{
				trackID: id,
				err:     err,
			})
			continue
		}

		if err = database.QueueUpdateTrack(tx, id); err != nil {
			skipReasons = append(skipReasons, skipped{
				trackID: id,
				err:     err,
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
		return err
	}

	// we all okay so we can return, otherwise we want to log the reason we failed here
	if len(qs.queue) >= queueMinimumLength {
		return nil
	}

	log.Printf("queue: failed to populate above minimum")
	if candidateCount == 0 {
		log.Printf("queue: empty candidate list")
	}
	if len(skipReasons) > 0 {
		log.Printf("queue: skipped song reasons:")
	}
	for i, err := range skipReasons {
		log.Printf("queue: %6d %s", i, err)
	}

	return ErrShortQueue
}

type skipped struct {
	trackID radio.TrackID
	reason  string
	err     error
}

func (s skipped) Error() string {
	if s.err == nil {
		return fmt.Sprintf("<%d> %s", s.trackID, s.reason)
	}
	return fmt.Sprintf("<%d> %v", s.trackID, s.err)
}
