package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"

	"github.com/R-a-dio/valkyrie/ircbot"
	_ "github.com/go-sql-driver/mysql"
)

func main() {
	configPath := flag.String("conf", "hanyuu.toml",
		"filepath to configuration file")

	flag.Parse()

	s, err := ircbot.NewState(*configPath)
	if err != nil {
		fmt.Println(err)
		return
	}

	go func() {
		err := s.RunClient()
		if err != nil {
			log.Println("client error:", err)
		}
	}()

	ch := make(chan os.Signal, 2)
	signal.Notify(ch, os.Interrupt)

	<-ch
	s.Shutdown()
}
