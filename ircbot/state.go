package ircbot

import (
	"log"
	"os"
	"time"

	"github.com/R-a-dio/valkyrie/config"
	"github.com/jmoiron/sqlx"
	"github.com/lrstanley/girc"
)

type State struct {
	config.AtomicGlobal

	client *girc.Client
	db     *sqlx.DB
}

func NewState(configPath string) (*State, error) {
	var s State

	f, err := os.Open(configPath)
	if err != nil {
		return nil, err
	}

	log.Println("start: loading configuration")
	s.AtomicGlobal, err = config.LoadAtomic(f)
	if err != nil {
		return nil, err
	}

	log.Println("start: loading database")
	if err = s.loadDatabase(); err != nil {
		return nil, err
	}

	log.Println("start: loading irc client")
	if err = s.loadClient(); err != nil {
		return nil, err
	}

	log.Println("start: loading irc handlers")
	if err = s.loadIRCHandlers(); err != nil {
		return nil, err
	}

	return &s, nil
}

// RunClient connects the irc client and tries to keep it connected until
// Shutdown is called
func (s *State) RunClient() error {
	var connectTime time.Time
	var backoffCount = 1

	for {
		log.Println("client: connecting to:", s.client.Config.Server)

		connectTime = time.Now()
		err := s.client.Connect()
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

// Shutdown closes all things associated with this state, should be called
// before program exit
func (s *State) Shutdown() {
	log.Println("shutdown: closing irc client")
	s.client.Close()
	log.Println("shutdown: error:", s.db.Close())
	//s.httpserver.Close()
	log.Println("shutdown: finished")
}

func (s *State) loadDatabase() (err error) {
	conf := s.Conf()

	s.db, err = sqlx.Open(conf.Database.DriverName, conf.Database.DSN)
	return err
}

func (s *State) loadClient() (err error) {
	var c girc.Config
	conf := s.Conf()

	c.Server = conf.IRC.Server
	c.Nick = conf.IRC.Nick
	c.User = c.Nick
	c.Name = c.Nick
	c.SSL = true
	c.Port = 6697
	c.AllowFlood = conf.IRC.AllowFlood
	c.RecoverFunc = girc.DefaultRecoverHandler
	c.Version = conf.UserAgent

	s.client = girc.New(c)
	return nil
}

func (s *State) loadIRCHandlers() (err error) {
	RegisterCommonHandlers(s)
	s.client.Handlers.Add(girc.PRIVMSG, parseCommand) // parse.go
	return
}
