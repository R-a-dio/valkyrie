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
	synopsis: "streams to a configured icecast server",
	usage: `streamer:
	streams to a configured icecast server
	`,
	execute: withConfig(streamer.Execute),
}
