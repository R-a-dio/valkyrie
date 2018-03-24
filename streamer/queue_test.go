package streamer

import (
	"runtime"
	"testing"
	"time"

	"github.com/Wessie/hanyuu/database"
	_ "github.com/go-sql-driver/mysql"
	"github.com/jmoiron/sqlx"
)

func openDatabase(t *testing.T) *sqlx.DB {
	db, err := sqlx.Open("mysql", "radio@/radio?parseTime=true")
	if err != nil {
		t.Fatal("failed to open database:", err)
	}
	return db
}

func TestQueue(t *testing.T) {
	t.Skip()
	//db := openDatabase(t)
	s, _ := NewState("db")
	var q Queue
	var err error

	q.State = s

	t.Run("SaveEmpty", func(t *testing.T) {
		err = q.Save()
		if err != nil {
			t.Fatal(err)
		}
	})

	t.Run("Populate", func(t *testing.T) {
		err = q.populate()
		if err != nil {
			t.Fatal(err)
		}

		if len(q.l) != queueMinimumLength {
			t.Fatalf("populate did not meet minimum length: %d", len(q.l))
		}

		if q.totalLength == 0 {
			//	t.Fatal("populate did not set totalLength field")
		}
	})

	t.Run("Save", func(t *testing.T) {
		err = q.Save()
		if err != nil {
			t.Fatal(err)
		}
	})

	// create a new queue to test our loading from database
	beforeQueue := q.l
	q = Queue{State: s}

	t.Run("Load", func(t *testing.T) {
		err = q.load()
		if err != nil {
			t.Fatal(err)
		}

		if len(q.l) != len(beforeQueue) {
			t.Fatalf("load did not load all songs: %d != %d",
				len(beforeQueue), len(q.l))
		}

		var mismatch bool
		for i := range beforeQueue {
			if q.l[i].Track.TrackID != beforeQueue[i].Track.TrackID {
				t.Errorf("queue mismatch: %d != %d",
					q.l[i].Track.TrackID, beforeQueue[i].Track.TrackID)
				mismatch = true
			}
		}

		if mismatch {
			t.Fatal("queue before and after load mismatch")
		}
	})

	t.Run("Peek", func(t *testing.T) {
		track := q.Peek()
		if track == database.NoTrack {
			t.Fatal("peek returned NoTrack")
		}

		if track != q.l[0].Track {
			t.Fatalf("peek returned wrong track: %v != %v",
				track, q.l[0].Track)
		}
	})

	t.Run("Pop", func(t *testing.T) {
		expected := q.l[0].Track

		popped := q.Pop()
		if popped == database.NoTrack {
			t.Fatal("pop returned NoTrack")
		}
		if popped != expected {
			t.Fatalf("pop returned wrong track: %v != %v", popped, expected)
		}

		runtime.Gosched()
		// lock around here to avoid trampling on the populate goroutine
		// we just spawned by using Pop
		q.mu.Lock()
		if len(q.l) != queueMinimumLength {
			t.Error("pop did not re-populate queue")
		}
		q.mu.Unlock()
	})

	t.Run("Add", func(t *testing.T) {
		track := database.Track{Acceptor: "test"}
		track.Length = time.Second * 300

		lengthBefore := q.totalLength
		q.addEntry(database.QueueEntry{
			Track: track,
		})

		last := q.l[len(q.l)-1]
		if last.Track.Acceptor != track.Acceptor {
			t.Errorf("add did not add track: %v != %v", last.Track, track)
		}

		if q.totalLength-lengthBefore != time.Second*300 {
			t.Errorf("add did not add length to total: %d != %d",
				q.totalLength, lengthBefore+time.Second*300)
		}
	})

}
