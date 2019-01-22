package ircbot

import (
	"context"
	"net/http"
	"net/http/pprof"
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

	// TODO: send fave notifications
	return new(rpc.Null), nil
}
