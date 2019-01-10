package engine

import (
	"log"
	"os"
	"os/signal"
	"reflect"
	"runtime"
	"strings"
	"syscall"

	"github.com/R-a-dio/valkyrie/config"
	"github.com/jmoiron/sqlx"
)

// Engine is the root state object containing both the full configuration object and an
// active database/sql/(sqlx) DB instance.
type Engine struct {
	// Config is our configuration instance
	config.Config
	// DB is a sqlx DB instance if `database.Component` has been loaded, otherwise nil
	DB *sqlx.DB

	defers []deferFn
}

// StartFn is a function that can start a component in the program, if
// the component has resources to cleanup it should return a non-nil DeferFn
// to be called when Engine.Shutdown is called. The returned DeferFn is only
// used if a nil error is returned.
type StartFn func(*Engine) (DeferFn, error)

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
func (e *Engine) Load(components ...StartFn) error {
	for _, fn := range components {
		if err := e.Start(fn); err != nil {
			return err
		}
	}

	return nil
}

// Run calls Load on the given components, and then waits for an error or
// signal to interrupt the program. Upon return, Shutdown is called
func (e *Engine) Run(errCh chan error, components ...StartFn) error {
	err := e.Load(components...)

	// defer before the error check so we run all defer functions of the
	// components that were successfully initialized
	defer e.Shutdown()
	if err != nil {
		log.Printf("shutdown: load error: %s", err)
		return err
	}

	signalCh := make(chan os.Signal, 2)
	signal.Notify(signalCh, os.Interrupt, syscall.SIGHUP)

	for {
		select {
		case sig := <-signalCh:
			switch sig {
			case os.Interrupt:
				log.Printf("shutdown: SIGINT received")
				return nil
			case syscall.SIGHUP:
				log.Printf("SIGHUP received: reload not implemented")

				// TODO: reload
			}
		case err := <-errCh:
			if err != nil {
				log.Printf("shutdown: error: %s", err)
			}
			return err
		}
	}
}

// Start starts a component and calls Defer if a DeferFn is returned
func (e *Engine) Start(fn StartFn) error {
	deferFn, err := fn(e)
	if deferFn != nil {
		// always register a returned defer before error check
		e.Defer(fn.String(), deferFn)
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
func (e *Engine) Defer(componentName string, fn DeferFn) {
	e.defers = append(e.defers, deferFn{componentName, fn})
}

// Shutdown calls all deferred functions added by calling Defer and returns any
// errors that were encountered
func (e *Engine) Shutdown() []error {
	var errs []error
	log.Println("engine: shutdown")
	defer log.Println("engine: shutdown: finished")

	for i, c := range e.defers {
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
