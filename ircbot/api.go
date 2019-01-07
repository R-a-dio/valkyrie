package ircbot

import (
	"context"
	"net/http"
	"net/http/pprof"
	"strings"

	pb "github.com/R-a-dio/valkyrie/rpc/irc"
	"github.com/R-a-dio/valkyrie/rpc/manager"
	"github.com/twitchtv/twirp"
)

func NewHTTPServer(b *Bot) (*http.Server, error) {
	rpcServer := pb.NewBotServer(b, nil)
	mux := http.NewServeMux()
	// rpc server path
	mux.Handle(pb.BotPathPrefix, rpcServer)

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

func (b *Bot) AnnounceSong(ctx context.Context, song *manager.Song) (*pb.Null, error) {
	e := Event{
		bot: b,
		c:   b.c,
	}

	fn, err := NowPlayingMessage(e)
	if err != nil {
		return nil, twirp.InternalErrorWith(err)
	}

	message, args, err := fn()
	if err != nil {
		return nil, twirp.InternalErrorWith(err)
	}

	// only difference between the announce and .np command is that it
	// starts with "Now starting" instead of "Now playing"
	message = strings.Replace(message, "playing", "starting", 1)

	b.c.Cmd.Message(b.Conf().IRC.MainChannel, Fmt(message, args...))
	return new(pb.Null), nil
}
