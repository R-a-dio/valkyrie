package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"strings"

	"github.com/R-a-dio/valkyrie/config"
	"github.com/R-a-dio/valkyrie/database"
)

// RootComponent is the root of execution and is passed an error channel to be used
// to send out-of-bound errors on. The process will exit if an error is received.
type RootComponent func(chan<- error) config.StateStart

var components = map[string]RootComponent{}

// AddComponent adds a component with the name given to the executable. The component
// will be executed when called like `hanyuu {name}`
func AddComponent(name string, component RootComponent) {
	name = strings.ToLower(name)
	if _, ok := components[name]; ok {
		panic("duplicate component found: " + name)
	}

	components[name] = component
}

func main() {
	configPath := flag.String("conf", "hanyuu.toml", "filepath to configuration file")
	flag.Parse()
	alternatePath := os.Getenv("HANYUU_CONFIG")

	componentName := flag.Arg(0)
	if componentName == "" {
		fmt.Println("no component name given")
		os.Exit(1)
	}

	root, ok := components[componentName]
	if !ok {
		fmt.Println("unknown component name given:", componentName)
		os.Exit(1)
	}

	var errCh = make(chan error, 2)
	var state config.State
	defer state.Shutdown()

	err := state.Load(
		config.Component(*configPath, alternatePath),
		database.Component,
		root(errCh),
	)
	if err != nil {
		log.Printf("load error: %s", err)
		return
	}

	signalCh := make(chan os.Signal, 2)
	signal.Notify(signalCh, os.Interrupt)

	select {
	case <-signalCh:
		log.Printf("shutdown: interrupt signal received")
	case err := <-errCh:
		if err != nil {
			log.Printf("shutdown: error: %s\n", err)
		}
	}
}
