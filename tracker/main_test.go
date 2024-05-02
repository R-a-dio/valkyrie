package tracker

import (
	"context"
	"fmt"
	"math/rand/v2"
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

	recorder := NewRecorder()
	var last atomic.Int64
	var count int

	manager := &mocks.ManagerServiceMock{
		UpdateListenersFunc: func(contextMoqParam context.Context, new int64) error {
			// we're done after 10 updates
			if count++; count > 10 {
				close(done)
			}
			// every 5 updates return an error
			if count%5 == 0 {
				return fmt.Errorf("that's an error")
			}

			// otherwise our new value should equal what we set it to previously
			if !assert.Equal(t, last.Load(), new) {
				close(done)
			}

			adjustment := rand.Int64()
			recorder.listenerAmount.Store(adjustment)
			last.Store(adjustment)

			return nil
		},
	}

	// set the tickrate a bit higher for testing purposes
	UpdateListenersTickrate = time.Millisecond * 10
	go PeriodicallyUpdateListeners(ctx, manager, recorder)

	<-done
}
