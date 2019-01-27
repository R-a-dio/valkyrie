package ircbot

import (
	"context"
	"net/http"
	"net/http/pprof"
	"strconv"
	"strings"
	"time"

	radio "github.com/R-a-dio/valkyrie"
	"github.com/R-a-dio/valkyrie/database"
	"github.com/R-a-dio/valkyrie/rpc"
	"github.com/lrstanley/girc"
)

func NewHTTPServer(b *Bot) (*http.Server, error) {
	rpcServer := rpc.NewAnnouncerServer(rpc.NewAnnouncer(b), nil)
	mux := http.NewServeMux()
	// rpc server path
	mux.Handle(rpc.AnnouncerPathPrefix, rpcServer)

	// debug symbols
	mux.HandleFunc("/debug/pprof/", pprof.Index)
	mux.HandleFunc("/debug/pprof/cmdline", pprof.Cmdline)
	mux.HandleFunc("/debug/pprof/profile", pprof.Profile)
	mux.HandleFunc("/debug/pprof/symbol", pprof.Symbol)
	mux.HandleFunc("/debug/pprof/trace", pprof.Trace)

	conf := b.Conf()
	server := &http.Server{Addr: conf.IRC.Addr, Handler: mux}
	return server, nil
}

func (b *Bot) AnnounceSong(ctx context.Context, status radio.Status) error {
	message := "Now starting:{red} '%s' {clear}[%s/%s](%s), %s, %s, {green}LP:{clear} %s"

	var lastPlayedDiff time.Duration
	if status.Song.LastPlayed.IsZero() {
		lastPlayedDiff = time.Since(status.Song.LastPlayed)
	}

	songPosition := time.Since(status.SongInfo.Start)
	songLength := status.SongInfo.End.Sub(status.SongInfo.Start)

	db := database.Handle(ctx, b.DB)
	favoriteCount, _ := database.SongFaveCount(db, status.Song)
	playedCount, _ := database.SongPlayedCount(db, status.Song)

	message = Fmt(message,
		status.Song.Metadata,
		FormatPlaybackDuration(songPosition), FormatPlaybackDuration(songLength),
		Pluralf("%d listeners", int64(status.Listeners)),
		Pluralf("%d faves", favoriteCount),
		Pluralf("played %d times", playedCount),
		FormatLongDuration(lastPlayedDiff),
	)

	b.c.Cmd.Message(b.Conf().IRC.MainChannel, message)

	if favoriteCount == 0 {
		// save ourselves some work if there are no favorites
		return nil
	}
	usersWithFave, err := database.GetSongFavorites(db, status.Song.ID)
	if err != nil {
		return err
	}

	// we only send notifications to people that are on the configured main channel
	channel := b.c.LookupChannel(b.Conf().IRC.MainChannel)
	if channel == nil {
		// just exit early if we are not on the channel somehow
		return nil
	}
	// create a map of all the users so we get simpler and faster lookups
	users := make(map[string]struct{}, len(channel.UserList))
	for _, name := range channel.UserList {
		users[name] = struct{}{}
	}

	// we want to send as few NOTICEs as possible, so send to server MAXTARGETS at a time
	var maxtargets = 1
	{
		max, ok := b.c.GetServerOption("MAXTARGETS")
		if ok {
			maxi, err := strconv.Atoi(max)
			if err == nil {
				maxtargets = maxi
			}
		}
	}

	// see below AddTmp handler to see why we need this
	//
	// targetMapping is a map between every second nick in each chunk to third-onwards
	// nicks in each chunk; this is so we can find them again when a 407 is send to us
	targetMapping := map[string][]string{}

	// we can send a notice to up to `maxtargets` targets at once, so start by grouping the
	// nicknames by `maxtargets`, while checking if they're in the channel
	var targets []string
	var chunk = make([]string, 0, maxtargets)
	for _, name := range usersWithFave {
		// check if the user is in the channel
		if _, ok := users[name]; !ok {
			continue
		}

		chunk = append(chunk, name)
		if len(chunk) == maxtargets {
			if len(chunk) > 2 {
				// map second nickname onwards to the rest
				targetMapping[chunk[1]] = chunk[2:]
			}
			targets = append(targets, strings.Join(chunk, ","))
			chunk = chunk[:0]
		}
	}
	// handle leftovers in the last chunk
	if len(chunk) > 0 {
		if len(chunk) > 2 {
			// map second nickname onwards to the rest
			targetMapping[chunk[1]] = chunk[2:]
		}
		targets = append(targets, strings.Join(chunk, ","))
	}

	message = Fmt("Fave: %s is playing.", status.Song.Metadata)

	// our main network, rizon lies to us about MAXTARGETS until a certain period of
	// time has passed since you connected, so we might get an ERR_TOOMANYTARGETS when
	// sending notices, this handler resends any that come back as single-target.
	b.c.Handlers.AddTmp("407", time.Second*10, func(c *girc.Client, e girc.Event) bool {
		target := e.Params[len(e.Params)-1]
		c.Cmd.Notice(target, message)
		for _, target = range targetMapping[target] {
			c.Cmd.Notice(target, message)
		}
		return false
	})

	// now send out the notices, we do this in another goroutine because it might
	// take a while to send all messages
	go func(message string) {
		for _, chunk := range targets {
			b.c.Cmd.Notice(chunk, message)
		}
	}(message)

	return nil
}

func (b *Bot) AnnounceRequest(ctx context.Context, song radio.Song) error {
	message := "Requested:{red} '%s'"

	// Get queue from streamer
	songQueue, err := b.Streamer.Queue(ctx);
	if err != nil {
		return err
	}

	// Search for the song in the queue, -1 means not found by default
	songPos:= -1
	for i, qs := range songQueue {
		if qs.ID == song.ID {
			songPos = i
			break
		}
	}

	// If song is queued, change message with remaining time to start
	if songPos > -1 {
		// Calculate the remaining time until song start
		var startTimeDiff time.Duration
		if !songQueue[songPos].ExpectedStartTime.IsZero() {
			startTimeDiff = time.Until(songQueue[songPos].ExpectedStartTime)
		}

		// Append new info to message
		message = Fmt(message + " (%s)",
			song.Metadata,
			FormatDayDuration(startTimeDiff),
		)
	} else {
		message = Fmt(message, song.Metadata)
	}

	// Announce to the channel the request
	b.c.Cmd.Message(b.Conf().IRC.MainChannel, message)

	// All done!
	return nil
}
