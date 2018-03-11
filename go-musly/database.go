package musly

import (
	"encoding/binary"
	"errors"

	"github.com/boltdb/bolt"
)

const binNumTracks = 1000

var (
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
		var buf = make([]byte, b.jukeboxBinSize(1, 0))
		n, err := b.jukeboxToBin(buf, 1, 0, 0)
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

	var max = b.TrackCount()
	var pos = 0
	var buf []byte

	fn := func(tx *bolt.Tx) error {
		putInt(intBuf, uint64(pos)/binNumTracks+1)
		bkt := tx.Bucket(jukeboxBucket)

		size := b.jukeboxBinSize(0, binNumTracks)
		if len(buf) < size {
			buf = make([]byte, size)
		}

		n, err := b.jukeboxToBin(buf, 0, binNumTracks, pos)
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

		expected, err := b.jukeboxFromBin(header, 1, 0)
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

			_, err = b.jukeboxFromBin(segment, 0, amount)
			if err != nil {
				return err
			}
		}

		return nil
	})
}

// StoreTrack stores a track for later use with the id given
func (b *Box) StoreTrack(t Track, id TrackID) error {
	return b.DB.Batch(func(tx *bolt.Tx) error {
		bkt, err := tx.CreateBucketIfNotExists(jukeboxTrackBucket)
		if err != nil {
			return err
		}

		return bkt.Put(keyTrackID(nil, id), b.trackToBytes(t))
	})
}

func keyTrackID(buf []byte, id TrackID) []byte {
	if buf == nil {
		buf = make([]byte, 8)
	}
	binary.BigEndian.PutUint64(buf, uint64(id))
	return buf
}

// Track returns a Track previously stored by a call to StoreTrack
func (b *Box) Track(id TrackID) (Track, error) {
	track := b.NewTrackBytes()
	err := b.DB.View(func(tx *bolt.Tx) error {
		bkt := tx.Bucket(jukeboxTrackBucket)
		if bkt == nil {
			return nil
		}

		t := bkt.Get(keyTrackID(nil, id))
		if t == nil {
			return errors.New("unknown track")
		}

		copy(track, t)
		return nil
	})

	return b.bytesToTrack(track), err
}

// Tracks returns multiple tracks previously stored by calls to StoreTrack
func (b *Box) Tracks(ids []TrackID) ([]Track, error) {
	var tracks = make([]Track, len(ids))

	err := b.DB.View(func(tx *bolt.Tx) error {
		bkt := tx.Bucket(jukeboxTrackBucket)
		if bkt == nil {
			return nil
		}

		key := make([]byte, 8)
		var err error
		for i, id := range ids {
			track := b.NewTrack()
			v := bkt.Get(keyTrackID(key, id))
			if v == nil {
				err = ErrMissingTracks
				b.FreeTrack(track)
				continue
			}

			copy(b.trackToBytes(track), v)
			tracks[i] = track
		}

		return err
	})

	return tracks, err
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
