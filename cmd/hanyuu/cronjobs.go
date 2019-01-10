package main

import (
	"context"

	"github.com/R-a-dio/valkyrie/config"
)

var listenerLogCmd = cmd{
	name:     "listenerlog",
	synopsis: "log listener count to database.",
	usage: `listenerlog:
	log listener count to database.
	`,
	execute: executeListenerLog,
}

func executeListenerLog(context.Context, config.Config) error {
	return nil
}

var requestCountCmd = cmd{
	name:     "requestcount",
	synopsis: "reduce request counter in database.",
	usage: `requestcount:
	reduce request counter in database.
	`,
	execute: executeRequestCount,
}

func executeRequestCount(context.Context, config.Config) error {
	return nil
}
