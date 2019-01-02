package config

import "github.com/R-a-dio/valkyrie/rpc/manager"

// DefaultManager contains the default values of the manager configuration
var DefaultManager = Manager{
	Addr: ":4646",
}

// Manager contains all fields relevant to the manager
type Manager struct {
	// Addr is the address for the HTTP API
	Addr string
	// StreamURL is the url to listen to the mp3 stream
	StreamURL string

	FallbackNames []string
}

func (m Manager) TwirpClient() manager.Manager {
	addr, client := PrepareTwirpClient(m.Addr)
	return manager.NewManagerProtobufClient(addr, client)
}
