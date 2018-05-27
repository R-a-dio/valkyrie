package streamer

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"time"

	"github.com/R-a-dio/valkyrie/config"
	_ "github.com/go-sql-driver/mysql" // only support mysql/mariadb for now
	"github.com/jmoiron/sqlx"
)

type State struct {
	db *sqlx.DB
	config.AtomicGlobal

	queue      *Queue
	streamer   *Streamer
	httpserver *http.Server
}

func (s *State) Shutdown() {
	fmt.Println("streamer error:", s.streamer.ForceStop())
	fmt.Println("queue    error:", s.queue.Save())
	fmt.Println("database error:", s.db.Close())
	fmt.Println("httpserv error:", s.httpserver.Close())
	fmt.Println("finished closing")
	time.Sleep(time.Millisecond * 250)
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
	s.AtomicGlobal, err = config.LoadAtomic(f)
	if err != nil {
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
