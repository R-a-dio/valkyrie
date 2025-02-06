package ircbot

import (
	"context"
	"testing"
	"time"

	"github.com/R-a-dio/valkyrie/config"
	"github.com/R-a-dio/valkyrie/mocks"
)

func TestHandlerTimeout(t *testing.T) {
	short, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	ms := &mocks.StorageServiceMock{}
	mss := &mocks.SearchServiceMock{}

	bot, err := NewBot(short, config.TestConfig())
	if err != nil {
		t.Error(err)
	}

	bot.Storage = ms
	bot.Searcher = mss

	hdls := RegexHandler{"test",
		"*", func(e Event) error {
			deadline, _ := e.Ctx.Deadline()
			c := time.NewTicker(time.Until(deadline) + time.Second)

			for {
				select {
				case <-c.C:
					return nil
				case <-e.Ctx.Done():
					return context.Canceled
				}
			}
		},
	}

	err = RegisterCommandHandlers(short, bot, hdls)
	if err != nil {
		t.Error(err)
	}

	// ev := girc.ParseEvent("PRIVMSG #test :test")

}
