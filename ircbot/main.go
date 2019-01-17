package ircbot

import (
	"context"
	"log"
	"net"
	"time"

	radio "github.com/R-a-dio/valkyrie"
	"github.com/R-a-dio/valkyrie/config"
	"github.com/R-a-dio/valkyrie/database"
	"github.com/R-a-dio/valkyrie/rpc/streamer"
	"github.com/jmoiron/sqlx"
	"github.com/lrstanley/girc"
)

// Execute executes the ircbot with the context and configuration given. it returns with
// any error that occurs; Execution can be interrupted by canceling the context given.
func Execute(ctx context.Context, cfg config.Config) error {
	b, err := NewBot(cfg)

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	// setup a http server for our RPC API
	srv, err := NewHTTPServer(b)
	if err != nil {
		return err
	}

	ln, err := net.Listen("tcp", srv.Addr)
	if err != nil {
		return err
	}

	errCh := make(chan error, 2)
	go func() {
		errCh <- b.runClient(ctx)
	}()
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

// NewBot returns a Bot with configuration and handlers loaded
func NewBot(cfg config.Config) (*Bot, error) {
	db, err := database.Connect(cfg)
	if err != nil {
		return nil, err
	}

	var ircConf girc.Config
	c := cfg.Conf()

	ircConf.Server = c.IRC.Server
	ircConf.Nick = c.IRC.Nick
	ircConf.User = c.IRC.Nick
	ircConf.Name = c.IRC.Nick
	ircConf.SSL = true
	ircConf.Port = 6697
	ircConf.AllowFlood = c.IRC.AllowFlood
	ircConf.RecoverFunc = girc.DefaultRecoverHandler
	ircConf.Version = c.UserAgent

	b := &Bot{
		Config:   cfg,
		DB:       db,
		manager:  c.Manager.TwirpClient(),
		streamer: c.Streamer.TwirpClient(),
		c:        girc.New(ircConf),
	}

	RegisterCommonHandlers(b, b.c)
	RegisterCommandHandlers(b, b.c)

	return b, nil
}

type Bot struct {
	config.Config
	DB *sqlx.DB

	// interfaces to other components
	manager  radio.ManagerService
	streamer streamer.Streamer

	c *girc.Client
}

// runClient connects the irc client and tries to keep it connected until
// the context given is canceled
func (b *Bot) runClient(ctx context.Context) error {
	go func() {
		<-ctx.Done()
		b.c.Close()
	}()

	// TODO: use backoff package out of config
	var connectTime time.Time
	var backoffCount = 1

	for {
		log.Println("client: connecting to:", b.c.Config.Server)

		connectTime = time.Now()
		err := b.c.Connect()
		if err == nil {
			// connect gives us a nil if it returned because of a matching
			// close call on the client; this means we want to shutdown
			return nil
		}
		// otherwise we want to try reconnecting on our own; We use a simple
		// backoff system to not flood the server
		log.Println("client: connect error:", err)

		// reset the backoff count if we managed to stay connected for a long
		// enough period otherwise we double the backoff period
		if time.Since(connectTime) > time.Minute*10 {
			backoffCount = 1
		} else {
			backoffCount *= 2
		}

		time.Sleep(time.Second * 5 * time.Duration(backoffCount))
	}
}
