// +build !nostreamer

package main

import (
	"context"

	"github.com/R-a-dio/valkyrie/config"
	"github.com/google/subcommands"
)

func init() {
	subcommands.Register(streamerCmd, "")
}

var streamerCmd = cmd{
	name:     "streamer",
	synopsis: "Streams to a configured icecast server.",
	usage: `streamer:
	Streams to a configured icecast server.
	`,
	execute: executeStreamer,
}

func executeStreamer(ctx context.Context, cfg config.Config) error {
	return nil
}
