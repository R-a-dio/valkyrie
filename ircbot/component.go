package ircbot

import (
	"log"
	"time"

	"github.com/R-a-dio/valkyrie/engine"
	"github.com/R-a-dio/valkyrie/rpc/manager"
	"github.com/lrstanley/girc"
)

type Bot struct {
	*engine.Engine

	Manager manager.Manager

	c *girc.Client
	// finished is closed when runClient returns
	finished chan struct{}
}

func (b *Bot) Shutdown() error {
	b.c.Close()

	<-b.finished
	// TODO: implement error return from connect
	return nil
}

func Component(errCh chan<- error) engine.StartFn {
	return func(e *engine.Engine) (engine.DeferFn, error) {
		var ircConf girc.Config
		c := e.Conf()

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
			Engine:   e,
			Manager:  c.Manager.TwirpClient(),
			finished: make(chan struct{}),
			c:        girc.New(ircConf),
		}

		err := e.Load(
			b.HandlerComponent(RegisterCommonHandlers),
			b.HandlerComponent(RegisterCommandHandlers),
		)

		go b.runClient()

		return b.Shutdown, err
	}
}

func (b *Bot) HandlerComponent(fn func(*Bot, *girc.Client) error) engine.StartFn {
	return func(e *engine.Engine) (engine.DeferFn, error) {
		return nil, fn(b, b.c)
	}
}

// runClient connects the irc client and tries to keep it connected until
// Shutdown is called
func (b *Bot) runClient() error {
	defer close(b.finished)

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
