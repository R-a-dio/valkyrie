// +build !nostreamer

package main

import (
	"github.com/google/subcommands"

	"github.com/R-a-dio/valkyrie/jobs"
	"github.com/R-a-dio/valkyrie/streamer"
)

func init() {
	subcommands.Register(streamerCmd, "")
	subcommands.Register(verifierCmd, "jobs")
}

var streamerCmd = cmd{
	name:     "streamer",
	synopsis: "streams to a configured icecast server",
	usage: `streamer:
	streams to a configured icecast server
	`,
	execute: withConfig(streamer.Execute),
}

var verifierCmd = cmd{
	name:     "verifier",
	synopsis: "verifies that tracks marked unusable can be decoded with ffmpeg",
	usage: `verifier:
	verifies that all tracks marked with usable=0 can be decoded with ffmpeg
	and marks them with usable=1 if it succeeds
	`,
	execute: withConfig(jobs.ExecuteVerifier),
}
