package manager

import (
	"context"
	"database/sql"
	"log"
	"net"
	"sync"
	"time"

	radio "github.com/R-a-dio/valkyrie"
	"github.com/R-a-dio/valkyrie/config"
	"github.com/R-a-dio/valkyrie/database"
	"github.com/jmoiron/sqlx"
)

// Execute executes a manager with the context and configuration given; it returns with
// any error that occurs; Execution can be interrupted by canceling the context given.
func Execute(ctx context.Context, cfg config.Config) error {
	m, err := NewManager(ctx, cfg)
	if err != nil {
		return err
	}

	ExecuteListener(ctx, cfg, m)

	// setup a http server for our RPC API
	srv, err := NewHTTPServer(m)
	if err != nil {
		return err
	}

	ln, err := net.Listen("tcp", srv.Addr)
	if err != nil {
		return err
	}

	errCh := make(chan error, 1)
	go func() {
		errCh <- srv.Serve(ln)
	}()

	// wait for our context to be canceled or Serve to error out
	select {
	case <-ctx.Done():
		return srv.Close()
	case err = <-errCh:
		return err
	}
}

// ExecuteListener is an alias for NewListener
var ExecuteListener = NewListener

// NewManager returns a manager ready for use
func NewManager(ctx context.Context, cfg config.Config) (*Manager, error) {
	db, err := database.Connect(cfg)
	if err != nil {
		return nil, err
	}

	m := Manager{
		Config: cfg,
		DB:     db,
		status: radio.Status{},
	}

	old, err := m.loadStreamStatus(ctx)
	if err != nil {
		return nil, err
	}
	m.status = *old

	m.client.announce = cfg.Conf().IRC.Client()
	m.client.streamer = cfg.Conf().Streamer.Client()
	return &m, nil
}

// Manager manages shared state between different processes
type Manager struct {
	config.Config
	DB *sqlx.DB

	// Other components
	client struct {
		announce radio.AnnounceService
		streamer radio.StreamerService
	}
	// mu protects the fields below and their contents
	mu     sync.Mutex
	status radio.Status
	// listener count at the start of a song
	songStartListenerCount int
}

// updateStreamStatus is a legacy layer to keep supporting streamstatus table usage
// in the website.
func (m *Manager) updateStreamStatus() {
	go func() {
		m.mu.Lock()
		status := m.status.Copy()
		m.mu.Unlock()

		ctx, cancel := context.WithTimeout(context.Background(), time.Second*2)
		defer cancel()

		h := database.Handle(ctx, m.DB)

		// we either want to do an UPDATE or an INSERT if our row doesn't exist
		var queries = []string{`
		UPDATE
			streamstatus
		SET
			djid=:user.dj.id,
			np=:song.metadata,
			listeners=:listeners,
			bitrate=192000,
			isafkstream=0,
			isstreamdesk=0,
			start_time=UNIX_TIMESTAMP(:songinfo.start),
			end_time=UNIX_TIMESTAMP(:songinfo.end),
			trackid=:song.trackid,
			thread=:thread,
			requesting=:requestsenabled,
			djname=:streamername,
			lastset=NOW()
		WHERE
			id=0;
		`, `
		INSERT INTO
			streamstatus
		(
			id,
			djid,
			np,
			listeners,
			isafkstream,
			start_time,
			end_time,
			trackid,
			thread,
			requesting,
			djname
		) VALUES (
			0,
			:user.dj.id,
			:song.metadata,
			:listeners,
			0,
			UNIX_TIMESTAMP(:songinfo.start),
			UNIX_TIMESTAMP(:songinfo.end),
			:song.trackid,
			:thread,
			:requestsenabled,
			:streamername
		);
		`}

		// do some minor adjustments so that we can safely pass the status object
		// straight to the Exec
		if !status.Song.HasTrack() {
			status.Song.DatabaseTrack = &radio.DatabaseTrack{}
		}
		// streamstatus expects an end equal to start if it's unknown
		if status.SongInfo.End.IsZero() {
			status.SongInfo.End = status.SongInfo.Start
		}

		// now try the UPDATE and if that fails do the same with an INSERT
		for _, query := range queries {
			query, args, err := sqlx.Named(query, status)
			if err != nil {
				log.Printf("manager: error: %v", err)
				return
			}

			res, err := h.Exec(query, args...)
			if err != nil {
				log.Printf("manager: error: %v", err)
				return
			}

			// check if we've successfully updated, otherwise we need to do an insert
			if i, err := res.RowsAffected(); err != nil || i > 0 {
				return
			}
		}
	}()
}

// loadStreamStatus is to load the legacy streamstatus table, we should only do this
// at startup
func (m *Manager) loadStreamStatus(ctx context.Context) (*radio.Status, error) {
	h := database.Handle(ctx, m.DB)

	var query = `
	SELECT
		djid AS 'user.dj.id',
		np AS 'song.metadata',
		listeners,
		from_unixtime(start_time) AS 'songinfo.start',
		from_unixtime(end_time) AS 'songinfo.end',
		trackid AS 'song.trackid',
		thread,
		requesting AS requestsenabled,
		djname AS 'streamername'
	FROM
		streamstatus
	WHERE
		id=0
	LIMIT 1;
	`
	var status radio.Status

	err := sqlx.Get(h, &status, query)
	if err != nil && err != sql.ErrNoRows {
		return nil, err
	}

	// see if we can get a full song from the database
	if status.Song.Metadata != "" {
		song, err := database.GetSongFromMetadata(h, status.Song.Metadata)
		if err == nil {
			status.Song = *song
		}
	}

	return &status, nil
}
