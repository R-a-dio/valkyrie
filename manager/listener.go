package manager

import (
	"bufio"
	"context"
	"errors"
	"html"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	radio "github.com/R-a-dio/valkyrie"
	"github.com/R-a-dio/valkyrie/config"
	"github.com/rs/zerolog"
)

const maxMetadataLength = 255 * 16

// Listener listens to an icecast mp3 stream with interleaved song metadata
type Listener struct {
	config.Config
	// done is closed when run exits, and indicates this listener instance stopped running
	done chan struct{}
	// cancel is called when Shutdown is called and cancels all operations started by run
	cancel context.CancelFunc

	// manager is an RPC client to the status manager
	manager radio.ManagerService

	// prevSong is the last song we got from the stream
	prevSong string
}

// NewListener creates a listener and starts running in the background immediately
func NewListener(ctx context.Context, cfg config.Config, m radio.ManagerService) *Listener {
	ln := Listener{
		Config:  cfg,
		manager: m,
		done:    make(chan struct{}),
	}

	ctx, ln.cancel = context.WithCancel(ctx)
	go func() {
		defer ln.cancel()
		defer close(ln.done)
		ln.run(ctx)
	}()

	return &ln
}

// Shutdown signals the listener to stop running, and waits for it to exit
func (ln *Listener) Shutdown() error {
	ln.cancel()
	<-ln.done
	return nil
}

func (ln *Listener) run(ctx context.Context) {
	logger := zerolog.Ctx(ctx)
	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		conn, metasize, err := ln.newConn(ctx)
		if err != nil {
			logger.Error().Err(err).Msg("connecting")
			// wait a bit before retrying the connection
			select {
			case <-ctx.Done():
			case <-time.After(time.Second * 2):
			}

			continue
		}

		err = ln.parseResponse(ctx, metasize, conn)
		if err != nil {
			// log the error, and try reconnecting
			logger.Error().Err(err).Msg("connection")
		}
	}
}

func (ln *Listener) newConn(ctx context.Context) (io.ReadCloser, int, error) {
	uri := ln.Conf().Manager.StreamURL

	req, err := http.NewRequest("GET", uri, nil)
	if err != nil {
		return nil, 0, err
	}
	// we don't want to re-use connections for the audio stream
	req.Close = true
	// we want interleaved metadata so we have to ask for it
	req.Header.Add("Icy-MetaData", "1")
	req = req.WithContext(ctx)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, 0, err
	}

	// special case for when a fallback isn't setup in icecast; this gives us a 404 status
	// code on the mountpoint configured so try and see if we can wake up the streamer by
	// sending a fake fallback to the manager
	if resp.StatusCode == http.StatusNotFound {
		fallbacks := ln.Conf().Manager.FallbackNames
		var fallback = "fallback"
		if len(fallbacks) > 0 {
			fallback = fallbacks[0]
		}

		ln.manager.UpdateSong(ctx, &radio.SongUpdate{
			Song: radio.Song{Metadata: fallback},
			Info: radio.SongInfo{
				Start:      time.Now(),
				IsFallback: true,
			},
		})
	}

	if resp.StatusCode != 200 {
		resp.Body.Close()
		return nil, 0, errors.New("listener: request error: " + resp.Status)
	}

	metasize, err := strconv.Atoi(resp.Header.Get("icy-metaint"))
	if err != nil {
		resp.Body.Close()
		return nil, 0, err
	}

	return resp.Body, metasize, nil
}

func (ln *Listener) parseResponse(ctx context.Context, metasize int, src io.Reader) error {
	r := bufio.NewReader(src)
	logger := zerolog.Ctx(ctx)

	var meta map[string]string
	var buf = make([]byte, metasize)
	if metasize <= maxMetadataLength {
		// we allocate one extra byte to support semicolon insertion in
		// metadata parsing
		buf = make([]byte, maxMetadataLength+1)
	}

	for {
		// we first get actual mp3 data from icecast
		_, err := io.ReadFull(r, buf[:metasize])
		if err != nil {
			return err
		}

		// then we get a single byte indicating metadata length
		b, err := r.ReadByte()
		if err != nil {
			return err
		}

		// if the length is set to 0 we're not expecting any metadata and can
		// read data again
		if b == 0 {
			continue
		}

		// else metadata length needs to be multiplied by 16 from the wire
		length := int(b * 16)
		_, err = io.ReadFull(r, buf[:length])
		if err != nil {
			return err
		}

		// now parse the metadata
		meta = parseMetadata(buf[:length])

		if len(meta) == 0 {
			// fatal because it most likely means we've lost sync with the data
			// stream and can't find our metadata anymore.
			return errors.New("listener: empty metadata: " + string(buf[:length]))
		}

		song := meta["StreamTitle"]
		if song == "" {
			logger.Info().Msg("empty metadata")
			continue
		}

		if song == ln.prevSong {
			logger.Info().Str("metadata", song).Msg("same metadata")
			continue
		}

		s := radio.Song{
			Metadata: strings.TrimSpace(song),
		}
		info := radio.SongInfo{
			Start:      time.Now(),
			IsFallback: ln.isFallback(song),
		}

		// set the previous song metadata only if we're not on a fallback to avoid
		// stream -> drop (onto fallback) -> stream patterns announcing multiple times
		if !info.IsFallback {
			ln.prevSong = song
		}

		go func() {
			ctx, cancel := context.WithTimeout(ctx, time.Second*5)
			defer cancel()

			update := &radio.SongUpdate{Song: s, Info: info}
			err := ln.manager.UpdateSong(ctx, update)
			if err != nil {
				logger.Error().Err(err).Msg("updating stream song")
			}
		}()
	}
}

// isFallback checks if the meta passed in matches one of the known fallback
// mountpoint meta as defined with `fallbacknames` in configuration file
func (ln *Listener) isFallback(meta string) bool {
	for _, fallback := range ln.Conf().Manager.FallbackNames {
		if fallback == meta {
			return true
		}
	}
	return false
}

func parseMetadata(b []byte) map[string]string {
	var meta = make(map[string]string, 2)

	// trim any padding nul bytes and insert a trailing semicolon if one
	// doesn't exist yet
	for i := len(b) - 1; i > 0; i-- {
		if b[i] == '\x00' {
			continue
		}

		if b[i] == ';' {
			// already have a trailing semicolon
			b = b[:i+1]
			break
		}

		// don't have one, so add one
		b = append(b[:i+1], ';')
		break
	}

	for {
		var key, value string

		b, key = findSequence(b, '=', '\'')
		b, value = findSequence(b, '\'', ';')

		if key == "" {
			break
		}

		// try and do any html escaping, icecast default configuration will send unicode chars
		// as html escaped characters
		value = html.UnescapeString(value)
		// replace any broken utf8, since other layers expect valid utf8 we do it at the edge
		value = strings.ToValidUTF8(value, string(utf8.RuneError))

		meta[key] = value
	}

	return meta
}

func findSequence(seq []byte, a, b byte) ([]byte, string) {
	for i := 1; i < len(seq); i++ {
		if seq[i-1] == a && seq[i] == b {
			return seq[i+1:], string(seq[:i-1])
		}
	}

	return nil, ""
}
