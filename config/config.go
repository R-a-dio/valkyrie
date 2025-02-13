package config

import (
	"io"
	"log"
	"os"
	"sync"
	"sync/atomic"

	"github.com/BurntSushi/toml"
	radio "github.com/R-a-dio/valkyrie"
)

// config represents a full configuration file of this project, each tool part
// of this repository share the same configuration file
type config struct {
	DevelopmentMode bool
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
	Search   search
	Balancer balancer
	Proxy    proxy
	Tracker  tracker

	Telemetry telemetry

	// tunein.com scrobbling configuration
	Tunein tunein

	Metadata []metadata
}

type tracker struct {
	// RPCAddr is the address to use for RPC client connections to this
	// component or the listening address for the RPC server
	RPCAddr AddrPort
	// ListenAddr is the address the http endpoint will be listening on
	// this would be the one you use in icecast config
	ListenAddr AddrPort
	// MasterServer is the address of the master icecast server
	MasterServer URL
	// MasterUsername is the admin username for the master icecast
	MasterUsername string
	// MasterPassword is the admin password for the master icecast
	MasterPassword string
	// PrimaryMountName is the mountname to keep track of, for example "/main.mp3"
	PrimaryMountName string
}

type proxy struct {
	// RPCAddr is the address to use for the RPC API server
	RPCAddr AddrPort
	// ListenAddr is the address to use for the proxy http server
	ListenAddr AddrPort
	// MasterServer is the icecast master server URL
	MasterServer URL
	// MasterUsername is the username for the master icecast
	MasterUsername string
	// MasterPassword is the password for the master icecast
	MasterPassword string
	// PrimaryMountName is the mountname to propagate all events for
	PrimaryMountName string

	// IcecastDescription is the description to send to icecast for the streams
	IcecastDescription string
	// IcecastName is the name to send to icecast for the streams
	IcecastName string
}

type telemetry struct {
	// Use disables telemetry when false
	Use bool
	// Endpoint is the endpoint that the opentelemetry collector is on
	Endpoint string
	// Auth is the value send in the Authorization header
	Auth string
	// PrometheusEndpoint is the endpoint that prometheus remotewrite is on
	PrometheusEndpoint string

	// StandaloneProxy lets you run the telemetry reverse-proxy as a standalone
	// service instead of being embedded in the website service
	StandaloneProxy struct {
		// Enabled enables or disables the standalone version, this is only used
		// to update the redirect in the website service, you're responsible for
		// running the service somehow
		Enabled bool
		// URL is the accessable url that /admin/telemetry should redirect to
		URL URL
		// ListenAddr is where the reverse-proxy should be listening
		ListenAddr AddrPort
	}

	// Pyroscope contains configuration values for the pyroscope support
	Pyroscope struct {
		// Endpoint is where the pyroscope instance lives
		Endpoint URL
		// UploadRate is how often pyroscope should collect data
		UploadRate Duration
	}
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
	// WebsiteAddr is the address to bind to for the public-facing website
	WebsiteAddr AddrPort
	// DJImageMaxSize is the maximum size of dj images in bytes
	DJImageMaxSize int64
	// DJImagePath is the path where to store dj images
	DJImagePath string // required
	// PublicStreamURL is the public url to the stream
	PublicStreamURL string
	// CSRFSecret is the key used to generate csrf tokens, should be secret
	CSRFSecret string

	// AkismetKey is the anti-spam API key to akismet
	AkismetKey string
	// AkismetBlog is the string to send as the blog value in the akismet api
	AkismetBlog string

	// AdminMonitoringURL is the url to proxy admin/telemetry/ requests to
	AdminMonitoringURL URL
	// AdminMonitoringUserHeader is the header to use for passing in the username
	AdminMonitoringUserHeader string
	AdminMonitoringRoleHeader string
}

// streamer contains all the fields only relevant to the streamer
type streamer struct {
	// RPCAddr is the address for the RPC API
	RPCAddr AddrPort
	// StreamURL is the URL to the streamer endpoint
	StreamURL URL
	// StreamUsername is the username to login with
	StreamUsername string
	// StreamPassword is the password for the username above
	StreamPassword string
	// RequestsEnabled indicates if requests are enabled currently
	RequestsEnabled bool
	// ConnectTimeout is how long to wait before connecting if the
	// proxy has no streamer. Set to 0 to disable
	ConnectTimeout Duration
}

// irc contains all the fields only relevant to the irc bot
type irc struct {
	// RPCAddr is the address for the RPC API
	RPCAddr AddrPort
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
	// RPCAddr is the address for the RPC API
	RPCAddr AddrPort
	// FallbackNames is a list of strings that indicate an icecast stream is playing a
	// fallback stream
	FallbackNames []string
	// GuestProxyAddr is the address guests should connect to to stream to icecast
	GuestProxyAddr URL
	// GuestAuthPeriod is how long a guest will be authorized to do things as a guest
	GuestAuthPeriod Duration
}

type tunein struct {
	// Enabled indicates of tunein scrobbling is enabled
	Enabled bool
	// Endpint to send updates to
	Endpoint string
	// StationID is the station id from tunein
	StationID string
	// PartnerID is the partner id from tunein
	PartnerID string
	// Key is the api key to access the tunein api
	Key string
}

type search struct {
	Endpoint  URL
	IndexPath string
}

// balancer contains fields for the load balancer.
type balancer struct {
	// Addr is the public facing address for the balancer.
	Addr string
	// Fallback is the stream to default to.
	Fallback string
}

type metadata struct {
	// Name is the name of the metadata provider
	Name string
	// Auth is an arbitrary string containing authentication information for the provider
	// It is up to the individual provider to parse and use it correctly
	Auth string
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
	Proxy    radio.ProxyService
	Guest    radio.GuestService
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

	cfg.Streamer = newStreamerService(cfg)
	cfg.Manager = newManagerService(cfg)
	cfg.Guest = newGuestService(cfg)
	cfg.Tracker = newTrackerService(cfg)
	cfg.Queue = newQueueService(cfg)
	cfg.IRC = newIRCService(cfg)
	cfg.Proxy = newProxyService(cfg)

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
	cfg.Guest = nil
	cfg.Tracker = nil
	cfg.Queue = nil
	cfg.IRC = nil
	cfg.Proxy = nil

	c := cfg.Conf()
	c.Database.DSN = ""
	cfg.StoreConf(c)
	return cfg
}

// Loader is a typed function that returns a Config, used to pass in a pre-set Load or
// LoadFile call from a closure
type Loader func() (Config, error)

// LoadFile loads a configuration file from the filename given, if multiple
// filenames are given it will load the first one that exists
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

// OnReload lets you register a function that will be called when
// a configuration reload occurs
func (c *Config) OnReload(cb func()) {
	c.reloader.Lock()
	c.reloader.callbacks = append(c.reloader.callbacks, cb)
	c.reloader.Unlock()
}

// TriggerReload is called after a reload occurred
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
		loaded := store.Load()
		if loaded == nil || *loaded == nil {
			// we loaded a nil, that means nobody has made this OnceValue yet
			// so we do the job
			g := sync.OnceValue(func() T {
				return fn(cfg)
			})

			// now we need to store our OnceValue in the store, but some other
			// routine might be racing us on the creation so use a CAS loop
			for loaded == nil || *loaded == nil {
				if store.CompareAndSwap(loaded, &g) {
					// if we stored successfully, the store now holds our &g, so
					// set that as the loaded value
					loaded = &g
					break
				}

				// get the new value from the store, in most cases this will be a
				// OnceValue that another goroutine just stored so we don't have
				// to loop. But it's possible for this slow path and the reload path
				// (that sets the store to nil) be running at the same time so we do
				// a loop check just to be sure
				loaded = store.Load()
			}
		}
		return (*loaded)()
	}
}

func Values[T1, T2 any](cfg Config, fn func(Config) (T1, T2)) func() (T1, T2) {
	var store atomic.Pointer[func() (T1, T2)]

	cfg.OnReload(func() {
		store.Store(nil)
	})

	return func() (T1, T2) {
		loaded := store.Load()
		if loaded == nil || *loaded == nil {
			// we loaded a nil, that means nobody has made this OnceValue yet
			// so we do the job
			g := sync.OnceValues(func() (T1, T2) {
				return fn(cfg)
			})

			// now we need to store our OnceValue in the store, but some other
			// routine might be racing us on the creation so use a CAS loop
			for loaded == nil || *loaded == nil {
				if store.CompareAndSwap(loaded, &g) {
					// if we stored successfully, the store now holds our &g, so
					// set that as the loaded value
					loaded = &g
					break
				}

				// get the new value from the store, in most cases this will be a
				// OnceValue that another goroutine just stored so we don't have
				// to loop. But it's possible for this slow path and the reload path
				// (that sets the store to nil) be running at the same time so we do
				// a loop check just to be sure
				loaded = store.Load()
			}
		}
		return (*loaded)()
	}
}

// Conf returns the configuration stored inside
//
// NOTE: Conf returns a shallow-copy of the config value stored inside; so do not edit
// any slices or maps that might be inside
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
