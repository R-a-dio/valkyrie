package cmd

import (
	"context"
	"os"
	"os/signal"
	"sync/atomic"
	"syscall"

	"github.com/R-a-dio/valkyrie/config"
)

// ExecuteFn is the function signature used by most things that can run in
// their own process in this codebase. It's effectively the type of our
// 'main' function.
type ExecuteFn func(context.Context, config.Config) error

type usr2Key struct{}

type usr2Handler struct {
	called atomic.Bool
}

func USR2Signal(ctx context.Context) <-chan os.Signal {
	v := ctx.Value(usr2Key{})
	if v == nil {
		return nil
	}

	h := v.(*usr2Handler)
	h.called.Store(true)

	ch := make(chan os.Signal, 2)
	signal.Notify(ch, syscall.SIGUSR2)
	return ch
}

func WithUSR2Signal(ctx context.Context) (_ context.Context, wasCalled func() bool) {
	h := &usr2Handler{}
	ctx = context.WithValue(ctx, usr2Key{}, h)
	return ctx, func() bool { return h.called.Load() }
}
