package ircbot

import (
	"context"
	"net"
	"os"
	"time"

	"github.com/cenkalti/backoff"
	"github.com/rs/zerolog"

	radio "github.com/R-a-dio/valkyrie"
	"github.com/R-a-dio/valkyrie/config"
	"github.com/R-a-dio/valkyrie/errors"
	"github.com/R-a-dio/valkyrie/search"
	"github.com/R-a-dio/valkyrie/storage"
	"github.com/lrstanley/girc"
)

// Execute executes the ircbot with the context and configuration given. it returns with
// any error that occurs; Execution can be interrupted by canceling the context given.
func Execute(ctx context.Context, cfg config.Config) error {
	b, err := NewBot(ctx, cfg)
	if err != nil {
		return err
	}

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	// setup our announce service
	announce := NewAnnounceService(b.Config, b.Storage, b)

	manager := cfg.Conf().Manager.Client()

	// setup a http server for our RPC API
	srv, err := NewHTTPServer(announce)
	if err != nil {
		return err
	}

	ln, err := net.Listen("tcp", cfg.Conf().IRC.ListenAddr)
	if err != nil {
		return err
	}

	errCh := make(chan error, 2)
	go func() {
		// run the irc client
		errCh <- b.runClient(ctx)
	}()
	go func() {
		// run the grpc server
		errCh <- srv.Serve(ln)
	}()
	go func() {
		// setup our listener for new songs on the stream
		errCh <- WaitForStatus(ctx, manager, announce)
	}()

	// wait for our context to be canceled or Serve to error out
	select {
	case <-ctx.Done():
		srv.Stop()
		return nil
	case err = <-errCh:
		return err
	}

}

// NewBot returns a Bot with configuration and handlers loaded
func NewBot(ctx context.Context, cfg config.Config) (*Bot, error) {
	const op errors.Op = "irc/NewBot"

	store, err := storage.Open(ctx, cfg)
	if err != nil {
		return nil, errors.E(op, err)
	}

	ss, err := search.Open(ctx, cfg)
	if err != nil {
		return nil, errors.E(op, err)
	}

	// setup irc configuration
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
	ircConf.Bind = c.IRC.BindAddr
	ircConf.AllowFlood = c.IRC.AllowFlood
	ircConf.RecoverFunc = girc.DefaultRecoverHandler
	ircConf.Version = c.UserAgent

	b := &Bot{
		Config:   cfg,
		Storage:  store,
		Manager:  c.Manager.Client(),
		Streamer: c.Streamer.Client(),
		Searcher: ss,
		c:        girc.New(ircConf),
	}

	RegisterCommonHandlers(b, b.c)
	RegisterCommandHandlers(ctx, b)

	go b.syncConfiguration(ctx)
	return b, nil
}

type Bot struct {
	config.Config
	Storage radio.StorageService

	// interfaces to other components
	Manager  radio.ManagerService
	Streamer radio.StreamerService
	Searcher radio.SearchService

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
		const op errors.Op = "irc/Bot.runClient.doConnect"

		zerolog.Ctx(ctx).Info().Str("address", b.c.Config.Server).Msg("connecting")
		err := b.c.Connect()
		if err != nil {
			zerolog.Ctx(ctx).Error().Str("address", b.c.Config.Server).Err(err).Msg("connecting")
			// reset the backoff if we managed to stay connected for a decent period of
			// time so we will retry fast again
			if cb.GetElapsedTime() > time.Minute*10 {
				cb.Reset()
			}

			err = errors.E(op, err)
		}
		return err
	}

	return backoff.Retry(doConnect, cbctx)
}

// syncConfiguration tries to keep the irc client state in sync with what is
// configured, this includes channels and the nickname we want
func (b *Bot) syncConfiguration(ctx context.Context) {
	tick := time.NewTicker(time.Second * 30)
	defer tick.Stop()

	for {
		select {
		case <-tick.C:
		case <-ctx.Done():
			return
		}

		// check if we're connected at all
		if !b.c.IsConnected() {
			continue
		}

		c := b.Conf()
		// check if we lost our nickname
		if b.c.GetNick() != c.IRC.Nick {
			b.c.Cmd.Nick(c.IRC.Nick)
		}

		// check if we're still on all our wanted channels
		for _, wanted := range c.IRC.Channels {
			if !b.c.IsInChannel(wanted) {
				b.c.Cmd.Join(wanted)
			}
		}
	}
}

func WaitForStatus(ctx context.Context, manager radio.ManagerService, announce radio.AnnounceService) error {
	const op errors.Op = "ircbot.WaitForStatus"

	var noRetry = make(chan time.Time)
	close(noRetry)
	var retry <-chan time.Time = noRetry

	for {
		stream, err := manager.CurrentStatus(ctx)
		if err != nil {
			return errors.E(op, err)
		}
		defer stream.Close()

		for {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-retry:
			}
			retry = noRetry

			status, err := stream.Next()
			if err != nil {
				retry = time.After(time.Second * 5)
				continue
			}

			err = announce.AnnounceSong(ctx, status)
			if err != nil {
				retry = time.After(time.Second * 5)
				continue
			}
		}
	}
}
