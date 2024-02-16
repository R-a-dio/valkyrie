package audio

import (
	"context"
	"fmt"
	"os"
	"os/exec"
)

func Spectrum(ctx context.Context, filename string) (string, error) {
	f, err := os.CreateTemp("", "spectrum*.jpg")
	if err != nil {
		return "", err
	}
	f.Close()

	cmd := exec.CommandContext(ctx, "ffmpeg", "-nostdin",
		"-y", "-v", "error", "-hide_banner",
		"-i", filename,
		"-filter_complex", "[0:a:0]aresample=48000:resampler=soxr,showspectrumpic=s=640x512,crop=780:544:70:50[o]",
		"-map", "[o]", "-frames:v", "1", "-q:v", "3", f.Name(),
	)

	out, err := cmd.Output()
	if err != nil {
		fmt.Println(string(out))
		return "", err
	}

	return f.Name(), nil
}
