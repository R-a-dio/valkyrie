package telemetry

import (
	"context"
	"runtime"
	"time"

	"github.com/R-a-dio/valkyrie/config"
	"github.com/grafana/pyroscope-go"
)

func IsPyroscopeEnabled(cfg config.Config) bool {
	return cfg.Conf().Telemetry.Pyroscope.Endpoint != ""
}

func InitPyroscope(ctx context.Context, cfg config.Config, service string) (Profiler, error) {
	if IsPyroscopeEnabled(cfg) {
		// disable if there is no endpoint configured
		return noopPyroscope{}, nil
	}
	runtime.SetMutexProfileFraction(5)
	runtime.SetBlockProfileRate(5)

	conf := pyroscope.Config{
		ApplicationName: "radio:" + service,
		ServerAddress:   string(cfg.Conf().Telemetry.Pyroscope.Endpoint),
		UploadRate:      time.Duration(cfg.Conf().Telemetry.Pyroscope.UploadRate),
		ProfileTypes: []pyroscope.ProfileType{
			pyroscope.ProfileCPU,
			pyroscope.ProfileAllocObjects,
			pyroscope.ProfileAllocSpace,
			pyroscope.ProfileInuseObjects,
			pyroscope.ProfileInuseSpace,
			// these profile types are optional:
			pyroscope.ProfileGoroutines,
			pyroscope.ProfileMutexCount,
			pyroscope.ProfileMutexDuration,
			pyroscope.ProfileBlockCount,
			pyroscope.ProfileBlockDuration,
		},
	}

	profiler, err := pyroscope.Start(conf)
	if err != nil {
		return nil, err
	}

	return profiler, nil
}

type Profiler interface {
	Stop() error
	Flush(wait bool)
}

type noopPyroscope struct{}

func (noopPyroscope) Stop() error {
	return nil
}

func (noopPyroscope) Flush(wait bool) {}
