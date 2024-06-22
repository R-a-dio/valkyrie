package audio

import (
	"fmt"
	"io"
	"os"
	"os/exec"

	"github.com/justincormack/go-memfd"
)

// DecodeFile decodes the filename given to an in-memory buffer as
// PCM audio data
func DecodeFile(filename string) (*MemoryBuffer, error) {
	ff, err := newFFmpeg(filename)
	if err != nil {
		return nil, err
	}

	return ff.Run()
}

type ffmpeg struct {
	Cmd    *exec.Cmd
	Stdout *MemoryBuffer
	Stderr *memfd.Memfd
}

// newFFmpeg prepares a new ffmpeg process for decoding the filename given. The context
// given is passed to os/exec.Cmd
func newFFmpeg(filename string) (*ffmpeg, error) {
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

	return newFFmpegCmd(filename, args)
}

func newFFmpegCmd(name string, args []string) (*ffmpeg, error) {
	out, err := NewMemoryBuffer(name, nil)
	if err != nil {
		return nil, err
	}

	errOut, err := memfd.Create()
	if err != nil {
		out.Close()
		return nil, err
	}

	// prepare the os/exec command and give us access to output
	cmd := exec.Command("ffmpeg", args...)
	cmd.Stdout = out.Memfd.File
	// stderr is only used when an error is reported by exec.Cmd
	cmd.Stderr = errOut.File
	return &ffmpeg{Cmd: cmd, Stdout: out, Stderr: errOut}, nil
}

func newFFmpegCmdFile(name string, args []string) (*ffmpeg, error) {
	out, err := NewMemoryBuffer(name, nil)
	if err != nil {
		return nil, err
	}

	errOut, err := memfd.Create()
	if err != nil {
		out.Close()
		return nil, err
	}

	args = append(args, "-y", fmt.Sprintf("/proc/%d/fd/%d", os.Getpid(), out.Fd()))

	// prepare the os/exec command and give us access to output
	cmd := exec.Command("ffmpeg", args...)
	//cmd.Stdout = out.Memfd.File
	// stderr is only used when an error is reported by exec.Cmd
	cmd.Stderr = errOut.File
	return &ffmpeg{Cmd: cmd, Stdout: out, Stderr: errOut}, nil
}

func (ff *ffmpeg) Close() error {
	ff.Stdout.Close()
	ff.Stderr.Close()
	return nil
}

func (ff *ffmpeg) ReadError() error {
	_, _ = ff.Stderr.Seek(0, 0)
	out, _ := io.ReadAll(ff.Stderr)

	return fmt.Errorf("stderr: %s", string(out))
}

func (ff *ffmpeg) Output() ([]byte, error) {
	out, err := ff.Run()
	if err != nil {
		return nil, err
	}
	defer out.Close()

	return io.ReadAll(out)
}

func (ff *ffmpeg) ErrOutput() ([]byte, error) {
	_, err := ff.Run()
	if err != nil {
		return nil, err
	}
	defer ff.Close()

	_, _ = ff.Stderr.Seek(0, io.SeekStart)
	return io.ReadAll(ff.Stderr)
}

func (ff *ffmpeg) Run() (*MemoryBuffer, error) {
	// try and start the ffmpeg instance
	if err := ff.Cmd.Start(); err != nil {
		ff.Close() // close everything if we fail
		return nil, err
	}

	// wait for ffmpeg to finish
	if err := ff.Cmd.Wait(); err != nil {
		// we need to read Stderr through ReadError before
		// we close it so defer the call
		defer ff.Close()
		return nil, fmt.Errorf("%w: %w", err, ff.ReadError())
	}

	ff.Stdout.CloseWrite()
	return ff.Stdout, nil
}
