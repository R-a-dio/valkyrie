package tracker

import (
	"cmp"
	"context"
	"net"
	"net/http"
	"slices"
	"strings"
	"sync/atomic"
	"time"

	radio "github.com/R-a-dio/valkyrie"
	"github.com/R-a-dio/valkyrie/config"
	"github.com/R-a-dio/valkyrie/util"
	"github.com/rs/zerolog"
)

func NewRecorder(ctx context.Context, cfg config.Config) *Recorder {
	r := &Recorder{
		cfg: cfg,
	}

	go r.PeriodicallyRemoveStale(ctx, RemoveStaleTickrate)
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
	cfg            config.Config
	listeners      util.Map[radio.ListenerClientID, *Listener]
	listenerAmount atomic.Int64
	syncing        atomic.Bool
}

func (r *Recorder) PeriodicallyRemoveStale(ctx context.Context, tickrate time.Duration) {
	ticker := time.NewTicker(tickrate)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			stale := r.removeStale(RemoveStalePeriod)
			if stale > 0 {
				zerolog.Ctx(ctx).Error().Int("amount", stale).Msg("found stale pending removals")
			}
		}
	}
}

func (r *Recorder) removeStale(period time.Duration) (found_stale int) {
	deadline := time.Now().Add(-period)

	r.listeners.Range(func(key radio.ListenerClientID, value *Listener) bool {
		if value.Removed && value.RemovedTime.Before(deadline) {
			// deadline exceeded, remove the entry
			if r.listeners.CompareAndDelete(key, value) {
				found_stale++
			}
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
	if loaded {
		if entry.Removed {
			// we loaded and received an entry with the Removed flag set, this means
			// ListenerRemove was called on this ID and we should not exist
			r.listeners.CompareAndDelete(listener.ID, entry)
		}
	} else {
		// only add to the listener count if we actually did a store
		r.listenerAmount.Add(1)
	}
}

func (r *Recorder) ListenerRemove(ctx context.Context, id radio.ListenerClientID) {
	entry, loaded := r.listeners.LoadOrStore(id, &Listener{
		Removed:     true,
		RemovedTime: time.Now(),
	})
	if loaded {
		// if we loaded it means there was an entry added by ListenerAdd so we
		// now want to delete that entry
		var deleted bool
		if r.syncing.Load() {
			// if we're in the process of syncing we might have listeners
			// added back by the process, guard against that by inserting
			// a Removed listener
			deleted = r.listeners.CompareAndSwap(id, entry, &Listener{
				Removed:     true,
				RemovedTime: time.Now(),
			})
		} else {
			// otherwise just do a normal delete
			deleted = r.listeners.CompareAndDelete(id, entry)
		}
		if !entry.Removed && deleted {
			// only remove a listener count if the entry wasn't marked as
			// Removed already and if we actually deleted an entry
			r.listenerAmount.Add(-1)
		}
	}
}

func (r *Recorder) ListClients(ctx context.Context) ([]radio.Listener, error) {
	res := make([]radio.Listener, 0, r.ListenerAmount())
	r.listeners.Range(func(_ radio.ListenerClientID, value *Listener) bool {
		if !value.Removed {
			res = append(res, value.Listener)
		}
		return true
	})

	// sort the entries
	sortListeners(res)
	return res, nil
}

func sortListeners(in []radio.Listener) {
	// sort the entries by their ID
	slices.SortFunc(in, func(a, b radio.Listener) int {
		return cmp.Compare(a.ID, b.ID)
	})
}

func (r *Recorder) RemoveClient(ctx context.Context, id radio.ListenerClientID) error {
	ctx, cancel := context.WithTimeout(ctx, time.Second*5)
	defer cancel()

	return RemoveIcecastClient(ctx, r.cfg, id)
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
		return r.RemoteAddr
	}
	return ip

}
