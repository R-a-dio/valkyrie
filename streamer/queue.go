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

	"github.com/R-a-dio/valkyrie/config"
	"github.com/R-a-dio/valkyrie/database"
	"github.com/R-a-dio/valkyrie/streamer/audio"
)

// queueMinimumLength is the minimum amount of songs required to be
// in the queue. If less than queueMinimumLength is in the queue after a call
// to Queue.Populate an error is returned.
const queueMinimumLength = queueRequestThreshold / 2

// queueRequestThreshold is the amount of requests that should be in the queue
// before random songs should stop being added to it.
const queueRequestThreshold = 10

// ErrEmptyQueue is returned when the queue is empty
var ErrEmptyQueue = errors.New("empty queue")

// NewQueue returns a ready to use Queue instance, restoring
// state from the database before returning.
func NewQueue(s *config.State) (*Queue, error) {
	var q = &Queue{
		State:            s,
		nextSongEstimate: time.Now(),
	}

	log.Println("queue: loading")
	if err := q.load(); err != nil {
		log.Printf("queue: loading error: %s\n", err)
		return nil, err
	}

	log.Println("queue: populating")
	if err := q.populate(); err != nil {
		log.Printf("queue: populate error: %s\n", err)
		return nil, err
	}

	log.Println("queue: finished initializing")
	return q, nil
}

// Queue is the queue of tracks for the streamer
type Queue struct {
	*config.State

	mu sync.Mutex
	// l is the in-memory representation of the queue
	l []database.QueueEntry
	// nextSongEstimate is the estimated start-time of the next song
	nextSongEstimate time.Time
	// totalLength is the length of all songs in the queue summed
	totalLength time.Duration
}

// AddRequest adds a track to the queue as requested by uid
func (q *Queue) AddRequest(t database.Track, uid string) {
	q.mu.Lock()
	q.addEntry(database.QueueEntry{
		Track:          t,
		IsRequest:      true,
		UserIdentifier: uid,
	})
	q.mu.Unlock()
	go q.Save()
}

// Add adds a track to the queue
func (q *Queue) Add(t database.Track) {
	q.mu.Lock()
	q.addEntry(database.QueueEntry{
		Track:          t,
		UserIdentifier: "internal",
	})
	q.mu.Unlock()
	go q.Save()
}

// addEntry adds the QueueEntry to the queue and populates its
// EstimatedPlayTime field.
//
// Before calling addEntry you should lock q.mu
func (q *Queue) addEntry(e database.QueueEntry) {
	// our database length is inaccurate due to human streamers adjusting
	// them when a song plays, so instead try to find the duration ourselves
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	path := filepath.Join(q.Conf().MusicPath, e.Track.FilePath)
	length, err := audio.ProbeDuration(ctx, path)
	cancel()
	if err == nil {
		// only use the result if there was no error
		e.Track.Length = length
	} else {
		fmt.Println("queue: probe error:", err)
	}

	// TODO: make this use relative times from Now
	e.EstimatedPlayTime = q.nextSongEstimate.Add(q.totalLength)
	q.totalLength += e.Track.Length

	q.l = append(q.l, e)
}

// Save saves the queue to the database
func (q *Queue) Save() error {
	q.mu.Lock()
	// recalculate playtime estimates, because we could have been sitting idle
	// and that would mean the queue drifts away
	//
	// TODO: make this use relative times from Now
	var length time.Duration
	for i := range q.l {
		q.l[i].EstimatedPlayTime = q.nextSongEstimate.Add(length)
		length += q.l[i].Track.Length
	}
	q.totalLength = length

	h := database.Handle(context.TODO(), q.DB)
	err := database.QueueSave(h, q.l)
	q.mu.Unlock()
	return err
}

// Entries returns a copy of all queue entries
func (q *Queue) Entries() []database.QueueEntry {
	q.mu.Lock()
	all := make([]database.QueueEntry, len(q.l))

	for i := range q.l {
		all[i] = q.l[i]
	}
	q.mu.Unlock()

	return all
}

func (q *Queue) peek() database.Track {
	if len(q.l) == 0 {
		return database.NoTrack
	}

	// refresh our in-memory track with database info, something might've
	// changed between the time we got queued, and the time we're being used.
	return q.refreshTrack(q.l[0].Track)
}

// Peek returns the next track to be returned from Pop
func (q *Queue) Peek() database.Track {
	q.mu.Lock()
	defer q.mu.Unlock()

	return q.peek()
}

// PeekTrack returns the track positioned after the track given or next track
// if track is not in queue
func (q *Queue) PeekTrack(t database.Track) database.Track {
	q.mu.Lock()
	defer q.mu.Unlock()

	var found bool
	for _, qt := range q.l {
		// we're returning the track that comes after track `t`
		if found {
			return q.refreshTrack(qt.Track)
		}

		if qt.Track.EqualTo(t) {
			found = true
		}
	}

	return q.peek()
}

func (q *Queue) refreshTrack(t database.Track) database.Track {
	h := database.Handle(context.TODO(), q.DB)
	nt, err := database.GetTrack(h, t.TrackID)
	if err != nil {
		// we just return our original in-memory version if the database query
		// failed to complete
		return t
	}

	// since we probe for a duration if length is zero, we have to copy it from
	// the track we already had
	if nt.Length == 0 {
		nt.Length = t.Length
	}

	return nt
}

// Pop removes and returns the next track in the queue
func (q *Queue) Pop() database.Track {
	q.mu.Lock()
	defer q.mu.Unlock()

	return q.pop()
}

// PopTrack pops the next track if it's the track given; otherwise does
// nothing.
func (q *Queue) PopTrack(t database.Track) {
	q.mu.Lock()
	defer q.mu.Unlock()

	if len(q.l) == 0 {
		return
	}

	e := q.l[0]

	// check if our top-most track is equal to the argument
	if e.Track.EqualTo(t) {
		q.pop()
	}
}

// RemoveTrack removes the first occurence of the track given from the queue
func (q *Queue) RemoveTrack(t database.Track) {
	q.mu.Lock()
	for i, qt := range q.l {
		if !qt.Track.EqualTo(t) {
			continue
		}

		q.l = append(q.l[:i], q.l[i+1:]...)
		break
	}
	q.mu.Unlock()
}

// pop pops a track from the queue, before calling pop you have to hold q.mu
func (q *Queue) pop() database.Track {
	if len(q.l) == 0 {
		return database.NoTrack
	}

	e := q.l[0]
	q.l = q.l[:copy(q.l, q.l[1:])]

	q.nextSongEstimate = time.Now().Add(e.Track.Length)
	q.totalLength -= e.Track.Length

	go func() {
		// TODO: make all calls use the same transaction
		h := database.Handle(context.TODO(), q.DB)
		database.UpdateTrackPlayTime(h, e.Track.TrackID)
		q.populate()
		q.Save()
	}()
	return e.Track
}

func (q *Queue) load() error {
	h := database.Handle(context.TODO(), q.DB)
	queue, err := database.QueueLoad(h)
	if err != nil {
		return err
	}

	q.mu.Lock()
	for _, e := range queue {
		q.addEntry(e)
	}
	q.mu.Unlock()

	return nil
}

func (q *Queue) populate() error {
	q.mu.Lock()
	defer q.mu.Unlock()

	var randomEntries, requestEntries int
	for _, e := range q.l {
		if e.IsRequest {
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
		return nil
	}

	tx, err := database.HandleTx(context.TODO(), q.DB)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	candidates, err := database.QueuePopulate(tx)
	if err != nil {
		return err
	}

	fmt.Println(candidates)
	var addedCount int
	for addedCount < randomThreshold-randomEntries {
		if len(candidates) == 0 {
			break
		}

		n := rand.Intn(len(candidates))
		tid := candidates[n]
		fmt.Println(tid)

		candidates[n] = candidates[len(candidates)-1]
		candidates = candidates[:len(candidates)-1]

		var dup bool
		for _, e := range q.l {
			if e.Track.TrackID == tid {
				fmt.Println("queue: found duplicate:", tid)
				dup = true
				break
			}
		}
		if dup {
			fmt.Println("queue: continue")
			continue
		}

		t, err := database.GetTrack(tx, tid)
		if err != nil {
			fmt.Println("queue: populate: track error:", err)
			continue
		}

		if err = database.QueueUpdateTrack(tx, tid); err != nil {
			fmt.Println("queue: populate: update error:", err)
			continue
		}

		addedCount++
		q.addEntry(database.QueueEntry{
			Track:          t,
			UserIdentifier: "internal",
		})
	}

	if len(q.l) < queueMinimumLength {
		return errors.New("not enough songs in queue")
	}

	return tx.Commit()
}
