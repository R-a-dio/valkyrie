package tracker

import (
	"context"

	radio "github.com/R-a-dio/valkyrie"
	"github.com/Wessie/fdstore"
)

type wireRecorder struct {
	listeners map[radio.ListenerClientID]*Listener
}

func (s *Server) storeSelf(ctx context.Context, fds *fdstore.Store) error {
	err := fds.AddListener(s.httpLn, HttpLn, nil)
	if err != nil {
		return err
	}
	err = fds.AddListener(s.grpcLn, GrpcLn, nil)
	if err != nil {
		return err
	}
	return s.recorder.storeSelf(ctx, fds)
}

func (r *Recorder) storeSelf(ctx context.Context, fds *fdstore.Store) error {
	return nil
}
