package graceful

import (
	"context"
	"errors"
	"net"
	"os"
	"os/signal"
	"syscall"
)

// syncKey is the key used for the sync channel, this channel is closed
// after all graceful state transition is done and the new program should
// start processing things
type gracefulKey struct{}

var noGracefulCh = make(chan struct{})

var (
	ErrNoGraceful = errors.New("no graceful")
	ErrNotChild   = errors.New("not a child")
)

func init() {
	close(noGracefulCh)
}

func Finish(ctx context.Context) {
	g := get(ctx)
	if g == nil {
		return
	}
	close(g.Sync)
}

// Sync returns the channel put in by WithSync, if no such
// channel exists it returns a closed channel.
func Sync(ctx context.Context) chan struct{} {
	g := get(ctx)
	if g == nil || !g.IsChild {
		// if no key exists we return a channel that is closed
		// this way sync never blocks for the caller
		return noGracefulCh
	}
	return g.Sync
}

func Signal(ctx context.Context) chan struct{} {
	g := get(ctx)
	if g == nil {
		// if no key exists we return a nil that always
		// blocks, since we are not using graceful and it
		// obviously should not be triggering
		return nil
	}
	return g.Signal
}

// Parent returns the unix connection that is connected to our
// parent process.
func Parent(ctx context.Context) (*net.UnixConn, error) {
	g := get(ctx)
	if g == nil {
		return nil, ErrNoGraceful
	}
	if !g.IsChild {
		return nil, ErrNotChild
	}
	if g.Parent != nil {
		return g.Parent, nil
	}

	conn, err := FD2Unix(3)
	if err != nil {
		return nil, err
	}
	return conn, nil
}

func get(ctx context.Context) *Graceful {
	v := ctx.Value(gracefulKey{})
	if v == nil {
		return nil
	}
	g := v.(Graceful)
	return &g
}

// SetupGraceful sets up the signal handler and data structure
// for handling SIGUSR2 with a graceful restart mechanism.
//
// The returned context (or children of) should be used with
// the rest of the functions in this package
func SetupGraceful(ctx context.Context) context.Context {
	signalCh := make(chan os.Signal, 1)
	signal.Notify(signalCh, syscall.SIGUSR2)

	g := newGraceful(ctx, isChild())

	go func() {
		select {
		case <-ctx.Done():
		case <-signalCh:
			close(g.Signal)
		}
	}()

	return g.Context(ctx)
}

// NewGraceful returns a new Graceful, this function should only
// be used for testing purposes
func newGraceful(ctx context.Context, isChild bool) Graceful {
	return Graceful{
		IsChild: isChild,
		Signal:  make(chan struct{}),
		Sync:    make(chan struct{}),
	}
}

func TestGraceful(ctx context.Context, isChild bool) (context.Context, Graceful) {
	var err error
	g := newGraceful(ctx, isChild)
	g.Parent, g.Child, err = SocketPair()
	if err != nil {
		panic("failed to make a pair: " + err.Error())
	}
	return g.Context(ctx), g
}

func TestMarkChild(ctx context.Context) context.Context {
	g := get(ctx)
	// mark ourselves as a child
	g.IsChild = true
	// undo the signal
	g.Signal = make(chan struct{})
	return g.Context(ctx)
}

type Graceful struct {
	// Parent is populated when TestGraceful is used to construct the graceful
	Parent *net.UnixConn
	// Child is populated when TestGraceful is used to construct the graceful
	Child *net.UnixConn
	// IsChild indicates if we are a child process
	IsChild bool
	// signal is closed when a signal is received
	Signal chan struct{}
	// sync is closed after graceful restoration is completed
	Sync chan struct{}
}

func (g Graceful) Context(ctx context.Context) context.Context {
	return context.WithValue(ctx, gracefulKey{}, g)
}
