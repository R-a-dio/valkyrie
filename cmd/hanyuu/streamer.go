// +build !nostreamer

package main

import "github.com/R-a-dio/valkyrie/streamer"

func init() {
	AddComponent("streamer", streamer.Component)
}
