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

type RootComponent func(chan<- error) config.StateStart

var components = map[string]RootComponent{}

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
		config.Component(*configPath),
		database.Component,
		root(errCh),
	)
	if err != nil {
		return
	}

	signalCh := make(chan os.Signal, 2)
	signal.Notify(signalCh, os.Interrupt)

	select {
	case <-signalCh:
		log.Printf("shutdown: interrupt signal received: shutting down")
	case err := <-errCh:
		log.Printf("shutdown: http server error: %s\n", err)
	}
}
