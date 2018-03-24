package streamer

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"net/http/pprof"
	"os"
	"os/exec"
	"sync/atomic"
)

func ListenAndServe(s *State) error {
	h := handlers{State: s}
	mux := http.NewServeMux()
	// streamer handles
	mux.Handle("/streamer/request", RequestHandler(s))
	mux.HandleFunc("/streamer/request/enable", h.requestEnableHandler)
	mux.HandleFunc("/streamer/request/disable", h.requestDisableHandler)

	mux.HandleFunc("/streamer/start", h.startHandler)
	mux.HandleFunc("/streamer/stop", h.stopHandler)

	mux.HandleFunc("/streamer/graceful/start", h.gracefulStartHandler)
	mux.HandleFunc("/streamer/graceful/poke", h.gracefulPokeHandler)

	// debug symbols
	mux.HandleFunc("/debug/pprof/", pprof.Index)
	mux.HandleFunc("/debug/pprof/cmdline", pprof.Cmdline)
	mux.HandleFunc("/debug/pprof/profile", pprof.Profile)
	mux.HandleFunc("/debug/pprof/symbol", pprof.Symbol)
	mux.HandleFunc("/debug/pprof/trace", pprof.Trace)

	conf := s.Conf()
	server := &http.Server{Addr: conf.InterfaceAddr, Handler: mux}
	s.httpserver = server

	if s.httplistener == nil {
		fmt.Println("http: listening on:", conf.InterfaceAddr)
		listener, err := net.Listen("tcp", conf.InterfaceAddr)
		if err != nil {
			return err
		}
		s.httplistener = listener
		// not expecting a poke
		close(h.gracefulWait)
	} else {
		// we inherited a listener, so expect a poke
		fmt.Println("graceful: new: expecting poke")
		h.gracefulPoke = 1
	}

	fmt.Println("http: serving")
	return server.Serve(s.httplistener)
}

type handlers struct {
	// indicates if a graceful restart is in progress
	gracefulUnderway int32
	// indicates if we're expecting a graceful poke
	gracefulPoke int32
	*State
}

func (h *handlers) startHandler(w http.ResponseWriter, r *http.Request) {
	h.streamer.Start(context.Background())
}

func (h *handlers) stopHandler(w http.ResponseWriter, r *http.Request) {
	fmt.Fprintf(w, "%s", h.streamer.Stop())
}

func (h *handlers) requestEnableHandler(w http.ResponseWriter, r *http.Request) {
	conf := h.Conf()
	conf.RequestsEnabled = true
	h.StoreConf(conf)
}

func (h *handlers) requestDisableHandler(w http.ResponseWriter, r *http.Request) {
	conf := h.Conf()
	conf.RequestsEnabled = false
	h.StoreConf(conf)
}

func (h *handlers) gracefulStartHandler(w http.ResponseWriter, r *http.Request) {
	if !atomic.CompareAndSwapInt32(&h.gracefulUnderway, 0, 1) {
		return
	}

	// setup our arguments
	var args []string
	args = append(args, "-graceful")
	args = append(args, os.Args[1:]...)

	// create our new process
	cmd := exec.Command(os.Args[0], args...)

	files := make([]*os.File, 2)
	// find our extra files, we want our http server and icecast connection
	if l, ok := h.httplistener.(*net.TCPListener); ok {
		fd, err := l.File()
		files[0] = fd
		if err != nil {
			fmt.Println("graceful: listener error:", err)
			sendJSON(w, 9080, "unsupported")
			return
		}
	}

	if i, ok := h.streamer.conn.(*net.TCPConn); ok {
		fd, err := i.File()
		files[1] = fd
		if err != nil {
			fmt.Println("graceful: icecast error:", err)
			sendJSON(w, 9081, "unsupported")
			return
		}
	}

	cmd.ExtraFiles = files
	err := cmd.Start()
	if err != nil {
		sendJSON(w, 9000, "process failed to start")
		return
	}

	// mark for the streamer to stop, ignore errors
	go h.streamer.stop(false, true)
	// stop our server from accepting new connections
	go h.httpserver.Shutdown(context.Background())

	// now we want to wait for our streamer to stop, and signal the new process
	// right after it occurs
	err = h.streamer.Wait()
	// ignore error since we already spawned a process
	if err != nil {
		fmt.Println("graceful: streamer wait error:", err)
	}

	addr := h.httplistener.Addr().String()
	url := fmt.Sprintf("http://%s/streamer/graceful/poke", addr)

	resp, err := http.Get(url)
	if err != nil {
		sendJSON(w, 9001, "failed to poke process")
		return
	}
	resp.Body.Close()

	sendJSON(w, 0, "success")
	// close ourselves
	go h.Shutdown()
}

func (h *handlers) gracefulPokeHandler(w http.ResponseWriter, r *http.Request) {
	if !atomic.CompareAndSwapInt32(&h.gracefulPoke, 1, 0) {
		sendJSON(w, 9050, "not ready for poke")
		return
	}

	close(h.gracefulWait)
}
