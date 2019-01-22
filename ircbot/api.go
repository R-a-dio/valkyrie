package ircbot

import (
	"context"
	"net/http"
	"net/http/pprof"
	"strings"
	"time"

	radio "github.com/R-a-dio/valkyrie"
	"github.com/R-a-dio/valkyrie/database"
	"github.com/R-a-dio/valkyrie/rpc"
)

func NewHTTPServer(b *Bot) (*http.Server, error) {
	rpcServer := rpc.NewBotServer(b, nil)
	mux := http.NewServeMux()
	// rpc server path
	mux.Handle(rpc.BotPathPrefix, rpcServer)

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

func (b *Bot) AnnounceSong(ctx context.Context, a *rpc.SongAnnouncement) (*rpc.Null, error) {
	message := "Now starting:{red} '%s' {clear}[%s/%s](%s), %s, %s, {green}LP:{clear} %s"

	var lastPlayedDiff time.Duration
	if a.Song.LastPlayed != 0 {
		lastPlayed := time.Unix(a.Song.LastPlayed, 0)
		lastPlayedDiff = time.Since(lastPlayed)
	}

	var songPosition time.Duration
	var songLength time.Duration

	{
		start := time.Unix(a.Song.StartTime, 0)
		end := time.Unix(a.Song.EndTime, 0)

		songPosition = time.Since(start)
		songLength = end.Sub(start)
	}

	db := database.Handle(ctx, b.DB)
	song := radio.Song{ID: radio.SongID(a.Song.Id)}

	favoriteCount, _ := database.SongFaveCount(db, song)
	playedCount, _ := database.SongPlayedCount(db, song)

	message = Fmt(message,
		a.Song.Metadata,
		FormatPlaybackDuration(songPosition), FormatPlaybackDuration(songLength),
		Pluralf("%d listeners", int64(a.Listeners)),
		Pluralf("%d faves", favoriteCount),
		Pluralf("played %d times", playedCount),
		FormatLongDuration(lastPlayedDiff),
	)

	b.c.Cmd.Message(b.Conf().IRC.MainChannel, message)

	if favoriteCount == 0 {
		// save ourselves some work if there are no favorites
		return new(rpc.Null), nil
	}
	usersWithFave, err := database.GetSongFavorites(db, song.ID)
	if err != nil {
		return nil, err
	}

	// we only send notifications to people that are on the configured main channel
	channel := b.c.LookupChannel(b.Conf().IRC.MainChannel)
	if channel == nil {
		// just exit early if we are not on the channel somehow
		return new(rpc.Null), nil
	}
	// create a map of all the users so we get simpler and faster lookups
	users := make(map[string]struct{}, len(channel.UserList))
	for _, name := range channel.UserList {
		users[name] = struct{}{}
	}

	// we can send a notice to up to 4 targets at once, so start by grouping the
	// nicknames by 4, while checking if they're in the channel
	var targets []string
	var chunk = make([]string, 0, 4)
	for _, name := range usersWithFave {
		// check if the user is in the channel
		if _, ok := users[name]; !ok {
			continue
		}

		chunk = append(chunk, name)
		if len(chunk) == 4 {
			targets = append(targets, strings.Join(chunk, ","))
			chunk = chunk[:0]
		}
	}
	// handle leftovers in the last chunk
	if len(chunk) > 0 {
		targets = append(targets, strings.Join(chunk, ","))
	}

	// now send out the notices, we do this in another goroutine because it might
	// take a while to send all messages
	go func(metadata string) {
		message := "Fave: %s is playing."
		for _, chunk := range targets {
			b.c.Cmd.Noticef(chunk, message, metadata)
		}
	}(a.Song.Metadata)

	return new(rpc.Null), nil
}
