package audio

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
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
			log.Panic().Str("key", key).Msg("unknown key")
		}
	}
	if err := s.Err(); err != nil {
		return nil, errors.E(op, err)
	}

	return &info, nil
}

func Probe(ctx context.Context, filename string) (*ProbeInfo, error) {
	const op errors.Op = "streamer/audio.Probe"

	cmd := exec.CommandContext(ctx, "ffprobe",
		"-loglevel", "fatal",
		"-show_error", "-show_format", "-show_streams", // probe for the format and data streams
		"-select_streams", "a", // only probe for audio streams
		"-of", "json=c=1", // json output
		"-i", filename)

	out, err := cmd.Output()
	if err != nil {
		return nil, errors.E(op, err, errors.Info(cmd.String()))
	}

	var info ProbeInfo

	err = json.Unmarshal(out, &info)
	if err != nil {
		return nil, errors.E(op, err)
	}

	if info.Format.Tags == nil && len(info.Streams) > 0 {
		info.Format.Tags = info.Streams[0].Tags
	}

	return &info, nil
}

type ProbeInfo struct {
	Format  *FormatInfo   `json:"format,omitempty"`
	Streams []StreamsInfo `json:"streams,omitempty"`
}

type FormatInfo struct {
	BitRate        string          `json:"bit_rate"`
	Duration       string          `json:"duration"`
	Filename       string          `json:"filename"`
	FormatLongName string          `json:"format_long_name"`
	FormatName     string          `json:"format_name"`
	NbStreams      int             `json:"nb_streams"`
	Size           string          `json:"size"`
	StartTime      string          `json:"start_time"`
	Tags           *FormatTagsInfo `json:"tags"`
}

type FormatTagsInfo struct {
	Anime         string `json:"ANIME"`
	Compilation   string `json:"COMPILATION,omitempty"`
	Originaldate  string `json:"ORIGINALDATE,omitempty"`
	R128AlbumGain string `json:"R128_ALBUM_GAIN,omitempty"`
	R128TrackGain string `json:"R128_TRACK_GAIN,omitempty"`
	Tbpm          string `json:"TBPM,omitempty"`
	Year          string `json:"YEAR,omitempty"`
	Album         string `json:"album"`
	AlbumArtist   string `json:"album_artist,omitempty"`
	Artist        string `json:"artist"`
	Date          string `json:"date,omitempty"`
	Disc          string `json:"disc,omitempty"`
	Title         string `json:"title"`
	Track         string `json:"track,omitempty"`
}

type StreamsInfo struct {
	AvgFrameRate       string          `json:"avg_frame_rate"`
	BitRate            string          `json:"bit_rate,omitempty"`
	BitsPerSample      int             `json:"bits_per_sample,omitempty"`
	Channels           int             `json:"channels,omitempty"`
	CodecLongName      string          `json:"codec_long_name"`
	CodecName          string          `json:"codec_name"`
	CodecTag           string          `json:"codec_tag"`
	CodecTagString     string          `json:"codec_tag_string"`
	CodecTimeBase      string          `json:"codec_time_base"`
	CodecType          string          `json:"codec_type"`
	DisplayAspectRatio string          `json:"display_aspect_ratio,omitempty"`
	Duration           string          `json:"duration"`
	DurationTs         int             `json:"duration_ts"`
	HasBFrames         int             `json:"has_b_frames,omitempty"`
	Height             int             `json:"height,omitempty"`
	Index              int             `json:"index"`
	Level              int             `json:"level,omitempty"`
	PixFmt             string          `json:"pix_fmt,omitempty"`
	RFrameRate         string          `json:"r_frame_rate"`
	SampleAspectRatio  string          `json:"sample_aspect_ratio,omitempty"`
	SampleFmt          string          `json:"sample_fmt,omitempty"`
	SampleRate         string          `json:"sample_rate,omitempty"`
	StartPts           int             `json:"start_pts"`
	StartTime          string          `json:"start_time"`
	Tags               *FormatTagsInfo `json:"tags,omitempty"`
	TimeBase           string          `json:"time_base"`
	Width              int             `json:"width,omitempty"`
}
