package ircbot

import (
	"context"
	"testing"
	"time"

	"github.com/R-a-dio/valkyrie/config"
	"github.com/R-a-dio/valkyrie/mocks"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHandlerTimeout(t *testing.T) {
	t.SkipNow()
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

func TestReHandlersCompiles(t *testing.T) {
	for _, re := range reHandlers {
		t.Run(re.name, func(t *testing.T) {
			compileRegex(re.regex)
		})
	}
}

func TestRegexpHandlers(t *testing.T) {
	type trhcase struct {
		input      string
		shouldFail bool
	}
	testCases := map[string][]trhcase{}
	testCases["now_playing"] = []trhcase{
		{input: ".np"},
		{input: ".nowplaying"},
		{input: ".nowp"},
		{input: ".nplaying"},
		{input: ".np something", shouldFail: true},
	}
	testCases["last_played"] = []trhcase{
		{input: ".lp"},
		{input: "!lplayed"},
		{input: ".lastplayed"},
		{input: "@lastp"},
		{input: ".lp something", shouldFail: true},
	}
	testCases["streamer_queue"] = []trhcase{
		{input: ".q"},
		{input: ".queue"},
		{input: ".queue something", shouldFail: true},
	}
	testCases["streamer_queue_length"] = []trhcase{
		{input: ".q l"},
		{input: ".queue l"},
		{input: ".q length"},
		{input: ".queue length"},
		{input: ".q l something", shouldFail: true},
	}
	testCases["streamer_user_info"] = []trhcase{}
	testCases["fave_track"] = []trhcase{}
	testCases["fave_list"] = []trhcase{}
	testCases["thread_url"] = []trhcase{}
	testCases["channel_topic"] = []trhcase{}
	testCases["kill_streamer"] = []trhcase{}
	testCases["random_track_request"] = []trhcase{}
	testCases["lucky_track_request"] = []trhcase{}
	testCases["search_track"] = []trhcase{}
	testCases["request_track"] = []trhcase{}
	testCases["last_request_info"] = []trhcase{}
	testCases["track_info"] = []trhcase{
		{input: ".info"},
		{input: ".i"},
		{input: ".i 503"},
		{input: ".info 1023232"},
		{input: "!iq", shouldFail: true},
	}
	testCases["track_tags"] = []trhcase{}
	testCases["guest_auth"] = []trhcase{}
	testCases["guest_create"] = []trhcase{}

	for _, re := range reHandlers {
		t.Run(re.name, func(t *testing.T) {
			r := compileRegex(re.regex)

			cases := testCases[re.name]
			require.NotNil(t, cases)

			for _, tc := range testCases[re.name] {
				match := FindNamedSubmatches(r, tc.input)

				// shouldFail path
				if tc.shouldFail {
					if match != nil {
						// failure path, we had a match
						t.Logf("regexp match even though shouldFail is set:\n\tregexp: %s\n\tinput: %s",
							re.regex, tc.input)
					}
					// success path, no match
					continue
				}

				// should succeed path
				if !assert.NotNil(t, match) {
					t.Logf("failed regexp match:\n\tregexp: %s\n\tinput: %s", re.regex, tc.input)
				}
			}
		})
	}
}
