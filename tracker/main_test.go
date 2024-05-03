package tracker

import (
	"context"
	"fmt"
	"math/rand/v2"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/R-a-dio/valkyrie/mocks"
	"github.com/stretchr/testify/assert"
)

func TestPeriodicallyUpdateListeners(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan struct{})

	recorder := NewRecorder(ctx)
	var last atomic.Int64
	var count int
	var closeOnce sync.Once

	manager := &mocks.ManagerServiceMock{
		UpdateListenersFunc: func(contextMoqParam context.Context, new int64) error {
			// we're done after 10 updates
			if count++; count > 10 {
				closeOnce.Do(func() {
					close(done)
				})
				return nil
			}
			// every 5 updates return an error
			if count%5 == 0 {
				return fmt.Errorf("that's an error")
			}

			// otherwise our new value should equal what we set it to previously
			if !assert.Equal(t, last.Load(), new) {
				closeOnce.Do(func() {
					close(done)
				})
				return nil
			}

			adjustment := rand.Int64()
			recorder.listenerAmount.Store(adjustment)
			last.Store(adjustment)

			return nil
		},
	}

	finished := make(chan struct{})
	go func() {
		defer close(finished)
		PeriodicallyUpdateListeners(ctx, manager, recorder, time.Millisecond*10)
	}()

	// wait for the 10 updates
	<-done

	// cancel the context we gave the function, it should clean
	// itself up
	cancel()

	select {
	case <-finished:
	case <-time.After(eventuallyDelay):
		t.Error("failed to cleanup")
	}
}
