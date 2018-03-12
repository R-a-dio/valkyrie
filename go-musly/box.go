package musly

/*
#include <musly/musly_types.h>
#include <musly/musly.h>
*/
import "C"

import (
	"encoding/binary"
	"errors"
	"sync/atomic"

	"github.com/boltdb/bolt"
)

type Box struct {
	// allocs counts the amount of tracks we've allocated and not freed yet
	allocs int64
	// modified indicates if our musly_jukebox has been modified
	modified int32

	jukebox     *jukebox
	MethodName  string
	DecoderName string
	Path        string

	binNumTracks uint64
	DB           *bolt.DB
}

// OpenBox opens or creates a jukebox with default options
func OpenBox(path string) (*Box, error) {
	return OpenBoxOpt(path, "", "")
}

// OpenBoxOpt opens a jukebox or creates one with the options given
func OpenBoxOpt(path string, method string, decoder string) (*Box, error) {
	db, err := bolt.Open(path, 0666, nil)
	if err != nil {
		return nil, err
	}

	// check if we are opening an existing or new jukebox
	var new bool
	err = db.View(func(tx *bolt.Tx) error {
		new = tx.Bucket(metadataBucket) == nil
		return nil
	})
	if err != nil {
		return nil, err
	}

	if new {
		return newBox(db, path, method, decoder)
	}

	return openBox(db, path)
}

func openBox(db *bolt.DB, path string) (*Box, error) {
	var method string
	var decoder string
	var numTracks uint64
	err := db.View(func(tx *bolt.Tx) error {
		b := tx.Bucket(metadataBucket)
		if b == nil {
			return errors.New("musly: invalid database file")
		}

		method = string(b.Get(metadataMethod))
		if method == "" {
			return errors.New("musly: empty method")
		}
		decoder = string(b.Get(metadataDecoder))
		if decoder == "" {
			return errors.New("musly: empty decoder")
		}

		num := b.Get(metadataBinNumTracks)
		if len(num) != 8 {
			numTracks = binNumTracks
		} else {
			numTracks = binary.BigEndian.Uint64(num)
		}
		return nil
	})
	if err != nil {
		db.Close()
		return nil, err
	}

	jukebox := newJukebox(method, decoder)
	if jukebox == nil {
		db.Close()
		return nil, errors.New("musly: power on failed")
	}

	box := &Box{
		jukebox:      jukebox,
		DecoderName:  decoder,
		MethodName:   method,
		binNumTracks: numTracks,
		Path:         path,
		DB:           db,
	}

	return box, box.loadJukebox()
}

// newBox returns a new musly jukebox with the given method and decoder
func newBox(db *bolt.DB, path string, method string, decoder string) (*Box, error) {
	jukebox := newJukebox(method, decoder)
	if jukebox == nil {
		db.Close()
		return nil, errors.New("musly: power on failed")
	}

	box := &Box{
		modified:    1,
		jukebox:     jukebox,
		DecoderName: C.GoString(jukebox.decoder_name),
		MethodName:  C.GoString(jukebox.method_name),
		Path:        path,
		DB:          db,
	}

	err := db.Update(func(tx *bolt.Tx) error {
		b, err := tx.CreateBucketIfNotExists(metadataBucket)
		if err != nil {
			return err
		}

		err = b.Put(metadataVersion, DatabaseVersion)
		if err != nil {
			return err
		}
		err = b.Put(metadataDecoder, []byte(box.DecoderName))
		if err != nil {
			return err
		}
		return b.Put(metadataMethod, []byte(box.MethodName))
	})
	if err != nil {
		box.Close()
		return nil, err
	}

	return box, nil
}

func (b *Box) RemoveTrack(id TrackID) error {
	return b.RemoveTracks([]TrackID{id})
}

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
