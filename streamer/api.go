package streamer

import (
	"context"
	"net/http"
	"net/http/pprof"
	"sync"

	"github.com/R-a-dio/valkyrie/config"
	"github.com/R-a-dio/valkyrie/rpc"
)

// NewHTTPServer returns a http server with RPC API handler and debug handlers
func NewHTTPServer(cfg config.Config, queue *Queue, streamer *Streamer) (*http.Server, error) {
	h := &streamHandler{
		Config:   cfg,
		queue:    queue,
		streamer: streamer,
	}

	rpcServer := rpc.NewStreamerServer(h, nil)
	mux := http.NewServeMux()
	// rpc server path
	mux.Handle(rpc.StreamerPathPrefix, rpcServer)

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

func (h *streamHandler) Start(ctx context.Context, _ *rpc.Null) (*rpc.Null, error) {
	h.streamer.Start(context.Background())
	return nil, nil
}

func (h *streamHandler) Stop(ctx context.Context, r *rpc.StopRequest) (*rpc.Null, error) {
	if r.ForceStop {
		return nil, h.streamer.ForceStop(ctx)
	}
	return nil, h.streamer.Stop(ctx)
}

func (h *streamHandler) SetRequestable(ctx context.Context, b *rpc.Bool) (*rpc.Null, error) {
	c := h.Conf()
	c.Streamer.RequestsEnabled = b.Bool
	h.StoreConf(c)
	return nil, nil
}
