package ircbot

import (
	"bytes"
	"context"
	"testing"

	radio "github.com/R-a-dio/valkyrie"
	"github.com/lrstanley/girc"
	"github.com/stretchr/testify/require"
)

func TestAnnounceThread(t *testing.T) {
	ctx := context.Background()
	thread := radio.Thread("image:https://example.org/show.png")

	newAnnounceService := func(last radio.Thread) (*announceService, *bytes.Buffer) {
		var debug bytes.Buffer
		ann := &announceService{
			cfgMainChannel: func() string { return "#test" },
			lastThread:     last,
			bot: &Bot{
				c: girc.New(girc.Config{
					Server: "localhost",
					Nick:   "test",
					Debug:  &debug,
				}),
			},
		}
		return ann, &debug
	}

	t.Run("empty", func(t *testing.T) {
		ann, debug := newAnnounceService(thread)

		err := ann.AnnounceThread(ctx, "")
		require.NoError(t, err)
		require.Equal(t, radio.Thread(""), ann.lastThread)
		require.NotContains(t, debug.String(), "PRIVMSG #test")
	})

	t.Run("non-empty", func(t *testing.T) {
		ann, debug := newAnnounceService("")

		err := ann.AnnounceThread(ctx, thread)
		require.NoError(t, err)
		require.Equal(t, thread, ann.lastThread)
		require.Contains(t, debug.String(), "PRIVMSG #test")
	})

	t.Run("same after empty", func(t *testing.T) {
		ann, debug := newAnnounceService(thread)

		err := ann.AnnounceThread(ctx, "")
		require.NoError(t, err)
		require.NotContains(t, debug.String(), "PRIVMSG #test")

		err = ann.AnnounceThread(ctx, thread)
		require.NoError(t, err)
		require.Equal(t, thread, ann.lastThread)
		require.Contains(t, debug.String(), "PRIVMSG #test")
	})
}
