package ircbot

import (
	"context"
	"log"
	"net"
	"os"
	"time"

	"github.com/cenkalti/backoff"

	radio "github.com/R-a-dio/valkyrie"
	"github.com/R-a-dio/valkyrie/config"
	"github.com/R-a-dio/valkyrie/database"
	"github.com/jmoiron/sqlx"
	"github.com/lrstanley/girc"
)

// Execute executes the ircbot with the context and configuration given. it returns with
// any error that occurs; Execution can be interrupted by canceling the context given.
func Execute(ctx context.Context, cfg config.Config) error {
	b, err := NewBot(ctx, cfg)

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
func NewBot(ctx context.Context, cfg config.Config) (*Bot, error) {
	db, err := database.Connect(cfg)
	if err != nil {
		return nil, err
	}

	var ircConf girc.Config
	c := cfg.Conf()

	if c.IRC.EnableEcho {
		ircConf.Out = os.Stdout
	}
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
		Manager:  c.Manager.TwirpClient(),
		Streamer: c.Streamer.TwirpClient(),
		c:        girc.New(ircConf),
	}

	RegisterCommonHandlers(b, b.c)
	RegisterCommandHandlers(ctx, b)

	return b, nil
}

type Bot struct {
	config.Config
	DB *sqlx.DB

	// interfaces to other components
	Manager  radio.ManagerService
	Streamer radio.StreamerService

	c *girc.Client
}

// runClient connects the irc client and tries to keep it connected until
// the context given is canceled
func (b *Bot) runClient(ctx context.Context) error {
	go func() {
		// call close on the client when our context is done
		<-ctx.Done()
		b.c.Close()
	}()

	cb := config.NewConnectionBackoff()
	cbctx := backoff.WithContext(cb, ctx)

	doConnect := func() error {
		log.Println("client: connecting to:", b.c.Config.Server)
		err := b.c.Connect()
		if err != nil {
			log.Println("irc: connect error:", err)
			// reset the backoff if we managed to stay connected for a decent period of
			// time so we will retry fast again
			if cb.GetElapsedTime() > time.Minute*10 {
				cb.Reset()
			}
		}
		return err
	}

	return backoff.Retry(doConnect, cbctx)
}
