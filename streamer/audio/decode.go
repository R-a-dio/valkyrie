package audio

import (
	"bytes"
	"fmt"
	"os/exec"
)

// DecodeFile decodes the audio filepath given and returns a PCMBuffer
func DecodeFile(path string) (*PCMBuffer, error) {
	cmd, buf := newFFmpeg(path)

	err := cmd.Start()
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

// DecodeError is returned when a non-zero exit code is returned by the decoder
//
// The ExtraInfo field contains stderr of the decoder process.
type DecodeError struct {
	Err       error
	ExtraInfo string
}

func (e *DecodeError) Error() string {
	return fmt.Sprintf("decode error: %s", e.Err.Error())
}

// newFFmpeg prepares a new ffmpeg process for decoding the filename given. The context
// given is passed to os/exec.Cmd
func newFFmpeg(filename string) (*exec.Cmd, *PCMBuffer) {
	// prepare arguments
	args := []string{
		"-hide_banner",
		"-loglevel", "error",
		"-i", filename,
		"-f", "s16le",
		"-ac", "2",
		"-ar", "44100",
		"-acodec", "pcm_s16le",
		"-",
	}

	// prepare the os/exec command and give us access to output pipes
	cmd := exec.Command("ffmpeg", args...)
	cmd.Stdout = NewPCMBuffer(AudioFormat{2, 2, 44100})
	// stderr is only used when an error is reported by exec.Cmd
	cmd.Stderr = new(bytes.Buffer)

	return cmd, cmd.Stdout.(*PCMBuffer)
}
