package graceful

import (
	"context"
	"os"
	"os/signal"
	"syscall"
)

// syncKey is the key used for the sync channel, this channel is closed
// after all graceful state transition is done and the new program should
// start processing things
type syncKey struct{}

var noGracefulCh = make(chan struct{})
var Signal = make(chan struct{})

func init() {
	close(noGracefulCh)
}

func WithSync(ctx context.Context) (context.Context, func()) {
	ch := make(chan struct{})
	return context.WithValue(ctx, syncKey{}, ch), func() { close(ch) }
}

// Sync returns the channel put in by WithSync, if no such
// channel exists it returns a closed channel.
func Sync(ctx context.Context) chan struct{} {
	v := ctx.Value(syncKey{})
	if v == nil {
		return noGracefulCh
	}
	return v.(chan struct{})
}

func Setup(ctx context.Context) context.Context {
	signalCh := make(chan os.Signal, 1)
	signal.Notify(signalCh, syscall.SIGUSR2)

	for {
		select {
		case <-ctx.Done():
		case <-signalCh:
			close(Signal)
		}
	}
}
