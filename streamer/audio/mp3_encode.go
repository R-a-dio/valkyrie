//go:build !nolame
// +build !nolame

package audio

/*
#cgo LDFLAGS: -lmp3lame
#include <lame/lame.h>
*/
import "C"
import (
	"fmt"
	"unsafe"
)

type LAME struct {
	flags *C.lame_global_flags

	AudioFormat
	out []byte
}

func NewLAME(opt AudioFormat) (*LAME, error) {
	var l LAME

	l.AudioFormat = opt
	l.flags = C.lame_init()
	l.out = make([]byte, 1024*16)

	// set input parameters
	ret := C.lame_set_in_samplerate(l.flags, C.int(opt.SampleRate))
	if ret < 0 {
		return nil, fmt.Errorf("lame: invalid input samplerate: %d",
			opt.SampleRate)
	}

	ret = C.lame_set_num_channels(l.flags, C.int(opt.ChannelCount))
	if ret < 0 {
		return nil, fmt.Errorf("lame: invalid channel amount: %d",
			opt.ChannelCount)
	}

	// set output parameters
	ret = C.lame_set_out_samplerate(l.flags, C.int(opt.SampleRate))
	if ret < 0 {
		return nil, fmt.Errorf("lame: invalid output samplerate: %d",
			opt.SampleRate)
	}
	// 192kbps CBR
	ret = C.lame_set_brate(l.flags, C.int(192))
	if ret < 0 {
		return nil, fmt.Errorf("lame: invalid bitrate: 192")
	}
	// don't write a XING header
	ret = C.lame_set_bWriteVbrTag(l.flags, C.int(0))
	if ret < 0 {
		return nil, fmt.Errorf("lame: invalid XING tag option")
	}
	// close to best quality encoding
	ret = C.lame_set_quality(l.flags, C.int(2))
	if ret < 0 {
		return nil, fmt.Errorf("lame: invalid quality setting: 2")
	}
	// enable joint stereo
	ret = C.lame_set_mode(l.flags, C.MPEG_mode(1))
	if ret < 0 {
		return nil, fmt.Errorf("lame: invalid mode set: 1 (joint stereo)")
	}

	ret = C.lame_init_params(l.flags)
	if ret < 0 {
		C.lame_close(l.flags)
		return nil, fmt.Errorf("lame: failed init: %d", int(ret))
	}

	return &l, nil
}

// Encode encodes in and returns available mp3 data, out is only valid until
// the next call to Encode
func (l *LAME) Encode(in []byte) (out []byte, err error) {
	var sampleCount = len(in) / l.BytesPerSample / l.ChannelCount
	var expectedSize = int(1.25*float32(sampleCount) + 7200)
	if expectedSize > len(l.out) {
		// grow our buffer if our wide estimate shows we need more
		l.out = make([]byte, expectedSize)
	}

	out = l.out

	ret := C.lame_encode_buffer_interleaved(l.flags,
		(*C.short)(unsafe.Pointer(&in[0])),
		C.int(sampleCount),
		(*C.uchar)(unsafe.Pointer(&out[0])),
		C.int(len(out)),
	)
	if ret < 0 {
		switch ret {
		case -1:
			err = fmt.Errorf("lame: encode error: buf too small: %d", len(out))
		case -2:
			err = fmt.Errorf("lame: encode error: malloc problem")
		case -3:
			err = fmt.Errorf("lame: encode error: lame_init_params not called")
		case -4:
			err = fmt.Errorf("lame: encode error: psycho acoustic problems")
		default:
			err = fmt.Errorf("lame: encode error (%d)", int(ret))
		}
		return nil, err
	}

	return out[:int(ret)], nil
}

// Flush flushes internal buffers and returns any mp3 data found
func (l *LAME) Flush() []byte {
	ret := C.lame_encode_flush_nogap(l.flags,
		(*C.uchar)(unsafe.Pointer(&l.out[0])), C.int(len(l.out)))
	if ret >= 0 {
		return l.out[:int(ret)]
	}
	return nil
}

// Close frees C resources associated with this instance
func (l *LAME) Close() error {
	if l.flags == nil {
		return nil
	}

	C.lame_close(l.flags)
	l.flags = nil
	return nil
}
