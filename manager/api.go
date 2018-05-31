package manager

import (
	"context"
	"sync"

	pb "github.com/R-a-dio/valkyrie/rpc/manager"
	"github.com/golang/protobuf/proto"
)

type api struct {
	*State

	mu     sync.Mutex
	status *pb.StatusResponse
}

func (a *api) Status(ctx context.Context, _ *pb.StatusRequest) (*pb.StatusResponse, error) {
	var sr pb.StatusResponse
	a.mu.Lock()
	proto.Merge(&sr, a.status)
	a.mu.Unlock()
	return &sr, nil
}

func (a *api) SetUser(ctx context.Context, u *pb.User) (*pb.User, error) {
	var old *pb.User
	a.mu.Lock()
	old, a.status.User = a.status.User, u
	a.mu.Unlock()
	return old, nil
}

func (a *api) SetSong(ctx context.Context, s *pb.Song) (*pb.Song, error) {
	var old *pb.Song
	a.mu.Lock()
	old, a.status.Song = a.status.Song, s
	a.mu.Unlock()
	return old, nil
}

func (a *api) SetBotConfig(ctx context.Context, bc *pb.BotConfig) (*pb.BotConfig, error) {
	var old *pb.BotConfig
	a.mu.Lock()
	old, a.status.BotConfig = a.status.BotConfig, bc
	a.mu.Unlock()
	return old, nil
}

func (a *api) SetThread(ctx context.Context, t *pb.Thread) (*pb.Thread, error) {
	var old *pb.Thread
	a.mu.Lock()
	old, a.status.Thread = a.status.Thread, t
	a.mu.Unlock()
	return old, nil
}

func (a *api) SetListenerInfo(ctx context.Context, li *pb.ListenerInfo) (*pb.ListenerInfo, error) {
	var old *pb.ListenerInfo
	a.mu.Lock()
	old, a.status.ListenerInfo = a.status.ListenerInfo, li
	a.mu.Unlock()
	return old, nil
}
