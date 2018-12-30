package main

import (
	"flag"
	"log"
	"os"
	"os/signal"

	"github.com/R-a-dio/valkyrie/config"
	"github.com/R-a-dio/valkyrie/database"
	"github.com/R-a-dio/valkyrie/manager"
)

func main() {
	configPath := flag.String("conf", "hanyuu.toml", "filepath to configuration file")
	flag.Parse()

	var errCh = make(chan error, 2)
	var state config.State
	defer state.Shutdown()

	err := state.Load(
		config.Component(*configPath),
		database.Component,
		manager.Component(errCh),
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
