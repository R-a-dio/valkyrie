package gomusly

/*
#cgo LDFLAGS: -lmusly
#include <musly/musly_types.h>
#include <musly/musly.h>
#include <stdlib.h>
*/
import "C"

import (
	"errors"
	"runtime"
	"strings"
	"unsafe"
)

type Jukebox struct {
	jukebox *C.musly_jukebox
}

// The headers say this is a float, and that you are then
// getting/passing float*, but the values in the pointer
// are pretty much useless to us so there's no reason to
// expose it as a float.
type Track C.musly_track

type TrackID int32

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

// musly_jukebox_poweron function as declared in musly/musly.h:136
func PowerOn(method string, decoder string) (*Jukebox, error) {
	cmethod := C.CString(method)
	defer C.free((unsafe.Pointer)(cmethod))
	cdecoder := C.CString(decoder)
	defer C.free((unsafe.Pointer)(cdecoder))

	cjuke := C.musly_jukebox_poweron(cmethod, cdecoder)
	if cjuke == nil {
		return nil, errors.New("musly: power on failed")
	}
	return &Jukebox{cjuke}, nil
}

// musly_jukebox_aboutmethod function as declared in musly/musly.h:108
func (J *Jukebox) AboutMethod() string {
	ret := C.musly_jukebox_aboutmethod(J.jukebox)
	str := C.GoString(ret)
	return str
}

// musly_jukebox_poweroff function as declared in musly/musly.h:152
func (J *Jukebox) PowerOff() {
	if J.jukebox == nil {
		panic("musly: powered off dead jukebox")
	}
	C.musly_jukebox_poweroff(J.jukebox)
	J.jukebox = nil
}

func trackFinalizer(T *Track) {
	if T != nil {
		C.musly_track_free((*C.musly_track)(T))
	}
}

// musly_track_alloc function as declared in musly/musly.h:609
func (J *Jukebox) CreateTrack() *Track {
	track := C.musly_track_alloc(J.jukebox)
	runtime.SetFinalizer(track, trackFinalizer)
	return (*Track)(track)
}

// musly_jukebox_setmusicstyle function as declared in musly/musly.h:183
func (J *Jukebox) SetMusicStyle(Tracks []*Track) error {
	numtracks := len(Tracks)
	if numtracks == 0 {
		return errors.New("musly: Tracks was empty")
	}
	ptr := (unsafe.Pointer)(&Tracks[0])
	ctracks := (**C.musly_track)(ptr)
	ret := C.musly_jukebox_setmusicstyle(J.jukebox, ctracks, (C.int)(numtracks))
	if ret == -1 {
		return errors.New("musly: failed")
	}
	return nil
}


// musly_jukebox_addtracks function as declared in musly/musly.h:219
func (J *Jukebox) AddTracks(Tracks []*Track, TrackIDs []TrackID) ([]TrackID, error) {
	numtracks := len(Tracks)
	if numtracks == 0 {
		return nil, errors.New("musly: Tracks was empty")
	}
	ptr := (unsafe.Pointer)(&Tracks[0])
	ctracks := (**C.musly_track)(ptr)
	var generate_ids C.int
	var ctrackids *C.musly_trackid
	if len(TrackIDs) > 0 {
		if len(TrackIDs) != numtracks {
			return nil, errors.New("musly: number of track ids does not match number of tracks")
		}
		generate_ids = 1
	} else {
		TrackIDs = make([]TrackID, numtracks)
		generate_ids = 0
	}
	ctrackids = (*C.musly_trackid)(&TrackIDs[0])

	ret := C.musly_jukebox_addtracks(J.jukebox, ctracks, ctrackids, C.int(numtracks), generate_ids)
	if ret != -1 {
		return nil, errors.New("musly: failed")
	}
	// TrackIDs should now contain the result.
	return TrackIDs, nil
}


// musly_jukebox_removetracks function as declared in musly/musly.h:239
func (J *Jukebox) RemoveTracks(TrackIDs []TrackID) error {
	numtracks := len(TrackIDs)
	if numtracks == 0 {
		return errors.New("musly: TrackIDs was empty")
	}
	var ctrackids *C.musly_trackid = (*C.musly_trackid)(&TrackIDs[0])
	ret := C.musly_jukebox_removetracks(J.jukebox, ctrackids, C.int(numtracks))
	if ret == -1 {
		return errors.New("musly: failed")
	}
	return nil
}


// musly_jukebox_trackcount function as declared in musly/musly.h:256
func (J *Jukebox) TrackCount() (int, error) {
	ret := int(C.musly_jukebox_trackcount(J.jukebox))
	if ret == -1 {
		return 0, errors.New("musly: failed")
	}
	return ret, nil
}


// musly_jukebox_maxtrackid function as declared in musly/musly.h:273
func (J *Jukebox) MaxTrackID() (TrackID, error) {
	ret := TrackID(C.musly_jukebox_maxtrackid(J.jukebox))
	if ret == -1 {
		return 0, errors.New("musly: no tracks seen so far")
	}
	return ret, nil
}


// musly_jukebox_gettrackids function as declared in musly/musly.h:288
func (J *Jukebox) GetTrackIDs() ([]TrackID, error) {
	numtracks, ok := J.TrackCount()
	if ok != nil {
		return nil, ok
	}
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
}


// musly_jukebox_guessneighbors function as declared in musly/musly.h:369
func (J *Jukebox) GuessNeighbors(Seed TrackID, MaxNeighbors int, Filter []TrackID) ([]TrackID, error) {
	if MaxNeighbors == 0 {
		return []TrackID{}, nil
	}
	ids := make([]TrackID, MaxNeighbors)
	ctrackids := (*C.musly_trackid)(&ids[0])
	var ret C.int
	if Filter == nil {
		ret = C.musly_jukebox_guessneighbors(J.jukebox, C.musly_trackid(Seed), ctrackids, C.int(MaxNeighbors))
	} else {
		cfilter := (*C.musly_trackid)(&Filter[0])
		ret = C.musly_jukebox_guessneighbors_filtered(J.jukebox, C.musly_trackid(Seed), ctrackids, C.int(MaxNeighbors), cfilter, C.int(len(Filter)))
	}
	if ret == -1 {
		return nil, errors.New("musly: failed")
	}
	return ids[:ret], nil
}

// musly_jukebox_binsize function as declared in musly/musly.h:434
func (J *Jukebox) BinSize(WithHeader bool, NumTracks int) (int, error) {
	if NumTracks == 0 {
		return 0, errors.New("musly: NumTracks cannot be zero")
	}
	var cheader C.int
	if WithHeader {
		cheader = 1
	} else {
		cheader = 0
	}
	ret := C.musly_jukebox_binsize(J.jukebox, cheader, C.int(NumTracks))
	return int(ret), nil
}


// musly_jukebox_tobin function as declared in musly/musly.h:469
func (J *Jukebox) ToBin(WithHeader bool, NumTracks int, SkipTracks int) ([]byte, error) {
	if NumTracks == 0 {
		return nil, errors.New("musly: NumTracks cannot be zero")
	}
	var cheader C.int
	if WithHeader {
		cheader = 1
	} else {
		cheader = 0
	}
	binsize, ok := J.BinSize(WithHeader, NumTracks)
	if ok != nil {
		return nil, ok
	}
	bytes := make([]byte, binsize)
	cbuffer := (*C.uchar)(&bytes[0])

	ret := C.musly_jukebox_tobin(J.jukebox, cbuffer, cheader, C.int(NumTracks), C.int(SkipTracks))
	if ret == -1 {
		return nil, errors.New("musly: failed")
	}
	return bytes[:ret], nil
}


// musly_jukebox_frombin function as declared in musly/musly.h:506
func (J *Jukebox) FromBin(Buffer []byte, WithHeader bool, NumTracks int) (int, error) {
	if NumTracks == 0 {
		return 0, errors.New("musly: NumTracks cannot be zero")
	}
	var cheader C.int
	if WithHeader {
		cheader = 1
	} else {
		cheader = 0
	}
	cbuffer := (*C.uchar)(&Buffer[0])
	ret := C.musly_jukebox_frombin(J.jukebox, cbuffer, cheader, C.int(NumTracks))
	if ret == -1 {
		return 0, errors.New("musly: failed")
	}
	return int(ret), nil
}


// musly_jukebox_tofile function as declared in musly/musly.h:573
func (J *Jukebox) ToFile(Filename string) (int, error) {
	cfilename := C.CString(Filename)
	defer C.free(unsafe.Pointer(cfilename))
	ret := C.musly_jukebox_tofile(J.jukebox, cfilename)
	if ret == -1 {
		return 0, errors.New("musly: failed")
	}
	return int(ret), nil
}

// musly_jukebox_fromfile function as declared in musly/musly.h:593
func (J *Jukebox) FromFile(Filename string) (*Jukebox, error) {
	cfilename := C.CString(Filename)
	defer C.free(unsafe.Pointer(cfilename))
	ret := C.musly_jukebox_fromfile(cfilename)
	if ret == nil {
		return nil, errors.New("musly: failed")
	}
	return &Jukebox{ret}, nil
}

// musly_track_size function as declared in musly/musly.h:640
func (J *Jukebox) TrackSize() int {
	ret := C.musly_track_size(J.jukebox)
	return int(ret)
}

// musly_track_binsize function as declared in musly/musly.h:656
func (J *Jukebox) TrackBinSize() int {
	ret := C.musly_track_binsize(J.jukebox)
	return int(ret)
}


// musly_track_tobin function as declared in musly/musly.h:680
func (J *Jukebox) TrackToBin(FromTrack *Track) ([]byte, error) {
	buffer := make([]byte, J.TrackBinSize())
	cbuffer := (*C.uchar)(&buffer[0])
	ret := C.musly_track_tobin(J.jukebox, (*C.musly_track)(FromTrack), cbuffer)
	if ret == -1 {
		return nil, errors.New("musly: failed")
	}
	return buffer[:ret], nil
}


// musly_track_frombin function as declared in musly/musly.h:705
func (J *Jukebox) TrackFromBin(Buffer []byte) (*Track, error) {
	cbuffer := (*C.uchar)(&Buffer[0])
	track := J.CreateTrack()
	ctrack := (*C.musly_track)(track)
	ret := C.musly_track_frombin(J.jukebox, cbuffer, ctrack)
	if ret == -1 {
		return nil, errors.New("musly: failed")
	}
	return track, nil
}


// musly_track_tostr function as declared in musly/musly.h:726
func (J *Jukebox) TrackToStr(FromTrack *Track) string {
	ctrack := (*C.musly_track)(FromTrack)
	ret := C.musly_track_tostr(J.jukebox, ctrack)
	return C.GoString(ret)
}


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


// musly_track_analyze_audiofile function as declared in musly/musly.h:798
func (J *Jukebox) AnalyzeAudiofile(AudioFile string, Length float32, Start float32) (*Track, error) {
	caudiofile := C.CString(AudioFile)
	defer C.free(unsafe.Pointer(caudiofile))
	track := J.CreateTrack()
	ctrack := (*C.musly_track)(track)
	ret := C.musly_track_analyze_audiofile(J.jukebox, caudiofile, C.float(Length), C.float(Start), ctrack)
	if ret == -1 {
		return nil, errors.New("musly: failed")
	}
	return track, nil
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
