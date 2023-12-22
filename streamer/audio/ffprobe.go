package audio

import (
	"context"
	"encoding/json"
	"os/exec"
	"strings"
	"time"

	"github.com/R-a-dio/valkyrie/errors"
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

	return &info, nil
}

type ProbeInfo struct {
	Format  *FormatInfo   `json:"format,omitempty"`
	Streams []StreamsInfo `json:"streams,omitempty"`
}

type FormatInfo struct {
	BitRate        string         `json:"bit_rate"`
	Duration       string         `json:"duration"`
	Filename       string         `json:"filename"`
	FormatLongName string         `json:"format_long_name"`
	FormatName     string         `json:"format_name"`
	NbStreams      int            `json:"nb_streams"`
	Size           string         `json:"size"`
	StartTime      string         `json:"start_time"`
	Tags           FormatTagsInfo `json:"tags"`
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
	AvgFrameRate       string           `json:"avg_frame_rate"`
	BitRate            string           `json:"bit_rate,omitempty"`
	BitsPerSample      int              `json:"bits_per_sample,omitempty"`
	Channels           int              `json:"channels,omitempty"`
	CodecLongName      string           `json:"codec_long_name"`
	CodecName          string           `json:"codec_name"`
	CodecTag           string           `json:"codec_tag"`
	CodecTagString     string           `json:"codec_tag_string"`
	CodecTimeBase      string           `json:"codec_time_base"`
	CodecType          string           `json:"codec_type"`
	DisplayAspectRatio string           `json:"display_aspect_ratio,omitempty"`
	Duration           string           `json:"duration"`
	DurationTs         int              `json:"duration_ts"`
	HasBFrames         int              `json:"has_b_frames,omitempty"`
	Height             int              `json:"height,omitempty"`
	Index              int              `json:"index"`
	Level              int              `json:"level,omitempty"`
	PixFmt             string           `json:"pix_fmt,omitempty"`
	RFrameRate         string           `json:"r_frame_rate"`
	SampleAspectRatio  string           `json:"sample_aspect_ratio,omitempty"`
	SampleFmt          string           `json:"sample_fmt,omitempty"`
	SampleRate         string           `json:"sample_rate,omitempty"`
	StartPts           int              `json:"start_pts"`
	StartTime          string           `json:"start_time"`
	Tags               *StreamsTagsInfo `json:"tags,omitempty"`
	TimeBase           string           `json:"time_base"`
	Width              int              `json:"width,omitempty"`
}

type StreamsTagsInfo struct {
	Comment string `json:"comment"`
	Title   string `json:"title"`
}
