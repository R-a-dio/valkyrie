package streamer

import (
	"fmt"
	"os"
	"time"

	"github.com/R-a-dio/valkyrie/config"
	_ "github.com/go-sql-driver/mysql" // mariadb
	"github.com/jmoiron/sqlx"
)

// State is a global-type application state, and is passed around all loaded
// components as a shared root
type State struct {
	db *sqlx.DB
	config.AtomicGlobal

	queue    *Queue
	streamer *Streamer

	closers []stateCloser
}

type stateCloser struct {
	component string
	fn        func() error
}

// Closer registers a function to be called when Shutdown is called on this
// state instance, functions are called in LIFO order similar to defer
func (s *State) Closer(component string, fn func() error) {
	s.closers = append(s.closers, stateCloser{component, fn})
}

// Shutdown stops all components using this state
func (s *State) Shutdown() {
	fmt.Println("starting: shutting down")
	for i := len(s.closers) - 1; i >= 0; i-- {
		c := s.closers[i]
		if err := c.fn(); err != nil {
			fmt.Printf("shutdown error [%d]: %s: %s\n", i, c.component, err)
		}
	}
	fmt.Println("finished: shutting down")
	time.Sleep(time.Millisecond * 250)
}

func (s *State) loadDatabase() (err error) {
	conf := s.Conf()

	s.db, err = sqlx.Open(conf.Database.DriverName, conf.Database.DSN)
	s.Closer("database", s.db.Close)
	return s.db.Ping()
}

// LoadQueue loads a Queue for this state, returns any errors encountered
func (s *State) loadQueue() (err error) {
	s.queue, err = NewQueue(s)
	s.Closer("queue", s.queue.Save)
	return err
}

func (s *State) loadStreamer() (err error) {
	s.streamer, err = NewStreamer(s)
	s.Closer("streamer", s.streamer.ForceStop)
	return err
}

// NewState initializes a state struct with all the required items
func NewState(configPath string) (*State, error) {
	var s State
	var err error
	// shutdown the things we've loaded already when an error occurs
	defer func() {
		if err != nil {
			s.Shutdown()
		}
	}()

	f, err := os.Open(configPath)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	fmt.Println("startup: loading configuration")
	s.AtomicGlobal, err = config.LoadAtomic(f)
	if err != nil {
		return nil, err
	}

	fmt.Println("startup: loading database")
	if err = s.loadDatabase(); err != nil {
		return nil, err
	}

	fmt.Println("startup: loading queue")
	if err = s.loadQueue(); err != nil {
		return nil, err
	}

	fmt.Println("startup: loading streamer")
	if err = s.loadStreamer(); err != nil {
		return nil, err
	}

	return &s, nil
}
