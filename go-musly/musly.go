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
	"errors"
	"fmt"
	"math/rand"
	"runtime"
	"sort"
	"strings"
	"sync/atomic"
	"time"
	"unsafe"

	"golang.org/x/sync/errgroup"
)

// track is an array of floats, the size differs based on the musly method
// used. Our Go code has no knowledge of what is inside this array, so we
// handle it around as a binary blob
type track = *C.musly_track

// TrackID is an ID used for internal tracking of tracks
type TrackID = C.musly_trackid

type jukebox = C.musly_jukebox

// addTracks adds the given tracks to the jukebox;
// It's an error if the slices given are not the same length
func (j *jukebox) addTracks(tracks []track, ids []TrackID) error {
	if len(tracks) == 0 {
		return nil
	} else if len(tracks) != len(ids) {
		return errors.New("musly: unequal slices length")
	}

	ret := C.musly_jukebox_addtracks(
		j,
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

func (j *jukebox) removeTracks(ids []TrackID) error {
	ret := C.musly_jukebox_removetracks(
		j,
		(*C.musly_trackid)(&ids[0]),
		C.int(len(ids)),
	)
	if ret < 0 {
		return errors.New("musly: failed removetracks")
	}
	return nil
}

// trackCount returns the amount of tracks added to the jukebox
func (j *jukebox) trackCount() int {
	ret := C.musly_jukebox_trackcount(j)
	if ret < 0 {
		return 0
	}
	return int(ret)
}

// trackIDs returns all IDs registered with the jukebox
//
// This value does not have to be equal to the amount of tracks we have stored
// in our database; Especially when SetMusicStyle hasn't been called yet.
func (j *jukebox) trackIDs() ([]TrackID, error) {
	numtracks := j.trackCount()
	if numtracks == 0 {
		return []TrackID{}, nil
	}

	ids := make([]TrackID, numtracks)

	ret := C.musly_jukebox_gettrackids(
		j,
		(*C.musly_trackid)(&ids[0]),
	)
	if ret < 0 {
		return nil, errors.New("musly: gettrackids")
	}
	if int(ret) > len(ids) {
		panic("musly buffer overflow")
	}
	return ids, nil
}

// musly_jukebox_binsize function as declared in musly/musly.h:434
func (j *jukebox) binSize(header int, num int) int {
	ret := C.musly_jukebox_binsize(j, C.int(header), C.int(num))
	return int(ret)
}

// musly_jukebox_tobin function as declared in musly/musly.h:469
func (j *jukebox) toBin(p []byte, header int, num int, skip int) (int, error) {
	ret := C.musly_jukebox_tobin(
		j,
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

func (j *jukebox) fromBin(p []byte, header int, num int) (int, error) {
	ret := C.musly_jukebox_frombin(
		j,
		(*C.uchar)(&p[0]),
		C.int(header),
		C.int(num),
	)

	if ret == -1 {
		return 0, errors.New("musly: failed jukebox_frombin")
	}

	return int(ret), nil
}

// Version returns libmusly version information
func Version() string {
	ret := C.musly_version()
	return C.GoString(ret)
}

// Debug sets the internal libmusly debug level. Enabling this causes libmusly
// to print debug information to stderr.
//
// Valid values are:
//	 0 (Quiet), 1 (Error), 2 (Warning), 3 (Info), 4 (Debug), 5 (Trace)
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

// initializes a musly jukebox with the provided options
func newJukebox(method, decoder string) *jukebox {
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
func (b *Box) newTrackBytes() []byte {
	return b.trackToBytes(b.newTrack())
}

// NewTrack returns a fresh musly track
func (b *Box) newTrack() track {
	atomic.AddInt64(&b.allocs, 1)
	return track(C.musly_track_alloc(b.jukebox))
}

// FreeTrack frees a musly track
func (b *Box) freeTrack(t track) {
	atomic.AddInt64(&b.allocs, -1)
	C.musly_track_free(t)
}

func (b *Box) freeTracks(t []track) {
	for i := range t {
		if t[i] != nil {
			b.freeTrack(t[i])
		}
	}
}

// Close closes a box and stores state to the path given on creation. Depending
// on database size, this can take a few seconds.
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
// Under normal circumstances, you are not required to call this method, and
// it is handled internally when you first call Similarity
//
// Calling this function invalidates any tracks previously added and requires
// re-adding them, this is handled internally but could make a call to this
// function take some time.
//
// This function will use a maximum of 1000 tracks given, if len(tracks) > 1000
// the tracks given to the algorithm will be randomly selected for you
func (b *Box) SetMusicStyle(ids []TrackID) error {
	if len(ids) == 0 {
		return errors.New("musly: empty track list")
	}

	tracks, err := b.loadTracks(ids)
	if err != nil {
		return err
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

		usableTracks = make([]track, 0, amount)
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

// Similarity calculates a similarity score for the seed given against the list
// of ids given.
//
// Returns a float array with floats in the range 0..1 with 0 being an absolute
// match, and 1 being the furthest away from the seed given. You can use FindMin
// or FindMax to sort the result with the IDs.
func (b *Box) Similarity(seed TrackID, against []TrackID) ([]float32, error) {
	seedTrack, err := b.loadTrack(seed)
	if err != nil {
		return nil, err
	}
	defer b.freeTrack(seedTrack)

	againstTracks, err := b.loadTracks(against)
	if err != nil {
		return nil, err
	}
	defer b.freeTracks(againstTracks)

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

// ParallelSimilarity is like Similarity, except it uses multiple goroutines
// to do so
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
	paraBreak:
		for i := 0; i < len(against); i += groupSize {
			select {
			case ch <- i:
			case <-ctx.Done():
				break paraBreak
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

// trackToBytes casts a track to a byte slice
func (b *Box) trackToBytes(t track) []byte {
	length := b.TrackSize()
	return (*[1 << 30]byte)(unsafe.Pointer(t))[:length:length]
}

// bytesToTrack casts a byte slice to a track
func (b *Box) bytesToTrack(p []byte) track {
	return track(unsafe.Pointer(&p[0]))
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
func (b *Box) TrackToBin(t track) ([]byte, error) {
	buf := make([]byte, b.TrackBinSize())
	ret := C.musly_track_tobin(b.jukebox,
		t,
		(*C.uchar)(&buf[0]),
	)
	if ret < 0 {
		return nil, errors.New("musly: failed track_tobin")
	}
	return buf[:ret], nil
}

// musly_track_frombin function as declared in musly/musly.h:705
func (b *Box) TrackFromBin(buf []byte) (track, error) {
	track := b.newTrack()
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
func (b *Box) TrackToStr(t track) string {
	ret := C.musly_track_tostr(b.jukebox, t)
	return C.GoString(ret)
}

// AnalyzePCM analyzes the PCM signal given and assigns it the ID given.
//
// The PCM signal has to be mono, sampled at 22kHz and float values between
// -1.0 and +1.0
func (b *Box) AnalyzePCM(id TrackID, pcm []byte) error {
	t := b.newTrack()
	defer b.freeTrack(t)

	ret := C.musly_track_analyze_pcm(
		b.jukebox,
		(*C.float)(unsafe.Pointer(&pcm[0])),
		C.int(len(pcm)/4),
		t,
	)
	if ret < 0 {
		return errors.New("musly: failed analyzing PCM")
	}

	return b.storeTrack(t, id)
}

// AnalyzeFile analyzes a file and gives it the ID given internally.
//
// see musly_track_analyze_audiofile in musly.h for details of parameters
func (b *Box) AnalyzeFile(id TrackID, path string, len float32, start float32) error {
	cpath := C.CString(path)
	defer C.free(unsafe.Pointer(cpath))
	t := b.newTrack()
	defer b.freeTrack(t)

	ret := C.musly_track_analyze_audiofile(
		b.jukebox,
		cpath,
		C.float(len),
		C.float(start),
		t,
	)
	if ret == -1 {
		return errors.New("musly: failed analyzing file")
	}

	return b.storeTrack(t, id)
}

// FindMax is the reverse of FindMin
func FindMax(sim []float32, ids []TrackID) {
	sort.Sort(sort.Reverse(simSort{sim, ids}))
}

// FindMin sorts the arguments from closest to furthest match
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
