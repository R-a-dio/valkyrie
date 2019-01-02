package main

import (
	"github.com/R-a-dio/valkyrie/cronjobs"
	"github.com/R-a-dio/valkyrie/engine"
)

func init() {
	AddComponent("listenlog", CronjobComponent(cronjobs.ListenLog))
	AddComponent("requestcount", CronjobComponent(cronjobs.RequestCount))
}

func CronjobComponent(fn func(*engine.Engine) error) func(chan<- error) engine.StartFn {
	return func(errCh chan<- error) engine.StartFn {
		return func(e *engine.Engine) (engine.DeferFn, error) {
			go func() {
				errCh <- fn(e)
			}()

			return nil, nil
		}
	}
}
