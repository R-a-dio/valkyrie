package main

import "github.com/R-a-dio/valkyrie/ircbot"

func init() {
	AddComponent("irc", ircbot.Component)
}
