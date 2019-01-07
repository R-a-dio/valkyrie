package config

import "github.com/R-a-dio/valkyrie/rpc/irc"

// DefaultIRC contains the default values of the irc bot configuration
var DefaultIRC = IRC{
	Addr: ":4444",
}

// IRC contains all the fields only relevant to the irc bot
type IRC struct {
	// Addr is the address for the HTTP API
	Addr string
	// Server is the address of the irc server to connect to
	Server string
	// Nick is the nickname to use
	Nick string
	// NickPassword is the nickserv password if any
	NickPassword string
	// Channels is the channels to join
	Channels []string
	// MainChannel is the channel for announceing songs
	MainChannel string
	// AllowFlood determines if flood protection is off or on
	AllowFlood bool
}

// TwirpClient returns an usable twirp client for the irc bot
func (i IRC) TwirpClient() irc.Bot {
	addr, client := PrepareTwirpClient(i.Addr)
	return irc.NewBotProtobufClient(addr, client)
}
