package config

import (
	"log"
	"os"
	"reflect"
	"runtime"
	"strings"

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

func (fn StateStart) String() string {
	p := reflect.ValueOf(fn).Pointer()
	fullname := runtime.FuncForPC(p).Name()
	// handle closure names, these have ".funcN" added to their full name
	fullname = strings.TrimRight(fullname, "1234567890")
	fullname = strings.TrimSuffix(fullname, ".func")

	// find the function name
	i := strings.LastIndex(fullname, ".")
	pkgName, fnName := fullname[:i], fullname[i+1:]

	i = strings.LastIndex(pkgName, "/")
	pkgName = strings.ToLower(pkgName[i+1:])

	// if the function is only named Component we only use the package name
	if fnName == "Component" {
		return pkgName
	}

	fnName = strings.ToLower(strings.TrimSuffix(fnName, "Component"))
	return pkgName + "-" + fnName
}

// StateDefer is a function that is registered to be called when State.Shutdown
// is called, and should be used by components to clean up resources
type StateDefer func() error

// stateDefer is used to hold a components name and it's StateDefer
type stateDefer struct {
	component string
	fn        StateDefer
}

// Load calls Start on given components, component names are extracted from the package
// and function names
func (s *State) Load(components ...StateStart) error {
	for _, fn := range components {
		if err := s.Start(fn); err != nil {
			return err
		}
	}

	return nil
}

// Start starts a component and calls Defer if a StateDefer is returned
func (s *State) Start(fn StateStart) error {
	defr, err := fn(s)
	if defr != nil {
		// always register a returned defer before error check
		s.Defer(fn.String(), defr)
	}
	if err != nil {
		log.Printf("state: start: %s: error: %s\n", fn, err)
		return err
	}

	log.Printf("state: start: %s: complete\n", fn)
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
	log.Println("state: shutdown")
	defer log.Println("state: shutdown: finished")

	for i, c := range s.defers {
		defer func(i int, c stateDefer) {
			if err := c.fn(); err != nil {
				errs = append(errs, err)
				log.Printf("state: shutdown: %s: error: %s\n", c.component, err)
			} else {
				log.Printf("state: shutdown: %s: complete\n", c.component)
			}
		}(i, c)
	}

	return errs
}
