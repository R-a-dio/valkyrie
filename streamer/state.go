package streamer

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"sync"
	"sync/atomic"
	"time"

	"github.com/BurntSushi/toml"
	"github.com/R-a-dio/valkyrie/database"
	_ "github.com/go-sql-driver/mysql" // only support mysql/mariadb for now
	"github.com/jmoiron/sqlx"
)

var DefaultConfig = Config{
	UserAgent:        "hanyuu/2.0",
	UserRequestDelay: time.Hour * 2,
	RequestsEnabled:  false,
	TemplateDir:      "templates/",
	InterfaceAddr:    ":4545",
}

type State struct {
	db     *sqlx.DB
	config atomic.Value

	queue        *Queue
	streamer     *Streamer
	httpserver   *http.Server
	httplistener net.Listener

	graceful graceful
}

type graceful struct {
	mu    sync.Mutex
	wait  chan struct{}
	conn  chan net.Conn
	_conn net.Conn
}

// GracefulSetup sets up re-used sockets from a previous process
func (s *State) GracefulSetup(l net.Listener, c net.Conn) {
	s.httplistener = l

	s.graceful.wait = make(chan struct{})
	s.graceful.conn = make(chan net.Conn, 1)
	go func() {
		<-s.graceful.wait
		s.graceful.conn <- c
		close(s.graceful.conn)
	}()
}

// setConn sets the connection to be passed along when restarted
func (g *graceful) setConn(c net.Conn) {
	g.mu.Lock()
	g._conn = c
	g.mu.Unlock()
}

// Conf returns the current active configuration
func (s *State) Conf() Config {
	return s.config.Load().(Config)
}

// StoreConf changes the active configuration
func (s *State) StoreConf(c Config) {
	s.config.Store(c)
}

func (s *State) Shutdown() {
	fmt.Println("streamer error:", s.streamer.ForceStop())
	fmt.Println("queue    error:", s.queue.Save())
	fmt.Println("database error:", s.db.Close())
	fmt.Println("httpserv error:", s.httpserver.Close())
	fmt.Println("finished closing")
	time.Sleep(time.Millisecond * 250)
}

// LoadConf loads a configuration file from reader r and changes the active
// configuration.
func (s *State) LoadConf(r io.Reader) error {
	var c = DefaultConfig

	m, err := toml.DecodeReader(r, &c)
	if err != nil {
		return err
	}

	fmt.Println("undecoded keys:", m.Undecoded())
	s.StoreConf(c)
	return nil
}

func (s *State) loadDatabase() (err error) {
	conf := s.Conf()

	s.db, err = sqlx.Open(conf.Database.DriverName, conf.Database.DSN)
	return err
}

// LoadQueue loads a Queue for this state, returns any errors encountered
func (s *State) loadQueue() (err error) {
	s.queue, err = NewQueue(s)
	return err
}

func (s *State) loadStreamer() (err error) {
	s.streamer, err = NewStreamer(s)
	return err
}

func (s *State) StartStreamer() {
	s.streamer.Start(context.Background())
}

// NewState initializes a state struct with all the required items
func NewState(configPath string) (*State, error) {
	var s State

	f, err := os.Open(configPath)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	fmt.Println("startup: loading configuration")
	if err = s.LoadConf(f); err != nil {
		return nil, err
	}

	fmt.Println("startup: loading database")
	if err = s.loadDatabase(); err != nil {
		return nil, err
	}

	fmt.Println("startup: loading queue")
	if err = s.loadQueue(); err != nil {
		return nil, err
	}

	fmt.Println("startup: loading streamer")
	if err = s.loadStreamer(); err != nil {
		return nil, err
	}

	return &s, nil
}

type Config struct {
	UserAgent string
	StreamURL string
	MusicPath string

	UserRequestDelay time.Duration
	RequestsEnabled  bool

	Database database.Config

	TemplateDir   string
	InterfaceAddr string
}
