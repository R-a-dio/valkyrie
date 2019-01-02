package engine

import (
	"log"
	"reflect"
	"runtime"
	"strings"

	"github.com/R-a-dio/valkyrie/config"
	"github.com/jmoiron/sqlx"
)

// State is the root state object containing both the full configuration object and an
// active database/sql/(sqlx) DB instance.
type State struct {
	// AtomicGlobal contains the root configuration instance
	config.AtomicGlobal
	// DB is a sqlx DB instance if `database.Component` has been loaded, otherwise nil
	DB *sqlx.DB

	defers []deferFn
}

// StartFn is a function that can start a component in the program, if
// the component has resources to cleanup it should return a non-nil DeferFn
// to be called when State.Shutdown is called. The returned DeferFn is only
// used if a nil error is returned.
type StartFn func(*State) (DeferFn, error)

func (fn StartFn) String() string {
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

// DeferFn is a function that is registered to be called when State.Shutdown
// is called, and should be used by components to clean up resources
type DeferFn func() error

// deferFn is used to hold a components name and its DeferFn
type deferFn struct {
	componentName string
	fn            DeferFn
}

// Load calls Start on given components, component names are extracted from the package
// and function names
func (s *State) Load(components ...StartFn) error {
	for _, fn := range components {
		if err := s.Start(fn); err != nil {
			return err
		}
	}

	return nil
}

// Start starts a component and calls Defer if a DeferFn is returned
func (s *State) Start(fn StartFn) error {
	deferFn, err := fn(s)
	if deferFn != nil {
		// always register a returned defer before error check
		s.Defer(fn.String(), deferFn)
	}
	if err != nil {
		log.Printf("engine: start: %s: error: %s\n", fn, err)
		return err
	}

	log.Printf("engine: start: %s: complete\n", fn)
	return nil
}

// Defer registers a function to be called when Shutdown is called on this state
//
// Defer works similar to the defer statement, and calls functions in LIFO order,
// the component string is used in logging
func (s *State) Defer(componentName string, fn DeferFn) {
	s.defers = append(s.defers, deferFn{componentName, fn})
}

// Shutdown calls all deferred functions added by calling Defer and returns any
// errors that were encountered
func (s *State) Shutdown() []error {
	var errs []error
	log.Println("engine: shutdown")
	defer log.Println("engine: shutdown: finished")

	for i, c := range s.defers {
		defer func(i int, c deferFn) {
			if err := c.fn(); err != nil {
				errs = append(errs, err)
				log.Printf("engine: shutdown: %s: error: %s\n", c.componentName, err)
			} else {
				log.Printf("engine: shutdown: %s: complete\n", c.componentName)
			}
		}(i, c)
	}

	return errs
}
