package audio

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
)

// DecodeFileGain decodes the path given with replaygain applied
func DecodeFileGain(ctx context.Context, af AudioFormat, filename string) (*PCMReader, error) {
	ff, err := newFFmpegWithReplaygain(ctx, filename)
	if err != nil {
		return nil, err
	}

	mb, err := ff.Run(ctx)
	if err != nil {
		return nil, err
	}
	defer ff.Close()

	mr, err := mb.Reader()
	if err != nil {
		ff.Close()
		return nil, err
	}

	return NewPCMReader(af, mr), nil
}

func newFFmpegWithReplaygain(ctx context.Context, filename string) (*ffmpeg, error) {
	const (
		// target loudness in LUFs (Loudness Units Full Scale)
		I = -14
		// true peak
		TP = 0
		// loudness range, this describes the overall loduness range,
		// from the softest part to the loudest part.
		LRA = 11
	)
	var settings = fmt.Sprintf("I=%d:TP=%d:LRA=%d", I, TP, LRA)
	// analyze track first
	args := []string{
		"-hide_banner",
		"-i", filename,
		"-map", "0:a:0",
		"-af", "loudnorm=print_format=json:" + settings,
		"-f", "null",
		"-",
	}

	ff, err := newFFmpegCmd(ctx, filename, args)
	if err != nil {
		return nil, err
	}

	// ffmpeg outputs the analyzes on stderr
	data, err := ff.ErrOutput(ctx)
	if err != nil {
		return nil, err
	}

	last := bytes.LastIndex(data, []byte("{"))
	if last > 0 {
		data = data[last-1:]
	}

	// do things with output
	var info = new(replaygainInfo)

	err = json.Unmarshal(data, info)
	if err != nil {
		return nil, err
	}

	// prepare arguments for second pass
	var replayinfo = "loudnorm=linear=true:" + settings
	replayinfo += fmt.Sprintf(":measured_I=%s:measured_LRA=%s:measured_TP=%s",
		info.InputI, info.InputLra, info.InputTp)
	replayinfo += fmt.Sprintf(":measured_thresh=%s:offset=%s",
		info.InputThresh, info.TargetOffset)

	args = []string{
		"-hide_banner",
		"-loglevel", "error",
		"-i", filename,
		"-af", replayinfo,
		"-f", "s16le",
		"-ac", "2",
		"-ar", "44100",
		"-acodec", "pcm_s16le",
		"-",
	}

	return newFFmpegCmd(ctx, filename, args)
}

type replaygainInfo struct {
	InputI            string `json:"input_i"`
	InputTp           string `json:"input_tp"`
	InputLra          string `json:"input_lra"`
	InputThresh       string `json:"input_thresh"`
	OutputI           string `json:"output_i"`
	OutputTp          string `json:"output_tp"`
	OutputLra         string `json:"output_lra"`
	OutputThresh      string `json:"output_thresh"`
	NormalizationType string `json:"normalization_type"`
	TargetOffset      string `json:"target_offset"`
}
