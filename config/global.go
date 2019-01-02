package config

import (
	"fmt"
	"io"
	"net/http"
	"sync/atomic"
	"time"

	"github.com/BurntSushi/toml"
)

// DefaultGlobal contains the default values for the global configuration
var DefaultGlobal = Global{
	UserAgent:        "hanyuu/2.0",
	UserRequestDelay: time.Hour * 1,
	TemplateDir:      "templates/",

	Database: DefaultDatabase,
	Streamer: DefaultStreamer,
	IRC:      DefaultIRC,
	Manager:  DefaultManager,
}

// Global represents a full configuration file of this project, each tool part
// of this repository share the same configuration file
type Global struct {
	// UserAgent to use when making HTTP requests
	UserAgent string
	// MusicPath is the prefix of music files in the database
	MusicPath string
	// UserRequestDelay is the delay between user requests
	UserRequestDelay time.Duration
	// TemplateDir is the path where html templates are stored for the HTTP
	// frontends
	TemplateDir string
	// Database contains the configuration to connect to the SQL database
	Database Database

	// Fields below are part of components
	Streamer Streamer
	IRC      IRC
	Manager  Manager
}

// Load loads a configuration from the reader given
func Load(r io.Reader) (Global, error) {
	var c = DefaultGlobal
	m, err := toml.DecodeReader(r, &c)
	if err != nil {
		return c, err
	}

	undec := m.Undecoded()
	if len(undec) > 0 {
		fmt.Println(undec)
	}

	return c, nil
}

// LoadAtomic is equal to calling Load and Atomic in order
func LoadAtomic(r io.Reader) (AtomicGlobal, error) {
	c, err := Load(r)
	if err != nil {
		return AtomicGlobal{}, nil
	}

	return Atomic(c), nil
}

// AtomicGlobal is a type-safe wrapper around an atomicically accessed Global
// configuration value
type AtomicGlobal struct {
	config *atomic.Value
}

// Atomic converts a Global into an AtomicGlobal
func Atomic(g Global) AtomicGlobal {
	var ag AtomicGlobal
	ag.config = new(atomic.Value)
	ag.config.Store(g)
	return ag
}

// Conf returns the global configuration stored
func (ag AtomicGlobal) Conf() Global {
	return ag.config.Load().(Global)
}

// StoreConf stores a new global configuration
func (ag AtomicGlobal) StoreConf(g Global) {
	ag.config.Store(g)
}

// PrepareTwirpClient prepares a HTTP client and an usable address string for creating
// a twirp client
func PrepareTwirpClient(addr string) (fullAddr string, client HTTPClient) {
	// TODO: check if we want to implement our own HTTPClient
	client = http.DefaultClient

	// our addr can either be 'ip:port' or ':port' but twirp expects http(s)://ip:port
	if len(addr) == 0 {
		panic("invalid address passed to PrepareTwirpClient: empty string")
	}

	if addr[0] == ':' {
		fullAddr = "http://localhost" + addr
	} else {
		fullAddr = "http://" + addr
	}

	return fullAddr, client
}

// HTTPClient interface used by twirp to fulfill requests
type HTTPClient interface {
	Do(req *http.Request) (*http.Response, error)
}
