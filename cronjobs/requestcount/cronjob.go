package main

import (
	"flag"
	"log"
	"os"
	"os/signal"

	"github.com/R-a-dio/valkyrie/config"
	"github.com/R-a-dio/valkyrie/database"
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
	)
	if err != nil {
		return
	}

	go func() {
		err := cronjob(&state)
		if err != nil {
			log.Println("error executing cronjob:", err)
			os.Exit(1)
		}
	}()

	signalCh := make(chan os.Signal, 2)
	signal.Notify(signalCh, os.Interrupt)

	select {
	case <-signalCh:
		log.Printf("shutdown: interrupt signal received: shutting down")
	case err := <-errCh:
		log.Printf("shutdown: http server error: %s\n", err)
	}
}
