package audio

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
	"time"
)

// ProbeDuration attempts to call ffprobe on the file given and returns
// the duration as returned by it. Requires ffprobe findable in the PATH.
func ProbeDuration(ctx context.Context, filename string) (time.Duration, error) {
	cmd := exec.CommandContext(ctx, "ffprobe",
		"-loglevel", "error",
		"-show_entries", "format=duration",
		"-of", "default=noprint_wrappers=1:nokey=1",
		filename,
	)

	durBytes, err := cmd.Output()
	if err != nil {
		fmt.Println(err)
		return 0, err
	}

	durString := strings.TrimSpace(string(durBytes))
	dur, err := time.ParseDuration(durString + "s")
	if err != nil {
		fmt.Println(err)
		return 0, err
	}

	return dur, nil
}
