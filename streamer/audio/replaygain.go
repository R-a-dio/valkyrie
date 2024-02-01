package audio

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os/exec"
)

func DecodeFileGain(path string) (*PCMBuffer, error) {
	cmd, buf, err := newFFmpegWithReplaygain(path)
	if err != nil {
		return nil, err
	}

	err = cmd.Start()
	if err != nil {
		return nil, err
	}

	go func() {
		err := cmd.Wait()
		if err != nil {
			err = &DecodeError{
				Err:       err,
				ExtraInfo: cmd.Stderr.(*bytes.Buffer).String(),
			}

			buf.SetError(err)
		} else {
			buf.Close()
		}
	}()

	return buf, nil
}

func newFFmpegWithReplaygain(filename string) (*exec.Cmd, *PCMBuffer, error) {
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

	output := new(bytes.Buffer)
	cmd := exec.Command("ffmpeg", args...)
	cmd.Stderr = output
	cmd.Stdout = new(bytes.Buffer) // we throw this away, but supply a buffer to be sure

	err := cmd.Run()
	if err != nil {
		return nil, nil, err
	}

	b := output.Bytes()
	last := bytes.LastIndex(b, []byte("{"))
	b = b[last-1:]

	// TODO: log results of the replaygain calculation maybe

	// do things with output
	var info = new(replaygainInfo)

	err = json.Unmarshal(b, info)
	if err != nil {
		return nil, nil, err
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

	// prepare the os/exec command and give us access to output pipes
	cmd = exec.Command("ffmpeg", args...)
	cmd.Stdout = NewPCMBuffer(AudioFormat{2, 2, 44100})
	// stderr is only used when an error is reported by exec.Cmd
	cmd.Stderr = new(bytes.Buffer)

	return cmd, cmd.Stdout.(*PCMBuffer), nil
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
