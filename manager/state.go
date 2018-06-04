package manager

import (
	"fmt"
	"os"
	"time"

	"github.com/jmoiron/sqlx"

	"github.com/R-a-dio/valkyrie/config"
)

// State is a global-type application state, and is passed around all loaded
// components as a shared root
type State struct {
	db *sqlx.DB

	config.AtomicGlobal
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

	s.db, err = sqlx.Connect(conf.Database.DriverName, conf.Database.DSN)
	if err != nil {
		return err
	}
	s.Closer("database", s.db.Close)
	return nil
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

	return &s, nil
}
