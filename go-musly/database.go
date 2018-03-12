package musly

import (
	"encoding/binary"
	"errors"
	"sync/atomic"

	"github.com/boltdb/bolt"
)

const binNumTracks = 1000

var (
	// DatabaseVersion indicates the internal version of the boltdb
	DatabaseVersion = []byte{0, 0, 1}

	// static bucket names and keys
	metadataBucket = []byte("metadata")
	// stores jukebox.method used
	metadataMethod = []byte("method")
	// stores jukebox.decoder used
	metadataDecoder = []byte("decoder")
	// stores DatabaseVersion
	metadataVersion = []byte("database_version")
	// stores amount of tracks per entry in jukeboxBucket
	metadataBinNumTracks = []byte("bin_num_tracks")

	jukeboxBucket      = []byte("jukebox")
	jukeboxTrackBucket = []byte("tracks")
)

var ErrMissingTracks = errors.New("musly: missing track")
var putInt = binary.BigEndian.PutUint64

func (b *Box) storeJukebox() error {
	// jukebox is stored in segments as to not require the full thing to be in
	// memory twice while loading it. Bucket layout uses sequential keys
	// starting from 0;
	//
	// 0: jukebox header
	// 1: tracks segment
	// 2: tracks segment
	// 3: ...
	var intBuf = make([]byte, 8)

	err := b.DB.Update(func(tx *bolt.Tx) error {
		tx.DeleteBucket(jukeboxBucket)
		bkt, err := tx.CreateBucket(jukeboxBucket)
		if err != nil {
			return err
		}

		// write out the jukebox header first
		var buf = make([]byte, b.jukebox.binSize(1, 0))
		n, err := b.jukebox.toBin(buf, 1, 0, 0)
		if err != nil {
			return err
		}

		// write header
		err = bkt.Put(intBuf, buf[:n])
		if err != nil {
			return err
		}

		// store the size of each segment
		bkt = tx.Bucket(metadataBucket)
		putInt(intBuf, binNumTracks)
		return bkt.Put(metadataBinNumTracks, intBuf)
	})
	if err != nil {
		return err
	}

	var max = b.jukebox.trackCount()
	var pos = 0
	var buf []byte

	fn := func(tx *bolt.Tx) error {
		putInt(intBuf, uint64(pos)/binNumTracks+1)
		bkt := tx.Bucket(jukeboxBucket)

		size := b.jukebox.binSize(0, binNumTracks)
		if len(buf) < size {
			buf = make([]byte, size)
		}

		n, err := b.jukebox.toBin(buf, 0, binNumTracks, pos)
		if err != nil {
			return err
		}

		// write track segment
		return bkt.Put(intBuf, buf[:n])
	}

	for pos = 0; pos < max; pos += binNumTracks {
		err = b.DB.Update(fn)
		if err != nil {
			return err
		}
	}

	return nil
}

func (b *Box) loadJukebox() error {
	return b.DB.View(func(tx *bolt.Tx) error {
		bkt := tx.Bucket(jukeboxBucket)
		if bkt == nil {
			// nothing to load
			return errors.New("musly: no jukebox found")
		}

		intBuf := make([]byte, 8)
		header := bkt.Get(intBuf)
		if header == nil {
			return errors.New("musly: no header found")
		}

		expected, err := b.jukebox.fromBin(header, 1, 0)
		if err != nil {
			return err
		}

		for pos := 0; pos < expected; pos += int(b.binNumTracks) {
			putInt(intBuf, uint64(pos)/b.binNumTracks+1)
			segment := bkt.Get(intBuf)
			if segment == nil {
				return errors.New("musly: missing track segment")
			}

			var amount = int(b.binNumTracks)
			if pos+amount > expected {
				amount = expected - pos
			}

			_, err = b.jukebox.fromBin(segment, 0, amount)
			if err != nil {
				return err
			}
		}

		return nil
	})
}

// storeTrack stores a track for later use with the id given
func (b *Box) storeTrack(t track, id TrackID) error {
	return b.DB.Batch(func(tx *bolt.Tx) error {
		bkt, err := tx.CreateBucketIfNotExists(jukeboxTrackBucket)
		if err != nil {
			return err
		}

		key := make([]byte, 8)
		putInt(key, uint64(id))
		return bkt.Put(key, b.trackToBytes(t))
	})
}

// loadTrack returns a Track previously stored by a call to storeTrack
func (b *Box) loadTrack(id TrackID) (track, error) {
	t := b.newTrackBytes()

	err := b.DB.View(func(tx *bolt.Tx) error {
		bkt := tx.Bucket(jukeboxTrackBucket)
		if bkt == nil {
			return nil
		}

		key := make([]byte, 8)
		putInt(key, uint64(id))
		tt := bkt.Get(key)
		if tt == nil {
			return errors.New("unknown track")
		}

		copy(t, tt)
		return nil
	})

	return b.bytesToTrack(t), err
}

// loadTracks returns multiple tracks previously stored by calls to storeTrack
func (b *Box) loadTracks(ids []TrackID) ([]track, error) {
	var tracks = make([]track, len(ids))

	err := b.DB.View(func(tx *bolt.Tx) error {
		bkt := tx.Bucket(jukeboxTrackBucket)
		if bkt == nil {
			return nil
		}

		key := make([]byte, 8)
		var err error
		for i, id := range ids {
			t := b.newTrack()
			putInt(key, uint64(id))
			v := bkt.Get(key)
			if v == nil {
				err = ErrMissingTracks
				b.freeTrack(t)
				continue
			}

			copy(b.trackToBytes(t), v)
			tracks[i] = t
		}

		return err
	})

	return tracks, err
}

// TrackCount returns the amount of tracks in the box
func (b *Box) TrackCount() (n int, err error) {
	return n, b.DB.View(func(tx *bolt.Tx) error {
		bkt := tx.Bucket(jukeboxTrackBucket)
		if bkt == nil {
			return nil
		}

		n = bkt.Stats().KeyN
		return nil
	})
}

// AllTrackIDs returns all TrackIDs stored in this box
func (b *Box) AllTrackIDs() ([]TrackID, error) {
	var ids = make([]TrackID, 0, 128)

	err := b.DB.View(func(tx *bolt.Tx) error {
		bkt := tx.Bucket(jukeboxTrackBucket)
		if bkt == nil {
			return nil
		}

		c := bkt.Cursor()
		for k, _ := c.First(); k != nil; k, _ = c.Next() {
			ids = append(ids, TrackID(binary.BigEndian.Uint64(k)))
		}

		return nil
	})

	return ids, err
}

// RemoveTrack removes a single track, see RemoveTracks
func (b *Box) RemoveTrack(id TrackID) error {
	return b.RemoveTracks([]TrackID{id})
}

// RemoveTracks removes all IDs given from both the jukebox aswell as the
// internal track database. This means to add a track again it has to be
// reanalyzed with AnalyzeFile or AnalyzePCM
func (b *Box) RemoveTracks(ids []TrackID) error {
	atomic.StoreInt32(&b.modified, 1)
	return b.DB.Update(func(tx *bolt.Tx) error {
		bkt := tx.Bucket(jukeboxTrackBucket)
		if bkt == nil {
			return nil
		}

		key := make([]byte, 8)
		for _, id := range ids {
			putInt(key, uint64(id))
			bkt.Delete(key)
		}

		return b.jukebox.removeTracks(ids)
	})
}
