package ircbot

import (
	"context"
	"net"
	"os"
	"time"

	"github.com/cenkalti/backoff"
	"github.com/rs/zerolog"
	"go.opentelemetry.io/otel"

	radio "github.com/R-a-dio/valkyrie"
	"github.com/R-a-dio/valkyrie/config"
	"github.com/R-a-dio/valkyrie/errors"
	"github.com/R-a-dio/valkyrie/search"
	"github.com/R-a-dio/valkyrie/storage"
	"github.com/R-a-dio/valkyrie/util"
	"github.com/R-a-dio/valkyrie/util/buildinfo"
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
	announce := NewAnnounceService(cfg, b.Storage, b)

	// setup a http server for our RPC API
	srv, err := NewGRPCServer(ctx, announce)
	if err != nil {
		return err
	}

	ln, err := net.Listen("tcp", cfg.Conf().IRC.RPCAddr.String())
	if err != nil {
		return err
	}

	b.StatusValue = util.StreamValue(ctx, cfg.Manager.CurrentStatus, func(ctx context.Context, s radio.Status) {
		err := announce.AnnounceSong(ctx, s)
		if err != nil {
			zerolog.Ctx(ctx).Error().Ctx(ctx).Err(err).Msg("failed to announce status")
		}

		err = announce.AnnounceThread(ctx, s.Thread)
		if err != nil {
			zerolog.Ctx(ctx).Error().Ctx(ctx).Err(err).Msg("failed to announce thread")
		}
	})
	b.UserValue = util.StreamValue(ctx, cfg.Manager.CurrentUser, func(ctx context.Context, user *radio.User) {
		err := announce.AnnounceUser(ctx, user)
		if err != nil {
			zerolog.Ctx(ctx).Error().Ctx(ctx).Err(err).Msg("failed to announce user")
		}
	})
	b.ListenersValue = util.StreamValue(ctx, cfg.Manager.CurrentListeners)

	errCh := make(chan error, 2)
	go func() {
		// run the irc client
		errCh <- b.runClient(ctx)
	}()
	go func() {
		// run the grpc server
		errCh <- srv.Serve(ln)
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
	ircConf.Version = c.UserAgent + " (" + buildinfo.ShortRef + ")"
	if c.IRC.NickPassword != "" {
		ircConf.SASL = &girc.SASLPlain{
			User: c.IRC.Nick,
			Pass: c.IRC.NickPassword,
		}
	}

	b := &Bot{
		cfgUserRequestDelay: config.Value(cfg, func(cfg config.Config) time.Duration {
			return time.Duration(cfg.Conf().UserRequestDelay)
		}),
		cfgNick: config.Value(cfg, func(cfg config.Config) string {
			return cfg.Conf().IRC.Nick
		}),
		cfgNickPassword: config.Value(cfg, func(cfg config.Config) string {
			return cfg.Conf().IRC.NickPassword
		}),
		cfgChannels: config.Value(cfg, func(cfg config.Config) []string {
			return cfg.Conf().IRC.Channels
		}),
		cfgMainChannel: config.Value(cfg, func(cfg config.Config) string {
			return cfg.Conf().IRC.MainChannel
		}),
		Storage:  store,
		Searcher: ss,
		Queue:    cfg.Queue,
		Streamer: cfg.Streamer,
		Manager:  cfg.Manager,
		Guest:    cfg.Guest,
		c:        girc.New(ircConf),
	}

	RegisterGuestHandlers(ctx, b.c, cfg.Guest)
	if err = RegisterCommonHandlers(b, b.c); err != nil {
		return nil, err
	}
	if err = RegisterCommandHandlers(ctx, b, reHandlers...); err != nil {
		return nil, err
	}
	//b.c.Handlers.Add(girc.ALL_EVENTS, ZerologLogger(ctx))

	go b.syncConfiguration(ctx)
	return b, nil
}

type Bot struct {
	cfgUserRequestDelay func() time.Duration
	cfgNick             func() string
	cfgNickPassword     func() string
	cfgChannels         func() []string
	cfgMainChannel      func() string

	Storage radio.StorageService

	// interfaces to other components
	Searcher radio.SearchService
	Queue    radio.QueueService
	Streamer radio.StreamerService
	Guest    radio.GuestService
	Manager  radio.ManagerService

	// Values used by commands
	StatusValue    *util.Value[radio.Status]
	ListenersValue *util.Value[radio.Listeners]
	UserValue      *util.Value[*radio.User]

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

	cb := config.NewConnectionBackoff(ctx)

	doConnect := func() error {
		const op errors.Op = "irc/Bot.runClient.doConnect"
		ctx, span := otel.Tracer("").Start(ctx, string(op))
		defer span.End()

		zerolog.Ctx(ctx).Info().Ctx(ctx).Str("address", b.c.Config.Server).Msg("connecting")
		err := b.c.Connect()
		if err != nil {
			zerolog.Ctx(ctx).Error().Ctx(ctx).Str("address", b.c.Config.Server).Err(err).Msg("error connecting")
			// reset the backoff if we managed to stay connected for a decent period of
			// time so we will retry fast again
			if cb.GetElapsedTime() > time.Minute*10 {
				cb.Reset()
			}
			return errors.E(op, err)
		}
		return nil
	}

	return backoff.Retry(doConnect, cb)
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

		// check if we lost our nickname
		if wanted := b.cfgNick(); b.c.GetNick() != wanted {
			b.c.Cmd.Nick(wanted)
		}

		// check if we're still on all our wanted channels
		for _, wanted := range b.cfgChannels() {
			if !b.c.IsInChannel(wanted) {
				b.c.Cmd.Join(wanted)
			}
		}
	}
}

func ZerologLogger(ctx context.Context) func(c *girc.Client, e girc.Event) {
	logger := zerolog.Ctx(ctx)

	return func(c *girc.Client, e girc.Event) {
		z := logger.Info().
			Time("timestamp", e.Timestamp).
			Str("command", e.Command)

		if e.Source != nil {
			z = z.Str("host", e.Source.Host).
				Str("ident", e.Source.Ident).
				Str("name", e.Source.Name)
		}
		if !e.Sensitive {
			z = z.Strs("params", e.Params)
		}

		z.Msg("raw-event")
	}
}
