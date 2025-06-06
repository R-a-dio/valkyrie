package telemetry

import (
	"context"
	"runtime"
	"time"

	"github.com/R-a-dio/valkyrie/config"
	"github.com/R-a-dio/valkyrie/util/buildinfo"
	"github.com/grafana/pyroscope-go"
	"github.com/rs/zerolog"
)

func IsPyroscopeEnabled(cfg config.Config) bool {
	return cfg.Conf().Telemetry.Pyroscope.Endpoint != ""
}

func InitPyroscope(ctx context.Context, cfg config.Config, service string) (Profiler, error) {
	if !IsPyroscopeEnabled(cfg) {
		// disable if there is no endpoint configured
		return noopPyroscope{}, nil
	}
	runtime.SetMutexProfileFraction(5)
	runtime.SetBlockProfileRate(5)

	conf := pyroscope.Config{
		ApplicationName: "radio." + service,
		ServerAddress:   string(cfg.Conf().Telemetry.Pyroscope.Endpoint),
		Tags: map[string]string{
			"service_git_ref":    buildinfo.GitRef,
			"service_repository": "https://github.com/r-a-dio/valkyrie",
		},
		UploadRate: time.Duration(cfg.Conf().Telemetry.Pyroscope.UploadRate),
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

	zerolog.Ctx(ctx).Info().Ctx(ctx).Msg("starting pyroscope profiling")

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
