package tracker

import (
	"context"
	"net"
	"net/http"
	"slices"
	"strings"
	"sync/atomic"
	"time"

	radio "github.com/R-a-dio/valkyrie"
	"github.com/R-a-dio/valkyrie/util"
	"github.com/rs/zerolog"
)

func NewRecorder(ctx context.Context) *Recorder {
	r := &Recorder{}

	go r.PeriodicallyRemoveStalePending(ctx, RemoveStalePendingTickrate)
	return r
}

func NewListener(id radio.ListenerClientID, req *http.Request) radio.Listener {
	return radio.Listener{
		ID:        id,
		UserAgent: req.PostFormValue("agent"), // passed by icecast
		Start:     time.Now(),
		IP:        IcecastRealIP(req), // grab real IP from the POST form data
	}
}

type Listener struct {
	radio.Listener
	Removed     bool
	RemovedTime time.Time
}

type Recorder struct {
	listeners      util.Map[radio.ListenerClientID, *Listener]
	listenerAmount atomic.Int64
}

func (r *Recorder) PeriodicallyRemoveStalePending(ctx context.Context, tickrate time.Duration) {
	ticker := time.NewTicker(tickrate)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			stale := r.removeStalePending()
			if stale > 0 {
				zerolog.Ctx(ctx).Error().Int("amount", stale).Msg("found stale pending removals")
			}
		}
	}
}

func (r *Recorder) removeStalePending() (found_stale int) {
	deadline := time.Now().Add(-RemoveStalePendingPeriod)

	r.listeners.Range(func(key radio.ListenerClientID, value *Listener) bool {
		if value.Removed && value.RemovedTime.Before(deadline) {
			// deadline exceeded, remove the entry
			r.listeners.Delete(key)
			found_stale++
		}
		return true
	})

	return found_stale
}

func (r *Recorder) ListenerAmount() int64 {
	return r.listenerAmount.Load()
}

func (r *Recorder) ListenerAdd(ctx context.Context, listener radio.Listener) {
	entry, loaded := r.listeners.LoadOrStore(listener.ID, &Listener{Listener: listener})
	if loaded && entry.Removed {
		// if we are here it means ListenerRemove was called before us and we should
		// just remove ourselves
		r.listeners.Delete(listener.ID)
	} else {
		r.listenerAmount.Add(1)
	}
}

func (r *Recorder) ListenerRemove(ctx context.Context, id radio.ListenerClientID) {
	_, loaded := r.listeners.LoadOrStore(id, &Listener{
		Removed:     true,
		RemovedTime: time.Now(),
	})
	if loaded {
		// if we loaded it means there was an entry added by ListenerAdd so we
		// now want to delete that entry
		r.listeners.Delete(id)
		r.listenerAmount.Add(-1)
	}
}

func (r *Recorder) ListClients(ctx context.Context) ([]radio.Listener, error) {
	res := make([]radio.Listener, 0, r.ListenerAmount())
	r.listeners.Range(func(key radio.ListenerClientID, value *Listener) bool {
		if value.Removed {
			// skip removed entries
			return true
		}
		res = append(res, value.Listener)
		return true
	})

	// sort the entries by their start time
	slices.SortFunc(res, func(a, b radio.Listener) int {
		return a.Start.Compare(b.Start)
	})
	return res, nil
}

func (r *Recorder) RemoveClient(ctx context.Context, id radio.ListenerClientID) error {
	// TODO: implement this
	return nil
}

const prefix = "client."

var xForwardedFor = strings.ToLower(prefix + "X-Forwarded-For")
var xRealIP = strings.ToLower(prefix + "X-Real-IP")
var trueClientIP = strings.ToLower(prefix + "True-Client-IP")

// icecastRealIP recovers the clients real ip address from the request
//
// This looks for X-Forwarded-For, X-Real-IP and True-Client-IP
func IcecastRealIP(r *http.Request) string {
	var ip string

	if tcip := r.PostForm.Get(trueClientIP); tcip != "" {
		ip = tcip
	} else if xrip := r.PostForm.Get(xRealIP); xrip != "" {
		ip = xrip
	} else if xff := r.PostForm.Get(xForwardedFor); xff != "" {
		i := strings.Index(xff, ",")
		if i == -1 {
			i = len(xff)
		}
		ip = xff[:i]
	}
	if ip == "" || net.ParseIP(ip) == nil {
		return ""
	}
	return ip

}
