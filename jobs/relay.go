package jobs

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	radio "github.com/R-a-dio/valkyrie"
	"github.com/R-a-dio/valkyrie/config"
	"github.com/R-a-dio/valkyrie/storage"
	"github.com/R-a-dio/valkyrie/streamer/icecast"
	"github.com/cenkalti/backoff"
	"github.com/rs/zerolog"
)

const maxMetadataLength = 255 * 16

func ExecuteRelay(ctx context.Context, cfg config.Config) error {
	ss, err := storage.Open(ctx, cfg)
	if err != nil {
		return err
	}

	scfg := cfg.Conf().Streamer
	endpoint := scfg.StreamURL.URL()
	if scfg.StreamUsername != "" {
		endpoint.User = url.UserPassword(scfg.StreamUsername, scfg.StreamPassword)
	}

	var conn net.Conn

	// setup a goroutine that reads data from icecast
	ln, _ := Listen(ctx, os.Getenv("RELAY_STREAM_ENDPOINT"), func(ctx context.Context, data []byte) error {
		var err error
		if conn == nil {
			conn, err = newConn(ctx, endpoint)
			if err != nil {
				return err
			}
		}

		_, err = conn.Write(data)
		return err
	})

	var lastThread radio.Thread
	for {
		select {
		case <-ctx.Done():
			return nil
		case meta := <-ln.metadataCh:
			icecast.MetadataURL(endpoint,
				icecast.UserAgent(cfg.Conf().UserAgent),
			)(ctx, meta)

			// we also check the api everytime metadata changes
			info, err := getAPI(ctx, ss.User(ctx), ss.Song(ctx))
			if err != nil {
				// do nothing if this failed
				zerolog.Ctx(ctx).Err(err).Msg("failed to retrieve user from api")
				continue
			}

			// just let the manager handle duplicate user updates, it's more important
			// we're actually correct at some point
			err = cfg.Manager.UpdateUser(ctx, info.User)
			if err != nil {
				zerolog.Ctx(ctx).Err(err).Msg("failed to update user")
			}

			if info.Thread != lastThread {
				lastThread = info.Thread
				cfg.Manager.UpdateThread(ctx, info.Thread)
			}
		}
	}
}

type apiResponse struct {
	Main struct {
		DJ struct {
			ID int `json:"id"`
		} `json:"dj"`
		Thread string `json:"thread"`
		Queue  []queueEntry
	} `json:"main"`
}

type queueEntry struct {
	Metadata  string `json:"meta" db:"meta"`
	Time      string `json:"time"`
	Type      int    `json:"type" db:"type"`
	Timestamp int64  `json:"timestamp" db:"time"`
}

type Info struct {
	User   *radio.User
	Thread radio.Thread
	Queue  radio.Queue
}

func getAPI(ctx context.Context, us radio.UserStorage, ss radio.SongStorage) (*Info, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, os.Getenv("RELAY_API_ENDPOINT"), nil)
	if err != nil {
		return nil, err
	}
	req.Host = "r-a-d.io"

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var res apiResponse

	err = json.NewDecoder(resp.Body).Decode(&res)
	if err != nil {
		return nil, err
	}

	user, err := us.GetByDJID(radio.DJID(res.Main.DJ.ID))
	if err != nil {
		return nil, err
	}

	var queue radio.Queue
	/* turns out we can't actually overwrite the queue easily
	for _, qe := range res.Main.Queue {
		song, err := ss.FromMetadata(qe.Metadata)
		if err != nil {
			// skip if this fails
			zerolog.Ctx(ctx).Err(err).Ctx(ctx).Msg("failed to retrieve song from metadata")
			continue
		}

		queue = append(queue, radio.QueueEntry{
			QueueID:           radio.NewQueueID(),
			Song:              *song,
			IsUserRequest:     qe.Type == 1,
			ExpectedStartTime: time.Unix(qe.Timestamp, 0),
		})
	}
	*/

	return &Info{
		User:   user,
		Thread: radio.Thread(res.Main.Thread),
		Queue:  queue,
	}, nil
}

func newConn(ctx context.Context, uri *url.URL) (net.Conn, error) {
	bo := config.NewConnectionBackoff(ctx)
	var conn net.Conn
	var err error

	err = backoff.RetryNotify(func() error {
		conn, err = icecast.DialURL(ctx, uri,
			icecast.ContentType("audio/mpeg"),
			icecast.UserAgent("hanyuu/relay"),
		)
		if err != nil {
			return err
		}

		zerolog.Ctx(ctx).Info().
			Str("endpoint", uri.Redacted()).
			Msg("connected to icecast")
		return nil
	}, bo, func(err error, d time.Duration) {
		zerolog.Ctx(ctx).Error().
			Err(err).
			Dur("backoff", d).
			Str("endpoint", uri.Redacted()).
			Msg("icecast connection failure")
	})
	if err != nil {
		return nil, err
	}

	return conn, nil
}

// listener listens to an icecast mp3 stream with interleaved song metadata
type listener struct {
	// cancel is called when Close is called and cancels all in-progress reads
	cancel context.CancelFunc
	done   chan struct{}

	handleData func(ctx context.Context, data []byte) error
	metadataCh chan string
}

func Listen(ctx context.Context, u string, dataFn func(ctx context.Context, data []byte) error) (*listener, error) {
	uri, err := url.Parse(u)
	if err != nil {
		return nil, fmt.Errorf("Listen: failed to parse url: %w", err)
	}

	return ListenURL(ctx, uri, dataFn), nil
}

func ListenURL(ctx context.Context, u *url.URL, dataFn func(ctx context.Context, data []byte) error) *listener {
	ln := listener{
		metadataCh: make(chan string),
		done:       make(chan struct{}),
		handleData: dataFn,
	}
	ctx, ln.cancel = context.WithCancel(ctx)

	go func() {
		defer ln.cancel()
		defer close(ln.done)
		ln.run(ctx, u)
	}()

	return &ln
}

// Shutdown signals the listener to stop running, and waits for it to exit
func (ln *listener) Close() error {
	ln.cancel()
	<-ln.done
	return nil
}

func (ln *listener) run(ctx context.Context, u *url.URL) {
	logger := zerolog.Ctx(ctx)
	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		conn, metasize, err := ln.newConn(ctx, u)
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

func (ln *listener) newConn(ctx context.Context, u *url.URL) (io.ReadCloser, int, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, 0, err
	}
	req.Host = "r-a-d.io"
	// we don't want to re-use connections for the audio stream
	req.Close = true
	// we want interleaved metadata so we have to ask for it
	req.Header.Add("Icy-MetaData", "1")
	req.Header.Set("User-Agent", "hanyuu/relay")

	// TODO: don't use the default client
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to do request: %w", err)
	}

	if resp.StatusCode != 200 {
		resp.Body.Close()
		return nil, 0, fmt.Errorf("status code is not OK was %d: %s", resp.StatusCode, resp.Status)
	}

	// convert the metadata size we got back from the server
	metasize, err := strconv.Atoi(resp.Header.Get("icy-metaint"))
	if err != nil {
		resp.Body.Close()
		return nil, 0, fmt.Errorf("icy-metaint is not an integer: %w", err)
	}

	return resp.Body, metasize, nil
}

func (ln *listener) parseResponse(ctx context.Context, metasize int, src io.Reader) error {
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

		if ln.handleData != nil {
			err = ln.handleData(ctx, buf[:metasize])
			if err != nil {
				logger.Err(err).Msg("failed handling mp3 data")
			}
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

		go func() {
			select {
			case ln.metadataCh <- song:
			case <-ctx.Done():
			}
		}()
	}
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
