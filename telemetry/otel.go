package telemetry

import (
	"context"
	"time"

	"github.com/R-a-dio/valkyrie/config"
	"github.com/R-a-dio/valkyrie/rpc"
	"github.com/R-a-dio/valkyrie/storage/mariadb"
	"github.com/R-a-dio/valkyrie/website"
	"github.com/XSAM/otelsql"
	"github.com/agoda-com/opentelemetry-go/otelzerolog"
	"github.com/agoda-com/opentelemetry-logs-go/exporters/otlp/otlplogs"
	"github.com/agoda-com/opentelemetry-logs-go/exporters/otlp/otlplogs/otlplogsgrpc"
	logsSDK "github.com/agoda-com/opentelemetry-logs-go/sdk/logs"
	"github.com/jmoiron/sqlx"
	"github.com/prometheus/client_golang/prometheus/collectors"
	"github.com/prometheus/client_golang/prometheus/push"
	"github.com/rs/zerolog"
	"go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetricgrpc"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/resource"
	"go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.24.0"
	"google.golang.org/grpc"
)

// temporary until otel gets log handling
var LogProvider *logsSDK.LoggerProvider
var logHook *otelzerolog.Hook

var Hook = zerolog.HookFunc(func(e *zerolog.Event, level zerolog.Level, message string) {
	if logHook == nil {
		return
	}

	logHook.Run(e, level, message)
})

func Init(ctx context.Context, cfg config.Config, service string) (func(), error) {
	tp, err := InitTracer(ctx, cfg, service)
	if err != nil {
		return nil, err
	}
	otel.SetTracerProvider(tp)

	mp, err := InitMetric(ctx, cfg, service)
	if err != nil {
		return nil, err
	}
	otel.SetMeterProvider(mp)

	lp, err := InitLogs(ctx, cfg, service)
	if err != nil {
		return nil, err
	}
	// otel.SetLogProvider(lp)
	LogProvider = lp
	logHook = otelzerolog.NewHook(lp)

	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(propagation.TraceContext{}, propagation.Baggage{}))
	// done setting up, swap global functions to inject telemetry
	mariadb.DatabaseConnectFunc = DatabaseConnect
	website.NewRouter = NewRouter
	rpc.NewGrpcServer = NewGrpcServer
	rpc.GrpcDial = GrpcDial

	// we want runtime statistics, but the current opentelemetry one is kinda garbage.
	// So use the prometheus library to collect them instead
	/*runtime.Start(
		runtime.WithMeterProvider(mp),
		runtime.WithMinimumReadMemStatsInterval(time.Second*30),
	)*/
	rc := collectors.NewGoCollector(
		collectors.WithGoCollectorRuntimeMetrics(collectors.MetricsAll),
	)

	pusher := push.New(cfg.Conf().Telemetry.PrometheusEndpoint, "radio:"+service).
		Collector(rc)
	go func() {
		ticker := time.NewTicker(time.Second * 15)
		for {
			select {
			case <-ticker.C:
				err := pusher.AddContext(ctx)
				if err != nil {
					zerolog.Ctx(ctx).Error().Err(err).Msg("failed to prometheus push")
				}
			case <-ctx.Done():
				return
			}
		}
	}()

	closeFn := func() {
		tp.Shutdown(context.Background())
		mp.Shutdown(context.Background())
		lp.Shutdown(context.Background())
	}

	return closeFn, err
}

func InitTracer(ctx context.Context, cfg config.Config, service string) (*trace.TracerProvider, error) {
	conf := cfg.Conf().Telemetry

	trace_exporter, err := otlptracegrpc.New(ctx, otlptracegrpc.WithInsecure(),
		otlptracegrpc.WithEndpoint(conf.Endpoint),
		otlptracegrpc.WithHeaders(map[string]string{
			"Authorization": conf.Auth,
			"organization":  "default",
			"stream-name":   "default",
		}),
	)
	if err != nil {
		return nil, err
	}

	res, err := resource.Merge(resource.Default(), resource.Environment())
	if err != nil {
		return nil, err
	}
	res, err = resource.Merge(res, resource.NewWithAttributes(semconv.SchemaURL, semconv.ServiceName("radio:"+service)))
	if err != nil {
		return nil, err
	}

	tp := trace.NewTracerProvider(
		trace.WithSampler(trace.AlwaysSample()),
		trace.WithBatcher(trace_exporter),
		trace.WithResource(res),
	)

	otel.SetTracerProvider(tp)
	return tp, nil
}

func InitMetric(ctx context.Context, cfg config.Config, service string) (*metric.MeterProvider, error) {
	conf := cfg.Conf().Telemetry

	metric_exporter, err := otlpmetricgrpc.New(ctx, otlpmetricgrpc.WithInsecure(),
		otlpmetricgrpc.WithEndpoint(conf.Endpoint),
		otlpmetricgrpc.WithHeaders(map[string]string{
			"Authorization": conf.Auth,
			"organization":  "default",
			"stream-name":   "default",
		}),
	)
	if err != nil {
		return nil, err
	}

	res, err := resource.Merge(resource.Default(), resource.Environment())
	if err != nil {
		return nil, err
	}
	res, err = resource.Merge(res, resource.NewWithAttributes(semconv.SchemaURL, semconv.ServiceName("radio:"+service)))
	if err != nil {
		return nil, err
	}

	mp := metric.NewMeterProvider(
		metric.WithReader(metric.NewPeriodicReader(metric_exporter)),
		metric.WithResource(res),
	)

	otel.SetMeterProvider(mp)
	return mp, nil
}

func InitLogs(ctx context.Context, cfg config.Config, service string) (*logsSDK.LoggerProvider, error) {
	conf := cfg.Conf().Telemetry

	logs_exporter, err := otlplogs.NewExporter(ctx,
		otlplogs.WithClient(
			otlplogsgrpc.NewClient(otlplogsgrpc.WithInsecure(),
				otlplogsgrpc.WithEndpoint(conf.Endpoint),
			),
		),
	)
	if err != nil {
		return nil, err
	}

	res, err := resource.Merge(resource.Default(), resource.Environment())
	if err != nil {
		return nil, err
	}
	res, err = resource.Merge(res, resource.NewWithAttributes(semconv.SchemaURL, semconv.ServiceName("radio:"+service)))
	if err != nil {
		return nil, err
	}

	lp := logsSDK.NewLoggerProvider(
		logsSDK.WithBatcher(logs_exporter),
		logsSDK.WithResource(res),
	)

	return lp, nil
}

// DatabaseConnect applies telemetry to a database/sql driver
func DatabaseConnect(ctx context.Context, driverName string, dataSourceName string) (*sqlx.DB, error) {
	db, err := otelsql.Open(driverName, dataSourceName, otelsql.WithSpanOptions(otelsql.SpanOptions{
		DisableErrSkip: true,
	}))
	if err != nil {
		return nil, err
	}

	if err = db.PingContext(ctx); err != nil {
		return nil, err
	}

	err = otelsql.RegisterDBStatsMetrics(db, otelsql.WithAttributes(
		semconv.DBSystemKey.String(driverName),
	))
	if err != nil {
		return nil, err
	}

	return sqlx.NewDb(db, driverName), nil
}

func NewGrpcServer(opts ...grpc.ServerOption) *grpc.Server {
	opts = append(opts, grpc.StatsHandler(otelgrpc.NewServerHandler()))
	return grpc.NewServer(opts...)
}

func GrpcDial(addr string, opts ...grpc.DialOption) (*grpc.ClientConn, error) {
	opts = append(opts,
		grpc.WithStatsHandler(otelgrpc.NewClientHandler()),
	)
	return grpc.NewClient(addr, opts...)
}
