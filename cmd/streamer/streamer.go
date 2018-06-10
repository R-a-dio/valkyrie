package main

import (
	"flag"
	"log"
	"os"
	"os/signal"

	"github.com/R-a-dio/valkyrie/config"
	"github.com/R-a-dio/valkyrie/database"
	"github.com/R-a-dio/valkyrie/streamer"
)

func main() {
	configPath := flag.String("conf", "hanyuu.toml", "filepath to configuration file")
	flag.Parse()

	var state config.State
	defer state.Shutdown()

	err := state.Start("config", config.Component(*configPath))
	if err != nil {
		log.Printf("init: config error: %s\n", err)
		return
	}

	err = state.Start("database", database.Component)
	if err != nil {
		log.Printf("init: database error: %s\n", err)
		return
	}

	errCh := make(chan error, 2)
	err = state.Start("streamer", streamer.Component(errCh))
	if err != nil {
		log.Printf("init: streamer error: %s\n", err)
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
