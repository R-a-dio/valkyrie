package audio

import (
	"context"
	"io"
	"os/exec"
	"path/filepath"
	"strings"

	radio "github.com/R-a-dio/valkyrie"
	"github.com/R-a-dio/valkyrie/errors"
	"github.com/spf13/afero"
	"go.opentelemetry.io/otel"
)

// DeleteID3Tags runs `id3v2 --delete-all <filename>` this removes any id3 tags
// in the file.
func DeleteID3Tags(ctx context.Context, filename string) {
	exec.CommandContext(ctx, "id3v2", "--delete-all", filename)
}

// WithMetadata puts the metadata of song into the file given by filename and returns
// it as an in-memory file.
func WriteMetadata(ctx context.Context, f afero.File, song radio.Song) (*MemoryBuffer, error) {
	const op errors.Op = "streamer/audio.WriteMetadata"
	ctx, span := otel.Tracer("").Start(ctx, string(op))
	defer span.End()

	args := []string{
		"-hide_banner",
		"-loglevel", "error",
		"-i", "-",
		"-c:a", "copy", // copy audio stream
		"-id3v2_version", "3", // windows apparently doesn't support v2.4
	}
	if song.Title != "" {
		args = append(args, "-metadata", "title="+song.Title)
	}
	if song.Artist != "" {
		args = append(args, "-metadata", "artist="+song.Artist)
	}
	if song.Album != "" {
		args = append(args, "-metadata", "album="+song.Album)
	}
	if song.Tags != "" {
		args = append(args, "-metadata", "comment="+song.Tags)
	}

	switch strings.ToLower(filepath.Ext(f.Name())) {
	case ".flac":
		args = append(args, "-f", "flac")
	case ".mp3":
		args = append(args, "-f", "mp3")
	case ".ogg":
		args = append(args, "-f", "ogg")
	default:
		return nil, errors.E(op, errors.InvalidArgument)
	}
	//args = append(args, "-")

	ff, err := newFFmpegCmdFile(ctx, "metadata-"+song.TrackID.String(), args)
	if err != nil {
		return nil, errors.E(op, err)
	}
	ff.Cmd.Stdin = f

	out, err := ff.Run(ctx)
	if err != nil {
		return nil, errors.E(op, err)
	}
	// seek the output to the start
	_, _ = out.Seek(0, io.SeekStart)
	return out, nil
}
