package config

import (
	crand "crypto/rand"
	"io"
	"log"
	"math"
	"math/big"
	"math/rand"
	"net/url"
	"os"
	"sync"
	"sync/atomic"
	"time"

	"github.com/BurntSushi/toml"
	radio "github.com/R-a-dio/valkyrie"
	"github.com/R-a-dio/valkyrie/rpc"
	"google.golang.org/grpc"
)

// defaultConfig is the default configuration for this project
var defaultConfig = config{
	UserAgent:        "hanyuu/2.0",
	UserRequestDelay: Duration(time.Hour * 1),
	UserUploadDelay:  Duration(time.Hour * 2),
	TemplatePath:     "templates/",
	MusicPath:        "/radio/music",
	AssetsPath:       "./assets/",
	Providers: providers{
		Storage: "mariadb",
		Search:  "storage",
	},
	Database: database{
		DriverName: "mysql",
		DSN:        "radio@unix(/run/mysqld/mysqld.sock)/radio?parseTime=true",
	},
	Website: website{
		WebsiteAddr:     "localhost:3241",
		Addr:            ":4747",
		ListenAddr:      ":4747",
		DJImageMaxSize:  10 * 1024 * 1024,
		DJImagePath:     "/radio/dj-images",
		PublicStreamURL: "http://localhost:8000/main.mp3",
	},
	Streamer: streamer{
		Addr:            ":4545",
		ListenAddr:      ":4545",
		StreamURL:       "",
		RequestsEnabled: true,
		ConnectTimeout:  Duration(time.Second * 30),
	},
	IRC: irc{
		Addr:           ":4444",
		ListenAddr:     ":4444",
		AllowFlood:     false,
		EnableEcho:     true,
		AnnouncePeriod: Duration(time.Second * 15),
	},
	Manager: manager{
		Addr:          ":4646",
		ListenAddr:    ":4646",
		StreamURL:     "",
		FallbackNames: []string{"fallback"},
	},
	Elastic: elasticsearch{
		URL: "http://127.0.0.1:9200/",
	},
	Balancer: balancer{
		Addr:     "127.0.0.1:4848",
		Fallback: "https://relay0.r-a-d.io/main.mp3",
	},
	Proxy: proxy{
		Addr:             ":1337",
		MasterServer:     "http://source:hackme@127.0.0.1:8000",
		PrimaryMountName: "/main.mp3",
	},
	Tracker: tracker{
		RPCAddr:        ":4949",
		ListenAddr:     ":9999",
		MasterServer:   "http://127.0.0.1:8000",
		MasterUsername: "admin",
		MasterPassword: "hackme",
		MountName:      "/main.mp3",
	},
	Telemetry: telemetry{
		Use:                false,
		Endpoint:           ":4317",
		PrometheusEndpoint: "localhost:9091",
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
	UserRequestDelay Duration
	// UserUploadDelay is the delay between song submissions
	UserUploadDelay Duration
	// TemplatePath is the path where html templates are stored for the HTTP
	// frontends
	TemplatePath string
	// AssetsPath is the path where assets are loaded from. The files in this
	// directory are simply served from the file system.
	AssetsPath string

	Providers providers
	Database  database

	Website  website
	Streamer streamer
	IRC      irc
	Manager  manager
	Elastic  elasticsearch
	Balancer balancer
	Proxy    proxy
	Tracker  tracker

	Telemetry telemetry
}

type tracker struct {
	// RPCAddr is the address to use for RPC client connections to this
	// component or the listening address for the RPC server
	RPCAddr string
	// ListenAddr is the address the http endpoint will be listening on
	// this would be the one you use in icecast config
	ListenAddr string
	// MasterServer is the address of the master icecast server
	MasterServer URL
	// MasterUsername is the admin username for the master icecast
	MasterUsername string
	// MasterPassword is the admin password for the master icecast
	MasterPassword string
	MountName      string
}

type proxy struct {
	Addr             string
	MasterServer     URL
	PrimaryMountName string
}

type telemetry struct {
	Use                bool
	Endpoint           string
	Auth               string
	PrometheusEndpoint string
}

type providers struct {
	// Storage is the name of the StorageService provider to use
	Storage string
	// Search is the name of the SearchService provider to use
	Search string
}

// database is the configuration for the database/sql package
type database struct {
	// DriverName to pass to database/sql
	DriverName string
	// DSN to pass to database/sql, format depends on driver used
	DSN string
}

// website contains configuration relevant to the website instance
type website struct {
	// Address to bind to for the public-facing website
	WebsiteAddr string
	// Addr is the address for the HTTP API
	Addr string
	// ListenAddr is the address to listen on for the HTTP API
	ListenAddr string
	// DJImageMaxSize is the maximum size of dj images in bytes
	DJImageMaxSize int64
	// DJImagePath is the path where to store dj images
	DJImagePath string // required
	// PublicStreamURL is the public url to the stream
	PublicStreamURL string
}

// streamer contains all the fields only relevant to the streamer
type streamer struct {
	// Addr is the address for the HTTP API
	Addr string
	// ListenAddr is the address to listen on for the HTTP API
	ListenAddr string
	// StreamURL is the full URL to the streamer endpoint, including any
	// authorization parameters required to connect.
	StreamURL URL
	// RequestsEnabled indicates if requests are enabled currently
	RequestsEnabled bool
	// ConnectTimeout is how long to wait before connecting if the
	// proxy has no streamer. Set to 0 to disable
	ConnectTimeout Duration
}

// irc contains all the fields only relevant to the irc bot
type irc struct {
	// Addr is the address for the HTTP API
	Addr string
	// ListenAddr is the address to listen on for the HTTP API
	ListenAddr string
	// BindAddr is the address to bind to when connecting to IRC, this has to resolve
	// to an IPv4/IPv6 address bindable on the system.
	BindAddr string
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
	// EnableEcho allows you to enable/disable IRC messages output
	EnableEcho bool
	// AnnouncePeriod is the amount of time that is required between two announcements
	AnnouncePeriod Duration
}

// manager contains all fields relevant to the manager
type manager struct {
	// Addr is the address for the HTTP API
	Addr string
	// ListenAddr is the address to listen on for the HTTP API
	ListenAddr string
	// StreamURL is the url to listen to the mp3 stream
	StreamURL string
	// FallbackNames is a list of strings that indicate an icecast stream is playing a
	// fallback stream
	FallbackNames []string
}

type elasticsearch struct {
	URL string
}

// balancer contains fields for the load balancer.
type balancer struct {
	// Addr is the public facing address for the balancer.
	Addr string
	// Fallback is the stream to default to.
	Fallback string
}

// errors is a slice of multiple config-file errors
type errors []error

func (e errors) Error() string {
	s := "config: error opening files:"
	if len(e) == 1 {
		return s + " " + e[0].Error()
	}

	for _, err := range e {
		s += "\n" + err.Error()
	}

	return s
}

// Config is a type-safe wrapper around the config type
type Config struct {
	config *atomic.Value

	reloader *reload

	// RPC helpers, call these to get an RPC interface to
	// the named component
	Streamer radio.StreamerService
	Manager  radio.ManagerService
	Tracker  radio.ListenerTrackerService
	Queue    radio.QueueService
	IRC      radio.AnnounceService
}

type reload struct {
	sync.RWMutex
	callbacks []func()
}

func newConfig(c config) Config {
	cfg := Config{
		config:   new(atomic.Value),
		reloader: new(reload),
	}

	streamerConn := Value(cfg, func(c Config) *grpc.ClientConn {
		return rpc.PrepareConn(cfg.Conf().Streamer.Addr)
	})

	// TODO: handle reloads by closing rpc connections
	cfg.Streamer = newStreamerService(cfg, streamerConn)
	cfg.Manager = newManagerService(cfg)
	cfg.Tracker = newTrackerService(cfg)
	cfg.Queue = newQueueService(cfg, streamerConn)
	cfg.IRC = newIRCService(cfg)

	cfg.StoreConf(c)
	return cfg
}

// TestConfig returns default config with RPC services disabled
func TestConfig() Config {
	cfg, err := LoadFile()
	if err != nil {
		panic("failed to load config: " + err.Error())
	}

	cfg.Streamer = nil
	cfg.Manager = nil
	cfg.Tracker = nil
	cfg.Queue = nil
	cfg.IRC = nil

	c := cfg.Conf()
	c.Database.DSN = ""
	cfg.StoreConf(c)
	return cfg
}

// Loader is a typed function that returns a Config, used to pass in a pre-set Load or
// LoadFile call from a closure
type Loader func() (Config, error)

// LoadFile loads a configuration file from the filename given
func LoadFile(filenames ...string) (Config, error) {
	var f *os.File
	var err error
	var errs errors

	for _, filename := range filenames {
		if filename == "" {
			// just skip empty filenames to not clutter errors returned
			continue
		}

		f, err = os.Open(filename)
		if err == nil {
			break
		}

		errs = append(errs, err)
	}

	if f == nil {
		if len(errs) > 0 {
			err = errs
		}
		return newConfig(defaultConfig), err
	}
	defer f.Close()

	return Load(f)
}

// Load loads a configuration file from the reader given, it expects TOML as input
func Load(r io.Reader) (Config, error) {
	var c = defaultConfig
	m, err := toml.NewDecoder(r).Decode(&c)
	if err != nil {
		return newConfig(defaultConfig), err
	}

	// print out keys that were found but don't have a destination
	for _, key := range m.Undecoded() {
		log.Printf("warning: unknown configuration field: %s", key)
	}

	return newConfig(c), nil
}

func (c Config) LoadAndUpdate(filenames ...string) error {
	conf, err := LoadFile(filenames...)
	if err != nil {
		return err
	}

	c.StoreConf(conf.Conf())
	return nil
}

func (c *Config) OnReload(cb func()) {
	c.reloader.Lock()
	c.reloader.callbacks = append(c.reloader.callbacks, cb)
	c.reloader.Unlock()
}

func (c Config) TriggerReload() {
	c.reloader.RLock()
	defer c.reloader.RUnlock()
	for _, fn := range c.reloader.callbacks {
		fn()
	}
}

func Value[T any](cfg Config, fn func(Config) T) func() T {
	var store atomic.Pointer[func() T]

	cfg.OnReload(func() {
		store.Store(nil)
	})

	return func() T {
		gp := store.Load()
		if gp == nil || *gp == nil {
			g := sync.OnceValue(func() T {
				return fn(cfg)
			})
			gp = &g
			store.Store(gp)
		}
		return (*gp)()
	}
}

func Values[T1, T2 any](cfg Config, fn func(Config) (T1, T2)) func() (T1, T2) {
	var store atomic.Pointer[func() (T1, T2)]

	cfg.OnReload(func() {
		store.Store(nil)
	})

	return func() (T1, T2) {
		gp := store.Load()
		if gp == nil || *gp == nil {
			g := sync.OnceValues(func() (T1, T2) {
				return fn(cfg)
			})
			gp = &g
			store.Store(gp)
		}
		return (*gp)()
	}
}

// Conf returns the configuration stored inside
//
// NOTE: Conf returns a shallow-copy of the config value stored inside; so do not edit
//
//	any slices or maps that might be inside
func (c Config) Conf() config {
	return c.config.Load().(config)
}

// StoreConf stores the configuration passed
func (c Config) StoreConf(new config) {
	c.config.Store(new)
}

// Save writes the configuration to w in TOML format
func (c Config) Save(w io.Writer) error {
	return toml.NewEncoder(w).Encode(c.Conf())
}

// Duration is a time.Duration that supports Text(Un)Marshaler
type Duration time.Duration

// MarshalText implements encoding.TextMarshaler
func (d Duration) MarshalText() ([]byte, error) {
	return []byte(time.Duration(d).String()), nil
}

// UnmarshalText implements encoding.TextUnmarshaler
func (d *Duration) UnmarshalText(text []byte) error {
	n, err := time.ParseDuration(string(text))
	*d = Duration(n)
	return err
}

type URL string

func (u URL) URL() *url.URL {
	// any file-based configuration will go through UnmarshalText
	// for the URL type, and the defaults are tested in the
	// roundtrip test, so there should be no way for a URL value
	// to be a string that doesn't url.Parse correctly.
	uri, err := url.Parse(string(u))
	if err != nil {
		panic("unreachable: unless you did something stupid")
	}
	return uri
}

func (u URL) MarshalText() ([]byte, error) {
	return []byte(u), nil
}

func (u *URL) UnmarshalText(text []byte) error {
	_, err := url.Parse(string(text))
	if err != nil {
		return err
	}
	*u = URL(text)
	return nil
}

// NewRand returns a fresh *rand.Rand seeded with either a crypto random seed or the
// current time if that fails to succeed
func NewRand(lock bool) *rand.Rand {
	var seed int64

	max := big.NewInt(math.MaxInt64)
	n, err := crand.Int(crand.Reader, max)
	if err != nil {
		seed = time.Now().UnixNano()
	} else {
		seed = n.Int64()
	}

	src := rand.NewSource(seed)
	if lock {
		// wrap our source in a lock if the caller didn't specifically ask for no lock
		if src64, ok := src.(rand.Source64); ok {
			src = &lockedSource{src: src64}
		} else {
			panic("source returned by NewSource does not implement Source64")
		}
	}
	return rand.New(src)
}

type lockedSource struct {
	mu  sync.Mutex
	src rand.Source64
}

func (ls *lockedSource) Int63() int64 {
	ls.mu.Lock()
	n := ls.src.Int63()
	ls.mu.Unlock()
	return n
}

func (ls *lockedSource) Seed(seed int64) {
	ls.mu.Lock()
	ls.src.Seed(seed)
	ls.mu.Unlock()
}

func (ls *lockedSource) Uint64() uint64 {
	ls.mu.Lock()
	n := ls.src.Uint64()
	ls.mu.Unlock()
	return n
}
