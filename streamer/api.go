package streamer

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/http/pprof"
	"os"
	"os/exec"
	"sync"
	"sync/atomic"

	"github.com/R-a-dio/valkyrie/database"
)

// ListenAndServe serves a HTTP API for the state given on the address
// configured in the state configuration.
func ListenAndServe(s *State) error {
	h := &streamHandler{State: s}
	mux := http.NewServeMux()
	// streamer handler
	mux.Handle("/", http.HandlerFunc(h.actionHandler))
	// request handler
	mux.Handle("/request", RequestHandler(s))
	mux.Handle("/info", http.HandlerFunc(h.statusHandler))

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
		atomic.StoreInt32(&h.gracefulPoke, 1)
	}

	fmt.Println("http: serving")
	return server.Serve(s.httplistener)
}

type streamHandler struct {
	// indicates if a graceful restart is in progress
	gracefulUnderway int32
	// indicates if we're expecting a graceful poke
	gracefulPoke int32
	*State
}

func (h *streamHandler) statusHandler(w http.ResponseWriter, r *http.Request) {
	var info = struct {
		Queue   []database.QueueEntry
		Running bool
	}{
		Queue:   h.queue.Entries(),
		Running: atomic.LoadInt32(&h.streamer.started) == 1,
	}

	w.Header().Add("Content-Type", "application/json")
	json.NewEncoder(w).Encode(info)
}

func (h *streamHandler) actionHandler(w http.ResponseWriter, r *http.Request) {
	// when you change api signature here, make sure to also adjust it below
	// in the graceful handlers
	//
	// supported actions:
	// 	"?action=start"
	//		start the streamer if it isn't running yet
	//	"?action=stop&force=false"
	//		wait for song to end and stop the streamer
	//	"?action=stop&force=true"
	//		stop the streamer without waiting
	//	"?action=restart"
	//		restart the streamer gracefully without dropping conns
	//	"?action=poke"
	//		internal use for graceful restarts
	//	"?request=enable" and "?request=disable"
	//		enable or disable the ability to request songs
	if err := r.ParseForm(); err != nil {
		http.Error(w, http.StatusText(400), 400)
		return
	}

	// streamer actions
	switch r.Form.Get("action") {
	case "start":
		h.streamer.Start(context.Background())
	case "stop":
		force := r.Form.Get("force") == "true"
		h.streamer.stop(force, false)
	case "restart":
		h.gracefulRestartHandler(w, r)
	case "poke":
		h.gracefulPokeHandler(w, r)
	}

	// request action
	var requestState bool

	switch r.Form.Get("request") {
	case "enable":
		requestState = true
	case "disable":
		requestState = false
	default:
		http.Error(w, http.StatusText(400), 400)
		return
	}

	c := h.Conf()
	c.RequestsEnabled = requestState
	h.StoreConf(c)
}

// gracefulRestart tries to spawn a new process to take over from the current
// process without downtime for the end-user.
func (h *streamHandler) gracefulRestartHandler(w http.ResponseWriter, r *http.Request) {
	// if we've already attempted one we don't need to try again
	if !atomic.CompareAndSwapInt32(&h.gracefulUnderway, 0, 1) {
		sendJSON(w, 9001, "already called")
		return
	}

	// setup our arguments for the new process, this should be equal to the old
	// process except that
	var args []string
	// check if the current process was also started with graceful, omit the flag
	// if so such that we don't get duplicate flags
	if len(os.Args) > 1 && os.Args[1] != "-graceful" {
		args = append(args, "-graceful")
		args = append(args, os.Args[2:]...)
	} else {
		args = append(args, os.Args[1:]...)
	}

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

	var wg sync.WaitGroup
	wg.Add(2)
	// mark for the streamer to stop, ignore errors
	go func() {
		defer wg.Done()
		h.streamer.stop(false, true)
	}()

	// stop our server from accepting new connections, the new process needs to
	// be able to catch the poke we do after shutting down
	go func() {
		defer wg.Done()
		h.httpserver.Shutdown(context.Background())
	}()

	// now we want to wait for our streamer to stop, and signal the new process
	// right after it occurs
	err = h.streamer.Wait()
	// ignore error since we already spawned a process
	if err != nil {
		fmt.Println("graceful: streamer wait error:", err)
	}

	addr := h.httplistener.Addr().String()
	url := fmt.Sprintf("http://%s/?action=poke", addr)

	resp, err := http.Get(url)
	if err != nil {
		sendJSON(w, 9001, "failed to poke process")
		return
	}
	resp.Body.Close()

	sendJSON(w, 0, "success")

	go func() {
		// wait for open server connections to close
		wg.Wait()
		// and then we can exit the old process completely
		h.Shutdown()
	}()
}

// gracefulPoke is called in the new process after a graceful restart is done;
// The poke signifies that the old process is done with what it is doing and
// that the new process should start actually doing stuff.
func (h *streamHandler) gracefulPokeHandler(w http.ResponseWriter, r *http.Request) {
	if !atomic.CompareAndSwapInt32(&h.gracefulPoke, 1, 0) {
		sendJSON(w, 9050, "not ready for poke")
		return
	}

	// close our waiting channel, this will unblock anyone waiting on the
	// previous process to exit.
	close(h.gracefulWait)
}
