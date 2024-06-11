package audio

import (
	"bufio"
	"bytes"
	"context"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/R-a-dio/valkyrie/errors"
	"github.com/rs/zerolog"
)

// ProbeDuration attempts to call ffprobe on the file given and returns
// the duration as returned by it. Requires ffprobe findable in the PATH.
func ProbeDuration(ctx context.Context, filename string) (time.Duration, error) {
	const op errors.Op = "streamer/audio.ProbeDuration"

	cmd := exec.CommandContext(ctx, "ffprobe",
		"-loglevel", "error",
		"-show_entries", "format=duration",
		"-of", "default=noprint_wrappers=1:nokey=1",
		filename,
	)

	durBytes, err := cmd.Output()
	if err != nil {
		return 0, errors.E(op, err)
	}

	durString := strings.TrimSpace(string(durBytes))
	dur, err := time.ParseDuration(durString + "s")
	if err != nil {
		return 0, errors.E(op, err)
	}

	return dur, nil
}

var (
	probeRegex = regexp.MustCompile(`(TAG:)?(?P<key>.+?)=(?P<value>.+)`)
	probeKey   = probeRegex.SubexpIndex("key")
	probeValue = probeRegex.SubexpIndex("value")
)

type Info struct {
	Duration   time.Duration
	FormatName string
	Title      string
	Artist     string
	Album      string
	Bitrate    int
}

func ProbeText(ctx context.Context, filename string) (*Info, error) {
	const op errors.Op = "streamer/audio.Probe"

	log := zerolog.Ctx(ctx)

	cmd := exec.CommandContext(ctx, "ffprobe",
		"-loglevel", "fatal",
		"-hide_banner",
		"-show_entries", "format_tags=title,artist,album:stream_tags=title,artist,album:stream=duration,bit_rate:format=format_name,bit_rate",
		"-of", "default=noprint_wrappers=1",
		"-i", filename)

	out, err := cmd.Output()
	if err != nil {
		return nil, errors.E(op, err, errors.Info(cmd.String()))
	}

	var info Info
	s := bufio.NewScanner(bytes.NewReader(out))
	for s.Scan() {
		m := probeRegex.FindStringSubmatch(s.Text())
		if m == nil {
			// invalid output
			log.Error().Str("line", s.Text()).Msg("invalid line")
			continue
		}

		key := strings.ToLower(m[probeKey])
		value := m[probeValue]

		switch key {
		case "duration":
			info.Duration, err = time.ParseDuration(value + "s")
			if err != nil {
				log.Error().Err(err).Str("line", s.Text()).Str("value", value).Msg("invalid duration")
			}
		case "title":
			info.Title = value
		case "artist":
			info.Artist = value
		case "album":
			info.Album = value
		case "bit_rate":
			if value != "N/A" { // could not exist
				info.Bitrate, err = strconv.Atoi(value)
				if err != nil {
					log.Error().Err(err).Str("line", s.Text()).Str("value", value).Msg("invalid bit_rate")
				}
			}
		case "format_name":
			info.FormatName = value
		default:
			log.WithLevel(zerolog.PanicLevel).Str("key", key).Msg("unknown key")
			panic("unknown key")
		}
	}
	if err := s.Err(); err != nil {
		return nil, errors.E(op, err)
	}

	return &info, nil
}
