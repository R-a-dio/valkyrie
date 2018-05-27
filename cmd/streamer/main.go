package main

import (
	"flag"
	"fmt"
	"os"
	"os/signal"

	// _ "expvar"
	_ "net/http/pprof"

	"github.com/R-a-dio/valkyrie/streamer"
)

func main() {
	configPath := flag.String("conf", "hanyuu.toml",
		"filepath to configuration file")
	flag.Parse()

	s, err := streamer.NewState(*configPath)
	if err != nil {
		fmt.Println("startup:", err)
		return
	}

	signalCh := make(chan os.Signal, 2)
	errCh := make(chan error, 2)

	go func() {
		err := streamer.ListenAndServe(s)
		if err != nil {
			fmt.Println("http: serve error:", err)
			errCh <- err
		}
	}()

	signal.Notify(signalCh, os.Interrupt)

	select {
	case <-signalCh:
		fmt.Println("received interrupt signal, exiting process")
	case <-errCh:
		fmt.Println("received error, exiting process")
	}

	s.Shutdown()
}
