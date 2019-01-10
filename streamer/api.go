package streamer

import (
	"context"
	"net/http"
	"net/http/pprof"
	"sync"
	"sync/atomic"
	"time"

	"github.com/R-a-dio/valkyrie/config"
	pb "github.com/R-a-dio/valkyrie/rpc/streamer"
)

// NewHTTPServer returns a http server with RPC API handler and debug handlers
func NewHTTPServer(cfg config.Config, queue *Queue, streamer *Streamer) (*http.Server, error) {
	h := &streamHandler{
		Config:   cfg,
		queue:    queue,
		streamer: streamer,
	}

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

	conf := cfg.Conf()
	server := &http.Server{Addr: conf.Streamer.Addr, Handler: mux}

	return server, nil
}

type streamHandler struct {
	config.Config

	queue        *Queue
	streamer     *Streamer
	requestMutex sync.Mutex
}

func (h *streamHandler) Start(ctx context.Context, _ *pb.Null) (*pb.Null, error) {
	h.streamer.Start(context.Background())
	return nil, nil
}

func (h *streamHandler) Stop(ctx context.Context, r *pb.StopRequest) (*pb.Null, error) {
	if r.ForceStop {
		return nil, h.streamer.ForceStop(ctx)
	}
	return nil, h.streamer.Stop(ctx)
}

func (h *streamHandler) Status(ctx context.Context, _ *pb.Null) (*pb.StatusResponse, error) {
	var resp pb.StatusResponse
	resp.Running = atomic.LoadInt32(&h.streamer.started) == 1

	for _, e := range h.streamer.queue.Entries() {
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
