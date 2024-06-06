package tracker

import (
	"context"
	"encoding/json"
	"os"

	radio "github.com/R-a-dio/valkyrie"
	"github.com/Wessie/fdstore"
)

type wireRecorder struct {
	Listeners map[radio.ListenerClientID]*Listener
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
	wr := new(wireRecorder)
	wr.Listeners = make(map[radio.ListenerClientID]*Listener)
	r.listeners.Range(func(key radio.ListenerClientID, value *Listener) bool {
		wr.Listeners[key] = value
		return true
	})
	d, err := json.Marshal(wr)
	if err != nil {
		return err
	}

	tmpf, err := os.CreateTemp("", "listeners")
	if err != nil {
		return err
	}
	// close since createtemp opens
	tmpf.Close()
	return fds.AddFile(tmpf, TrackerFile, d)
}

func (r *Recorder) restoreSelf(ctx context.Context, fds *fdstore.Store) error {
	fes := fds.RemoveFile(TrackerFile)
	if len(fes) != 1 {
		return nil
	}
	fe := fes[0]
	var wr wireRecorder

	err := json.Unmarshal(fe.Data, &wr)
	if err != nil {
		return err
	}

	var i int64
	for k, v := range wr.Listeners {
		r.listeners.Store(k, v)
		i += 1
	}
	r.listenerAmount.Add(i)

	err = os.Remove(fe.File.Name())
	if err != nil {
		return err
	}

	return nil
}
