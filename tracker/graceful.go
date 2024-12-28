package tracker

import (
	"context"
	"encoding/json"

	radio "github.com/R-a-dio/valkyrie"
	"github.com/Wessie/fdstore"
)

type wireRecorder struct {
	Listeners map[radio.ListenerClientID]*Listener
}

func (s *Server) storeSelf(ctx context.Context, fds *fdstore.Store) error {
	recorderState, err := s.recorder.MarshalJSON()
	if err != nil {
		return err
	}

	err = fds.AddListener(s.httpLn, HttpLn, recorderState)
	if err != nil {
		return err
	}
	err = fds.AddListener(s.grpcLn, GrpcLn, nil)
	if err != nil {
		return err
	}
	return nil
}

func (r *Recorder) MarshalJSON() ([]byte, error) {
	wr := new(wireRecorder)
	wr.Listeners = make(map[radio.ListenerClientID]*Listener)
	r.listeners.Range(func(key radio.ListenerClientID, value *Listener) bool {
		wr.Listeners[key] = value
		return true
	})

	return json.Marshal(wr)
}

func (r *Recorder) UnmarshalJSON(p []byte) error {
	var wr wireRecorder

	err := json.Unmarshal(p, &wr)
	if err != nil {
		return err
	}

	var i int64
	for k, v := range wr.Listeners {
		r.listeners.Store(k, v)
		i += 1
	}
	r.listenerAmount.Add(i)
	return nil
}
