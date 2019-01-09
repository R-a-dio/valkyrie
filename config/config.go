package config

import (
	"net/http"
	"time"

	rpcirc "github.com/R-a-dio/valkyrie/rpc/irc"
	rpcmanager "github.com/R-a-dio/valkyrie/rpc/manager"
	rpcstreamer "github.com/R-a-dio/valkyrie/rpc/streamer"
)

// defaultConfig is the default configuration for this project
var defaultConfig = config{
	UserAgent:        "hanyuu/2.0",
	UserRequestDelay: time.Hour * 1,
	TemplatePath:     "templates/",
	MusicPath:        "",
	Database: database{
		DriverName: "mysql",
		DSN:        "",
	},
	Streamer: streamer{
		Addr:            ":4545",
		StreamURL:       "",
		RequestsEnabled: true,
	},
	IRC: irc{
		Addr:       ":4444",
		AllowFlood: false,
	},
	Manager: manager{
		Addr:          ":4646",
		StreamURL:     "",
		FallbackNames: []string{"fallback"},
	},
}

// config represents a full configuration file of this project, each tool part
// of this repository share the same configuration file
type config struct {
	// UserAgent to use when making HTTP requests
	UserAgent string
	// MusicPath is the prefix of music files in the database
	MusicPath string
	// UserRequestDelay is the delay between user requests
	UserRequestDelay time.Duration
	// TemplatePath is the path where html templates are stored for the HTTP
	// frontends
	TemplatePath string
	// Database contains the configuration to connect to the SQL database
	Database database

	Streamer streamer
	IRC      irc
	Manager  manager
}

// database is the configuration for the database/sql package
type database struct {
	// DriverName to pass to database/sql
	DriverName string
	// DSN to pass to database/sql, format depends on driver used
	DSN string
}

// streamer contains all the fields only relevant to the streamer
type streamer struct {
	// Addr is the address for the HTTP API
	Addr string
	// StreamURL is the full URL to the streamer endpoint, including any
	// authorization parameters required to connect.
	StreamURL string
	// RequestsEnabled indicates if requests are enabled currently
	RequestsEnabled bool
}

// TwirpClient returns an usable twirp client for the streamer
func (s streamer) TwirpClient() rpcstreamer.Streamer {
	return rpcstreamer.NewStreamerProtobufClient(prepareTwirpClient(s.Addr))
}

// irc contains all the fields only relevant to the irc bot
type irc struct {
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
func (i irc) TwirpClient() rpcirc.Bot {
	return rpcirc.NewBotProtobufClient(prepareTwirpClient(i.Addr))
}

// manager contains all fields relevant to the manager
type manager struct {
	// Addr is the address for the HTTP API
	Addr string
	// StreamURL is the url to listen to the mp3 stream
	StreamURL string
	// FallbackNames is a list of strings that indicate an icecast stream is playing a
	// fallback stream
	FallbackNames []string
}

func (m manager) TwirpClient() rpcmanager.Manager {
	return rpcmanager.NewManagerProtobufClient(prepareTwirpClient(m.Addr))
}

// prepareTwirpClient prepares a http client and an usable address string for creating
// a twirp client
func prepareTwirpClient(addr string) (fullAddr string, client httpClient) {
	// TODO: check if we want to configure our own http client
	client = http.DefaultClient

	// our addr can either be 'ip:port' or ':port' but twirp expects http(s)://ip:port
	if len(addr) == 0 {
		panic("invalid address passed to prepareTwirpClient: empty string")
	}

	if addr[0] == ':' {
		fullAddr = "http://localhost" + addr
	} else {
		fullAddr = "http://" + addr
	}

	return fullAddr, client
}

// httpClient interface used by twirp to fulfill requests
type httpClient interface {
	Do(req *http.Request) (*http.Response, error)
}
