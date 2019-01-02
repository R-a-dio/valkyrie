package config

import "github.com/R-a-dio/valkyrie/rpc/manager"

// DefaultStatus contains the default values of the manager configuration
var DefaultStatus = Status{
	Addr: ":4646",
}

// Status contains all fields relevant to the manager
type Status struct {
	// Addr is the address for the HTTP API
	Addr string
	// StreamURL is the url to listen to the mp3 stream
	StreamURL string

	FallbackNames []string
}

func (s Status) TwirpClient() manager.Manager {
	addr, client := PrepareTwirpClient(s.Addr)
	return manager.NewManagerProtobufClient(addr, client)
}
