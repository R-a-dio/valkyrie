// +build !nostreamer

package main

import (
	"github.com/google/subcommands"

	"github.com/R-a-dio/valkyrie/streamer"
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
	execute: streamer.Execute,
}
