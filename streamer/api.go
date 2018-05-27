package streamer

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"net/http/pprof"
	"sync"
	"sync/atomic"
	"time"

	pb "github.com/R-a-dio/valkyrie/rpc/streamer"
)

// ListenAndServe serves a HTTP API for the state given on the address
// configured in the state configuration.
func ListenAndServe(s *State) error {
	h := &streamHandler{State: s}
	rpcServer := pb.NewStreamerServer(h, nil)
	mux := http.NewServeMux()
	// rpc server path
	mux.Handle(pb.StreamerPathPrefix, rpcServer)

	// debug symbols
	mux.HandleFunc("/debug/pprof/", pprof.Index)
	mux.HandleFunc("/debug/pprof/cmdline", pprof.Cmdline)
	mux.HandleFunc("/debug/pprof/profile", pprof.Profile)
	mux.HandleFunc("/debug/pprof/symbol", pprof.Symbol)
	mux.HandleFunc("/debug/pprof/trace", pprof.Trace)

	conf := s.Conf()
	server := &http.Server{Addr: conf.Streamer.Addr, Handler: mux}

	s.Closer("http", server.Close)

	fmt.Println("http: listening on:", conf.Streamer.Addr)
	l, err := net.Listen("tcp", conf.Streamer.Addr)
	if err != nil {
		return err
	}

	fmt.Println("http: serving")
	return server.Serve(l)
}

type streamHandler struct {
	*State

	requestMutex sync.Mutex
}

func (h *streamHandler) Start(ctx context.Context, _ *pb.Null) (*pb.Null, error) {
	h.streamer.Start(context.Background())
	return nil, nil
}

func (h *streamHandler) Stop(ctx context.Context, r *pb.StopRequest) (*pb.Null, error) {
	if r.ForceStop {
		return nil, h.streamer.ForceStop()
	}
	return nil, h.streamer.Stop()
}

func (h *streamHandler) Status(ctx context.Context, _ *pb.Null) (*pb.StatusResponse, error) {
	var resp pb.StatusResponse
	resp.Running = atomic.LoadInt32(&h.streamer.started) == 1

	for _, e := range h.queue.Entries() {
		resp.Queue = append(resp.Queue, &pb.QueueEntry{
			IsRequest:         e.IsRequest,
			UserIdentifier:    e.UserIdentifier,
			EstimatedPlayTime: e.EstimatedPlayTime.Format(time.RFC3339Nano),
			TrackId:           int64(e.Track.ID),
			TrackTags:         e.Track.Metadata,
		})
	}

	return &resp, nil
}

func (h *streamHandler) SetRequestable(ctx context.Context, b *pb.Bool) (*pb.Null, error) {
	c := h.Conf()
	c.Streamer.RequestsEnabled = b.Bool
	h.StoreConf(c)
	return nil, nil
}
