package config

import (
	"log"
	"os"

	"github.com/jmoiron/sqlx"
)

// Component loads the configuration file given
func Component(path string) StateStart {
	return func(s *State) (StateDefer, error) {
		f, err := os.Open(path)
		if err != nil {
			return nil, err
		}
		defer f.Close()

		s.AtomicGlobal, err = LoadAtomic(f)
		if err != nil {
			return nil, err
		}

		return nil, nil
	}
}

type State struct {
	AtomicGlobal
	DB *sqlx.DB

	defers []stateDefer
}

// StateStart is a function that can start a component in the program, if
// the component has resources to cleanup it should return a non-nil StateDefer
// to be called when State.Shutdown is called. The returned StateDefer is only
// used if a nil error is returned.
type StateStart func(*State) (StateDefer, error)

// StateDefer is a function that is registered to be called when State.Shutdown
// is called, and should be used by components to clean up resources
type StateDefer func() error

// stateDefer is used to hold a components name and it's StateDefer
type stateDefer struct {
	component string
	fn        StateDefer
}

// Start starts a component and calls Defer if a StateDefer is returned
func (s *State) Start(component string, fn StateStart) error {
	defr, err := fn(s)
	if err != nil {
		log.Printf("state: start: %s: error: %s\n", component, err)
		return err
	}
	if defr != nil {
		s.Defer(component, defr)
	}
	log.Printf("state: start: %s: complete\n", component)
	return nil
}

// Defer registers a function to be called when Shutdown is called on this state
//
// Defer works similar to the defer statement, and calls functions in LIFO order,
// the component string is used in logging
func (s *State) Defer(component string, fn StateDefer) {
	s.defers = append(s.defers, stateDefer{component, fn})
}

// Shutdown calls all deferred functions added by calling Defer and returns any
// errors that were encountered
func (s *State) Shutdown() []error {
	var errs []error
	log.Println("state: shutdown: starting")
	defer log.Println("state: shutdown: finished")

	for i, c := range s.defers {
		defer func(i int, c stateDefer) {
			if err := c.fn(); err != nil {
				errs = append(errs, err)
				log.Printf("state: shutdown: [%d/%d] %s: error: %s\n",
					i, len(s.defers), // print current index and total indices
					c.component,
					err,
				)
			} else {
				log.Printf("state: shutdown: [%d/%d] %s: complete\n",
					i, len(s.defers), // print current index and total indices
					c.component,
				)
			}
		}(i, c)
	}

	return errs
}
