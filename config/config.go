package config

import (
	crand "crypto/rand"
	"io"
	"log"
	"math"
	"math/big"
	"math/rand"
	"net"
	"net/netip"
	"net/url"
	"os"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"github.com/BurntSushi/toml"
	radio "github.com/R-a-dio/valkyrie"
)

// defaultConfig is the default configuration for this project
var defaultConfig = config{
	DevelopmentMode:  true,
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
		WebsiteAddr:               MustParseAddrPort("localhost:3241"),
		DJImageMaxSize:            10 * 1024 * 1024,
		DJImagePath:               "/radio/dj-images",
		PublicStreamURL:           "http://localhost:8000/main.mp3",
		AdminMonitoringURL:        "http://grafana:3000",
		AdminMonitoringUserHeader: "x-proxy-user",
		AdminMonitoringRoleHeader: "x-proxy-role",
	},
	Streamer: streamer{
		RPCAddr:         MustParseAddrPort(":4545"),
		StreamURL:       "http://127.0.0.1:1337/main.mp3",
		RequestsEnabled: true,
		ConnectTimeout:  Duration(time.Second * 30),
	},
	IRC: irc{
		RPCAddr:        MustParseAddrPort(":4444"),
		AllowFlood:     false,
		EnableEcho:     true,
		AnnouncePeriod: Duration(time.Second * 15),
	},
	Manager: manager{
		RPCAddr:         MustParseAddrPort(":4646"),
		StreamURL:       "",
		FallbackNames:   []string{"fallback"},
		GuestProxyAddr:  "localhost:9123",
		GuestAuthPeriod: Duration(time.Hour * 24),
	},
	Search: search{
		Endpoint:  "http://127.0.0.1:9200/",
		IndexPath: "/radio/search",
	},
	Balancer: balancer{
		Addr:     "127.0.0.1:4848",
		Fallback: "https://relay0.r-a-d.io/main.mp3",
	},
	Proxy: proxy{
		RPCAddr:          MustParseAddrPort(":5151"),
		ListenAddr:       MustParseAddrPort(":1337"),
		MasterServer:     "http://127.0.0.1:8000",
		MasterUsername:   "source",
		MasterPassword:   "hackme",
		PrimaryMountName: "/main.mp3",
	},
	Tracker: tracker{
		RPCAddr:          MustParseAddrPort(":4949"),
		ListenAddr:       MustParseAddrPort(":9999"),
		MasterServer:     "http://127.0.0.1:8000",
		MasterUsername:   "admin",
		MasterPassword:   "hackme",
		PrimaryMountName: "/main.mp3",
	},
	Telemetry: telemetry{
		Use:                false,
		Endpoint:           ":4317",
		PrometheusEndpoint: "localhost:9091",
	},
	Tunein: tunein{
		Endpoint: "https://air.radiotime.com/Playing.ashx",
	},
}

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
	// PrimaryMountName is the mountname to keep track of
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
}

type telemetry struct {
	Use                bool
	Endpoint           string
	Auth               string
	PrometheusEndpoint string

	StandaloneProxy struct {
		Enabled    bool
		URL        URL
		ListenAddr AddrPort
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
	// StreamURL is the url to listen to the mp3 stream
	StreamURL string
	// FallbackNames is a list of strings that indicate an icecast stream is playing a
	// fallback stream
	FallbackNames []string

	GuestProxyAddr  URL
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

type AddrPort struct {
	ap netip.AddrPort
}

func MustParseAddrPort(s string) AddrPort {
	ap, err := ParseAddrPort(s)
	if err != nil {
		panic("MustParseAddrPort: " + err.Error())
	}
	return ap
}

func ParseAddrPort(s string) (AddrPort, error) {
	host, sPort, err := net.SplitHostPort(s)
	if err != nil {
		return AddrPort{}, err
	}

	if host == "localhost" {
		host = localAddr.String()
	}

	var addr = localAddr
	if host != "" {
		addr, err = netip.ParseAddr(host)
		if err != nil {
			return AddrPort{}, err
		}
	}

	port, err := strconv.ParseUint(sPort, 10, 16)
	if err != nil {
		return AddrPort{}, err
	}

	return AddrPort{
		ap: netip.AddrPortFrom(addr, uint16(port)),
	}, nil
}

func (ap AddrPort) String() string {
	return ap.ap.String()
}

func (ap AddrPort) Port() uint16 {
	return ap.ap.Port()
}

func (ap AddrPort) Addr() netip.Addr {
	return ap.ap.Addr()
}

func (ap AddrPort) MarshalText() ([]byte, error) {
	return ap.ap.MarshalText()
}

var localAddr = netip.MustParseAddr("127.0.0.1")

func (ap *AddrPort) UnmarshalText(text []byte) error {
	if len(text) == 0 {
		return nil
	}
	res, err := ParseAddrPort(string(text))
	if err != nil {
		return err
	}
	*ap = res

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
