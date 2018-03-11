// fuck anime

// WARNING: This file has automatically been generated on Fri, 25 Aug 2017 21:58:00 UTC.
// By https://git.io/c-for-go. DO NOT EDIT.

package musly

/*
#cgo LDFLAGS: -llibmusly
#include <musly/musly_types.h>
#include <musly/musly.h>
#include <stdlib.h>
*/
import "C"

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"math/rand"
	"runtime"
	"sort"
	"strings"
	"sync/atomic"
	"time"
	"unsafe"

	"github.com/boltdb/bolt"
	"golang.org/x/sync/errgroup"
)

// DatabaseVersion indicates the internal version of the boltdb
var DatabaseVersion = []byte{0, 0, 1}

var (
	metadataBucket       = []byte("metadata")
	metadataMethod       = []byte("method")
	metadataDecoder      = []byte("decoder")
	metadataVersion      = []byte("database_version")
	metadataBinNumTracks = []byte("bin_num_tracks")
)

// The headers say this is a float, and that you are then
// getting/passing float*, but the values in the pointer
// are pretty much useless to us so there's no reason to
// expose it as a float.
type Track = *C.musly_track

type TrackID = C.musly_trackid

// musly_version function as declared in musly/musly.h:58
func Version() string {
	ret := C.musly_version()
	return C.GoString(ret)
}

// musly_debug function as declared in musly/musly.h:70
func Debug(level int) {
	C.musly_debug((C.int)(level))
}

// musly_jukebox_listmethods function as declared in musly/musly.h:82
func ListMethods() []string {
	ret := C.musly_jukebox_listmethods()
	str := C.GoString(ret)
	return strings.Split(str, ",")
}

// musly_jukebox_listdecoders function as declared in musly/musly.h:95
func ListDecoders() []string {
	ret := C.musly_jukebox_listdecoders()
	str := C.GoString(ret)
	return strings.Split(str, ",")
}

type Box struct {
	// allocs counts the amount of tracks we've allocated and not freed yet
	allocs int64
	// modified indicates if our musly_jukebox has been modified
	modified int32

	jukebox     *C.musly_jukebox
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

	jukebox := jukeboxPowerOn(method, decoder)
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
	jukebox := jukeboxPowerOn(method, decoder)
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

// initializes a musly jukebox with the provided options
func jukeboxPowerOn(method, decoder string) *C.musly_jukebox {
	cmethod := C.CString(method)
	if method == "" {
		cmethod = nil
	}
	defer C.free((unsafe.Pointer)(cmethod))
	cdecoder := C.CString(decoder)
	if decoder == "" {
		cdecoder = nil
	}
	defer C.free((unsafe.Pointer)(cdecoder))

	return C.musly_jukebox_poweron(cmethod, cdecoder)
}

// NewTrack returns a fresh musly track as a Go byte slice
func (b *Box) NewTrackBytes() []byte {
	return b.trackToBytes(b.NewTrack())
}

// NewTrack returns a fresh musly track
func (b *Box) NewTrack() Track {
	atomic.AddInt64(&b.allocs, 1)
	return Track(C.musly_track_alloc(b.jukebox))
}

// FreeTrack frees a musly track
func (b *Box) FreeTrack(t Track) {
	atomic.AddInt64(&b.allocs, -1)
	C.musly_track_free(t)
}

func (b *Box) FreeTracks(t []Track) {
	for i := range t {
		if t[i] != nil {
			b.FreeTrack(t[i])
		}
	}
}

// musly_jukebox_aboutmethod function as declared in musly/musly.h:108
func (b *Box) AboutMethod() string {
	ret := C.musly_jukebox_aboutmethod(b.jukebox)
	str := C.GoString(ret)
	return str
}

// Close closes a jukebox and makes it unusable for future calls. Close does
// not save any state of the jukebox, call one of the Save functions before
// calling Close if desired.
func (b *Box) Close() error {
	if b.jukebox == nil {
		panic("musly: close on nil box")
	}
	if b.allocs != 0 {
		fmt.Println("tracks alive on close:", b.allocs)
	}

	if atomic.LoadInt32(&b.modified) != 0 {
		err := b.storeJukebox()
		if err != nil {
			return err
		}
	}

	C.musly_jukebox_poweroff(b.jukebox)
	b.jukebox = nil

	return b.DB.Close()
}

// SetMusicStyle primes the algorithm used with the tracks given.
// See the musly documentation on musly_jukebox_setmusicstyle for details.
//
// Calling this function invalidates any tracks previously added with AddTracks
//
// This function will use a maximum of 1000 tracks given, if len(tracks) > 1000
// the tracks given to the algorithm will be randomly selected for you
func (b *Box) SetMusicStyle(tracks []Track) error {
	if len(tracks) == 0 {
		return errors.New("musly: empty track list")
	}

	usableTracks := tracks
	// limit the amount of tracks if we got given more
	if len(tracks) > 1000 {
		var amount int
		if len(tracks) < 1500 {
			// lower our amount by about 10% if we're only
			// slightly above the 1000 count
			amount = len(tracks) - len(tracks)/10
		} else {
			amount = 1000
		}
		var index = make(map[int]struct{}, amount)

		rnd := rand.New(rand.NewSource(time.Now().UnixNano()))

		for len(index) <= amount {
			i := rnd.Intn(len(tracks))
			index[i] = struct{}{}
		}

		usableTracks = make([]Track, 0, amount)
		for k := range index {
			usableTracks = append(usableTracks, tracks[k])
		}
	}

	ret := C.musly_jukebox_setmusicstyle(
		b.jukebox,
		(**C.musly_track)(&usableTracks[0]),
		C.int(len(usableTracks)),
	)
	if ret < 0 {
		return errors.New("musly: failed setmusicstyle")
	}

	return nil
}

// AddTracks adds the given tracks to the box with the given TrackIDs, both
// slices should have equal length
func (b *Box) AddTracks(tracks []Track, ids []TrackID) error {
	if len(tracks) == 0 {
		fmt.Println("musly: warning: empty track list given")
		return nil
	}

	if len(tracks) != len(ids) {
		return errors.New("musly: unequal length tracks and ids given")
	}

	atomic.StoreInt32(&b.modified, 1)

	ret := C.musly_jukebox_addtracks(
		b.jukebox,
		(**C.musly_track)(&tracks[0]),
		(*C.musly_trackid)(&ids[0]),
		C.int(len(tracks)),
		0,
	)
	if ret < 0 {
		return errors.New("musly: failed addtracks")
	}

	return nil
}

// RemoveTracks removes the given TrackIDs from the box
func (b *Box) RemoveTracks(ids []TrackID) error {
	if len(ids) == 0 {
		return nil
	}

	atomic.StoreInt32(&b.modified, 1)

	ret := C.musly_jukebox_removetracks(
		b.jukebox,
		(*C.musly_trackid)(&ids[0]),
		C.int(len(ids)),
	)
	if ret < 0 {
		return errors.New("musly: failed removetracks")
	}
	return nil
}

// TrackCount returns the amount of tracks added to the box
func (b *Box) TrackCount() int {
	ret := C.musly_jukebox_trackcount(b.jukebox)
	if ret < 0 {
		return 0
	}
	return int(ret)
}

// MaxTrackID returns the highest trackid registered with the box
func (b *Box) MaxTrackID() TrackID {
	ret := TrackID(C.musly_jukebox_maxtrackid(b.jukebox))
	if ret == -1 {
		return 0
	}
	return ret
}

// musly_jukebox_gettrackids function as declared in musly/musly.h:288
func (J *Box) GetTrackIDs() ([]TrackID, error) {
	numtracks := J.TrackCount()
	if numtracks == 0 {
		return []TrackID{}, nil
	}
	ids := make([]TrackID, numtracks)
	var ctrackids *C.musly_trackid = (*C.musly_trackid)(&ids[0])
	ret := C.musly_jukebox_gettrackids(J.jukebox, ctrackids)
	if ret == -1 {
		return nil, errors.New("musly: failed")
	}
	if int(ret) > len(ids) {
		panic("musly buffer overflow")
	}
	return ids, nil
}

func (b *Box) Similarity(seed TrackID, against []TrackID) ([]float32, error) {
	seedTrack, err := b.Track(seed)
	if err != nil {
		return nil, err
	}
	defer b.FreeTrack(seedTrack)

	againstTracks, err := b.Tracks(against)
	if err != nil {
		return nil, err
	}
	defer b.FreeTracks(againstTracks)

	similarity := make([]float32, len(against))

	ret := C.musly_jukebox_similarity(
		b.jukebox,
		seedTrack,
		seed,
		(**C.musly_track)(&againstTracks[0]),
		(*C.musly_trackid)(&against[0]),
		C.int(len(against)),
		(*C.float)(&similarity[0]),
	)
	if ret < 0 {
		return nil, errors.New("musly: similarity failed")
	}

	return similarity, nil
}

func (b *Box) ParallelSimilarity(seed TrackID, against []TrackID) ([]float32, error) {
	g, ctx := errgroup.WithContext(context.Background())
	var ch = make(chan int)
	var groupSize = 50
	var results = make([]float32, len(against))

	for i := 0; i < runtime.NumCPU(); i++ {
		g.Go(func() error {
			for start := range ch {
				end := start + groupSize
				if end > len(against) {
					end = len(against)
				}

				sim, err := b.Similarity(seed, against[start:end])
				if err != nil {
					return err
				}

				copy(results[start:end], sim)
			}

			return nil
		})
	}

	go func() {
		for i := 0; i < len(against); i += groupSize {
			select {
			case ch <- i:
			case <-ctx.Done():
				break
			}
		}

		close(ch)
	}()

	err := g.Wait()
	if err != nil {
		return nil, err
	}

	return results, nil
}

/*
// musly_jukebox_similarity function as declared in musly/musly.h:333
func (J *Jukebox) Similarity(Seed *Track, SeedID TrackID, Tracks []*Track, TrackIDs []TrackID) ([]float32, error) {
	numtracks := len(Tracks)
	if len(TrackIDs) != numtracks {
		return nil, errors.New("musly: Tracks and TrackIDs are different length")
	}
	ptr := (unsafe.Pointer)(&Tracks[0])
	ctracks := (**C.musly_track)(ptr)
	ctrackids := (*C.musly_trackid)(&TrackIDs[0])
	simil := make([]float32, numtracks)
	csimil := (*C.float)(&simil[0])
	ret := C.musly_jukebox_similarity(J.jukebox, (*C.musly_track)(Seed), (C.musly_trackid)(SeedID), ctracks, ctrackids, C.int(numtracks), csimil)
	if ret == -1 {
		return nil, errors.New("musly: failed")
	}
	return simil, nil
}*/

func (b *Box) GuessNeighbors(seed TrackID, maxNeighbors int, filter []TrackID) ([]TrackID, error) {
	if maxNeighbors == 0 {
		return nil, nil
	}

	ids := make([]TrackID, maxNeighbors)

	var ret C.int
	if len(filter) > 0 {
		ret = C.musly_jukebox_guessneighbors_filtered(
			b.jukebox,
			seed,
			(*C.musly_trackid)(&ids[0]),
			C.int(maxNeighbors),
			(*C.musly_trackid)(&filter[0]),
			C.int(len(filter)),
		)
	} else {
		ret = C.musly_jukebox_guessneighbors(
			b.jukebox,
			seed,
			(*C.musly_trackid)(&ids[0]),
			C.int(maxNeighbors),
		)
	}

	if ret < 0 {
		return nil, errors.New("musly: failed to guess neighbors")
	}
	return ids[:int(ret)], nil
}

// musly_jukebox_binsize function as declared in musly/musly.h:434
func (b *Box) jukeboxBinSize(header int, num int) int {
	ret := C.musly_jukebox_binsize(b.jukebox, C.int(header), C.int(num))
	return int(ret)
}

// musly_jukebox_tobin function as declared in musly/musly.h:469
func (b *Box) jukeboxToBin(p []byte, header int, num int, skip int) (int, error) {
	ret := C.musly_jukebox_tobin(
		b.jukebox,
		(*C.uchar)(&p[0]),
		C.int(header),
		C.int(num),
		C.int(skip),
	)

	if ret == -1 {
		return 0, errors.New("musly: failed jukebox_tobin")
	}
	return int(ret), nil
}

func (b *Box) jukeboxFromBin(p []byte, header int, num int) (int, error) {
	ret := C.musly_jukebox_frombin(
		b.jukebox,
		(*C.uchar)(&p[0]),
		C.int(header),
		C.int(num),
	)

	if ret == -1 {
		return 0, errors.New("musly: failed jukebox_frombin")
	}

	return int(ret), nil
}

// musly_jukebox_tofile function as declared in musly/musly.h:573
func (b *Box) SaveFile(path string) error {
	cpath := C.CString(path)
	defer C.free(unsafe.Pointer(cpath))
	ret := C.musly_jukebox_tofile(b.jukebox, cpath)
	if ret == -1 {
		return errors.New("musly: failed to tofile")
	}
	return nil
}

// musly_jukebox_fromfile function as declared in musly/musly.h:593
func LoadFile(path string) (*Box, error) {
	cpath := C.CString(path)
	defer C.free(unsafe.Pointer(cpath))
	box := C.musly_jukebox_fromfile(cpath)
	if box == nil {
		return nil, errors.New("musly: failed")
	}

	b := Box{
		jukebox:     box,
		Path:        path,
		DecoderName: C.GoString(box.decoder_name),
		MethodName:  C.GoString(box.method_name),
	}
	return &b, nil
}

// TrackToBytes turns Track into a byte slice with no copying
func (b *Box) trackToBytes(t Track) []byte {
	length := b.TrackSize()
	return (*[1 << 30]byte)(unsafe.Pointer(t))[:length:length]
}
func (b *Box) TrackToBytes(t Track) []byte {
	length := b.TrackSize()
	return (*[1 << 30]byte)(unsafe.Pointer(t))[:length:length]
}

// BytesToTrack turns a byte slice into a Track with no copying
func (b *Box) bytesToTrack(p []byte) Track {
	return Track(unsafe.Pointer(&p[0]))
}

// musly_track_size function as declared in musly/musly.h:640
func (b *Box) TrackSize() int {
	return int(C.musly_track_size(b.jukebox))
}

// musly_track_binsize function as declared in musly/musly.h:656
func (b *Box) TrackBinSize() int {
	return int(C.musly_track_binsize(b.jukebox))
}

// musly_track_tobin function as declared in musly/musly.h:680
func (b *Box) TrackToBin(track Track) ([]byte, error) {
	buf := make([]byte, b.TrackBinSize())
	ret := C.musly_track_tobin(b.jukebox,
		track,
		(*C.uchar)(&buf[0]),
	)
	if ret < 0 {
		return nil, errors.New("musly: failed track_tobin")
	}
	return buf[:ret], nil
}

// musly_track_frombin function as declared in musly/musly.h:705
func (b *Box) TrackFromBin(buf []byte) (Track, error) {
	track := b.NewTrack()
	ret := C.musly_track_frombin(
		b.jukebox,
		(*C.uchar)(&buf[0]),
		track,
	)
	if ret == -1 {
		return nil, errors.New("musly: failed")
	}
	return track, nil
}

// musly_track_tostr function as declared in musly/musly.h:726
func (b *Box) TrackToStr(track Track) string {
	ret := C.musly_track_tostr(b.jukebox, track)
	return C.GoString(ret)
}

/*
// musly_track_analyze_pcm function as declared in musly/musly.h:756
func (J *Jukebox) AnalyzePcm(Pcm []float32) (*Track, error) {
	length := len(Pcm)
	track := J.CreateTrack()
	ctrack := (*C.musly_track)(track)
	cpcm := (*C.float)(&Pcm[0])
	ret := C.musly_track_analyze_pcm(J.jukebox, cpcm, C.int(length), ctrack)
	if ret == -1 {
		return nil, errors.New("musly: failed")
	}
	return track, nil
}
*/

// musly_track_analyze_audiofile function as declared in musly/musly.h:798
func (b *Box) AnalyzeAudioFile(id TrackID, path string, len float32, start float32) error {
	cpath := C.CString(path)
	defer C.free(unsafe.Pointer(cpath))
	track := b.NewTrack()
	defer b.FreeTrack(track)

	ret := C.musly_track_analyze_audiofile(
		b.jukebox,
		cpath,
		C.float(len),
		C.float(start),
		track,
	)
	if ret == -1 {
		return errors.New("musly: failed analyzing audio")
	}

	return b.StoreTrack(track, id)
}

/*
// musly_findmin function as declared in musly/musly.h:826
func FindMin(values []float32, ids []musly_trackid, count int32, min_values []float32, min_ids []musly_trackid, min_count int32, ordered int32) int32 {
	cvalues, _ := (*C.float)(unsafe.Pointer((*sliceHeader)(unsafe.Pointer(&values)).Data)), cgoAllocsUnknown
	cids, _ := (*C.musly_trackid)(unsafe.Pointer((*sliceHeader)(unsafe.Pointer(&ids)).Data)), cgoAllocsUnknown
	ccount, _ := (C.int)(count), cgoAllocsUnknown
	cmin_values, _ := (*C.float)(unsafe.Pointer((*sliceHeader)(unsafe.Pointer(&min_values)).Data)), cgoAllocsUnknown
	cmin_ids, _ := (*C.musly_trackid)(unsafe.Pointer((*sliceHeader)(unsafe.Pointer(&min_ids)).Data)), cgoAllocsUnknown
	cmin_count, _ := (C.int)(min_count), cgoAllocsUnknown
	cordered, _ := (C.int)(ordered), cgoAllocsUnknown
	__ret := C.musly_findmin(cvalues, cids, ccount, cmin_values, cmin_ids, cmin_count, cordered)
	__v := (int32)(__ret)
	return __v
}
*/

func FindMin(sim []float32, ids []TrackID) {
	sort.Sort(simSort{sim, ids})
}

type simSort struct {
	sim []float32
	ids []TrackID
}

func (s simSort) Len() int {
	return len(s.sim)
}

func (s simSort) Swap(i, j int) {
	s.sim[i], s.sim[j] = s.sim[j], s.sim[i]
	s.ids[i], s.ids[j] = s.ids[j], s.ids[i]
}

func (s simSort) Less(i, j int) bool {
	return s.sim[i] < s.sim[j]
}
