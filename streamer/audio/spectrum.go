package audio

import (
	"context"
	"io"
	"os"
	"os/exec"

	"github.com/R-a-dio/valkyrie/errors"
	"github.com/justincormack/go-memfd"
)

func Spectrum(ctx context.Context, filename string) (*os.File, error) {
	const op errors.Op = "streamer/audio.Spectrum"

	f, err := memfd.CreateNameFlags("spectrum", memfd.AllowSealing|memfd.Cloexec)
	if err != nil {
		return nil, errors.E(op, err)
	}

	cmd := exec.CommandContext(ctx, "ffmpeg", "-nostdin",
		"-y", "-v", "error", "-hide_banner",
		"-i", filename,
		"-filter_complex", "[0:a:0]aresample=48000:resampler=soxr,showspectrumpic=s=640x512,crop=780:544:70:50[o]",
		"-map", "[o]", "-frames:v", "1", "-q:v", "3", "-f", "webp", "-",
	)
	cmd.Stdout = f.File

	if err = cmd.Start(); err != nil {
		f.Close()
		return nil, errors.E(op, err)
	}

	if err = cmd.Wait(); err != nil {
		f.Close()
		return nil, errors.E(op, err)
	}

	if _, err = f.Seek(0, io.SeekStart); err != nil {
		f.Close()
		return nil, errors.E(op, err)
	}

	return f.File, nil
}
