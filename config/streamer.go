package config

import (
	"github.com/R-a-dio/valkyrie/rpc/streamer"
)

// DefaultStreamer contains the default values of the streamer configuration
var DefaultStreamer = Streamer{
	Addr: ":4545",
}

// Streamer contains all the fields only relevant to the streamer
type Streamer struct {
	// Addr is the address for the HTTP API
	Addr string
	// StreamURL is the full URL to the streamer endpoint, including any
	// authorization parameters required to connect.
	StreamURL string
	// RequestsEnabled indicates if requests are enabled currently
	RequestsEnabled bool
}

// TwirpClient returns an usable twirp client for the streamer
func (s Streamer) TwirpClient() streamer.Streamer {
	addr, client := PrepareTwirpClient(s.Addr)
	return streamer.NewStreamerProtobufClient(addr, client)
}
