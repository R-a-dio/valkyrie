package audio

import (
	"bufio"
	"bytes"
	"context"
	"io"
	"os"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"time"

	radio "github.com/R-a-dio/valkyrie"
	"github.com/R-a-dio/valkyrie/config"
	"github.com/R-a-dio/valkyrie/errors"
	"github.com/R-a-dio/valkyrie/util"
	"github.com/rs/zerolog"
)

type Prober func(ctx context.Context, song radio.Song) (time.Duration, error)

func NewProber(cfg config.Config, timeout time.Duration) Prober {
	cfgMusicPath := config.Value(cfg, func(cfg config.Config) string {
		return cfg.Conf().MusicPath
	})

	return func(ctx context.Context, song radio.Song) (time.Duration, error) {
		ctx, cancel := context.WithTimeout(ctx, timeout)
		defer cancel()

		path := util.AbsolutePath(cfgMusicPath(), song.FilePath)
		return ProbeDuration(ctx, path)
	}
}

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
	Comment    string
	Bitrate    int
}

var ffprobeTextArgs = []string{
	"-loglevel", "fatal",
	"-hide_banner",
	"-show_entries", "format_tags=title,artist,album,comment:stream_tags=title,artist,album,comment:stream=duration,bit_rate:format=format_name,bit_rate",
	"-of", "default=noprint_wrappers=1",
}

func ProbeText(ctx context.Context, filename string) (*Info, error) {
	const op errors.Op = "streamer/audio.Probe"

	cmd := exec.CommandContext(ctx, "ffprobe",
		append(ffprobeTextArgs, "-i", filename)...)

	out, err := cmd.Output()
	if err != nil {
		return nil, errors.E(op, err, errors.Info(cmd.String()))
	}

	return parseProbeText(ctx, bytes.NewReader(out))
}

func probeText(ctx context.Context, file *os.File) (*Info, error) {
	const op errors.Op = "streamer/audio.Probe"

	cmd := exec.CommandContext(ctx, "ffprobe",
		append(ffprobeTextArgs, "-i", "-")...)
	cmd.Stdin = file

	out, err := cmd.Output()
	if err != nil {
		return nil, errors.E(op, err, errors.Info(cmd.String()))
	}

	return parseProbeText(ctx, bytes.NewReader(out))
}

func parseProbeText(ctx context.Context, out io.Reader) (*Info, error) {
	const op errors.Op = "streamer/audio.parseProbeText"
	var err error
	var info Info

	s := bufio.NewScanner(out)
	log := zerolog.Ctx(ctx)
	for s.Scan() {
		m := probeRegex.FindStringSubmatch(s.Text())
		if m == nil {
			// invalid output
			log.Error().Ctx(ctx).Str("line", s.Text()).Msg("invalid line")
			continue
		}

		key := strings.ToLower(m[probeKey])
		value := m[probeValue]

		switch key {
		case "duration":
			info.Duration, err = time.ParseDuration(value + "s")
			if err != nil {
				log.Error().Ctx(ctx).Err(err).Str("line", s.Text()).Str("value", value).Msg("invalid duration")
			}
		case "title":
			info.Title = value
		case "artist":
			info.Artist = value
		case "album":
			info.Album = value
		case "comment":
			info.Comment = value
		case "bit_rate":
			if value != "N/A" { // could not exist
				info.Bitrate, err = strconv.Atoi(value)
				if err != nil {
					log.Error().Ctx(ctx).Err(err).Str("line", s.Text()).Str("value", value).Msg("invalid bit_rate")
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
