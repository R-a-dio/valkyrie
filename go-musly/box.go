package musly

/*
#include <musly/musly_types.h>
#include <musly/musly.h>
*/
import "C"

import (
	"encoding/binary"
	"fmt"
	"sync"
	"sync/atomic"

	"github.com/boltdb/bolt"
)

// Box is an abstraction on-top of a musly_jukebox and an on-disk database of
// audio tracks.
type Box struct {
	// allocs counts the amount of tracks we've allocated and not freed yet
	allocs int64
	// modified indicates if our musly_jukebox has been modified
	modified atomicBool
	// set if setmusicstyle has been called on our jukebox
	musicStyleSet atomicBool

	// mutex to protect the musly_jukebox; which does not seem to do internal
	// locking of any kind. We do a best-effort job by assuming some operations
	// are read-only and should be safe for multiple readers
	jukeboxMu   sync.RWMutex
	jukebox     *jukebox
	MethodName  string
	DecoderName string
	Path        string

	trackSize    int
	binNumTracks uint64
	DB           *bolt.DB

	musicStyleOnce errorOnce
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
	var musicStyleSet bool

	err := db.View(func(tx *bolt.Tx) error {
		bkt := tx.Bucket(metadataBucket)
		if bkt == nil {
			return &Error{
				Err:  ErrInvalidDatabase,
				Info: "missing metadata",
			}
		}

		method = string(bkt.Get(metadataMethod))
		if method == "" {
			return ErrInvalidDatabase
		}
		decoder = string(bkt.Get(metadataDecoder))
		if decoder == "" {
			return ErrInvalidDatabase
		}

		num := bkt.Get(metadataBinNumTracks)
		if len(num) != 8 {
			numTracks = binNumTracks
		} else {
			numTracks = binary.BigEndian.Uint64(num)
		}

		musicStyleSet = string(bkt.Get(jukeboxMusicStyle)) == "true"
		return nil
	})
	if err != nil {
		db.Close()
		return nil, err
	}

	jukebox, err := newJukebox(method, decoder)
	if err != nil {
		db.Close()
		return nil, err
	}

	box := &Box{
		jukebox:      jukebox,
		trackSize:    jukebox.trackSize(),
		DecoderName:  decoder,
		MethodName:   method,
		binNumTracks: numTracks,
		Path:         path,
		DB:           db,
	}

	if musicStyleSet {
		// trigger our once, since we've already set our music style in a previous
		// use of the jukebox
		box.musicStyleSet.Set()
		box.musicStyleOnce.Do(func() error { return nil })
	}

	return box, box.loadJukebox()
}

// newBox returns a new musly jukebox with the given method and decoder
func newBox(db *bolt.DB, path string, method string, decoder string) (*Box, error) {
	jukebox, err := newJukebox(method, decoder)
	if err != nil {
		db.Close()
		return nil, err
	}

	box := &Box{
		modified:    1,
		jukebox:     jukebox,
		trackSize:   jukebox.trackSize(),
		DecoderName: C.GoString(jukebox.decoder_name),
		MethodName:  C.GoString(jukebox.method_name),
		Path:        path,
		DB:          db,
	}

	err = db.Update(func(tx *bolt.Tx) error {
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
		err = b.Put(jukeboxMusicStyle, []byte("false"))
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

// Close closes a box and stores state to the path given on creation. Depending
// on database size, this can take a few seconds.
func (b *Box) Close() error {
	// note: locking in this function is a mess due to relocking by storeJukebox
	// internally, make sure lock logic is correct when modifying this code.
	b.jukeboxMu.RLock()
	if b.jukebox == nil {
		b.jukeboxMu.RUnlock()
		return nil
	}
	b.jukeboxMu.RUnlock()
	if atomic.LoadInt64(&b.allocs) != 0 {
		fmt.Println("musly: unfreed tracks exist:", b.allocs)
	}

	if b.modified.IsSet() {
		err := b.storeJukebox()
		if err != nil {
			return err
		}
	}

	b.jukeboxMu.Lock()
	C.musly_jukebox_poweroff(b.jukebox)
	b.jukebox = nil
	b.jukeboxMu.Unlock()

	return b.DB.Close()
}

// errorOnce is similar to sync.Once with two exceptions
//
// 1. it returns an error from Do; a non-nil error means the function passed
//    to Do will be called again. In other words Do is called until a nil error
//    is returned from the function.
// 2. it does not mark done when a panic occurs in f
type errorOnce struct {
	m    sync.Mutex
	done uint32
}

func (o *errorOnce) Do(f func() error) error {
	if atomic.LoadUint32(&o.done) == 1 {
		return nil
	}

	o.m.Lock()
	defer o.m.Unlock()
	if o.done == 0 {
		err := f()
		if err != nil {
			return err
		}
		atomic.StoreUint32(&o.done, 1)
	}

	return nil
}

type atomicBool int32

func (b *atomicBool) Set() {
	atomic.StoreInt32((*int32)(b), 1)
}

func (b *atomicBool) IsSet() bool {
	return atomic.LoadInt32((*int32)(b)) == 1
}
