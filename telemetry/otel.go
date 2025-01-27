package telemetry

import (
	"context"
	"database/sql/driver"
	"strconv"
	"time"

	"github.com/R-a-dio/valkyrie/config"
	"github.com/R-a-dio/valkyrie/rpc"
	"github.com/R-a-dio/valkyrie/storage/mariadb"
	"github.com/R-a-dio/valkyrie/website"
	"github.com/XSAM/otelsql"
	"github.com/jmoiron/sqlx"
	"github.com/prometheus/client_golang/prometheus/collectors"
	"github.com/prometheus/client_golang/prometheus/push"
	"github.com/rs/zerolog"
	"go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/otlp/otlplog/otlploggrpc"
	"go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetricgrpc"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/log/global"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/log"
	"go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/resource"
	"go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.26.0"
	"google.golang.org/grpc"
)

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
	// swap the next two lines once otlplog goes stable
	global.SetLoggerProvider(lp)
	// otel.SetLogProvider(lp)

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
					zerolog.Ctx(ctx).Error().Ctx(ctx).Err(err).Msg("failed to prometheus push")
				}
			case <-ctx.Done():
				return
			}
		}
	}()

	closeFn := func() {
		// this function will most likely run after our global context is already canceled so
		// remove that cancel from it
		ctx = context.WithoutCancel(ctx)
		// then at our own timeout so we don't wait forever on telemetry shutdown
		ctx, cancel := context.WithTimeout(ctx, time.Second*10)
		defer cancel()
		tp.Shutdown(ctx)
		mp.Shutdown(ctx)
		lp.Shutdown(ctx)
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

func InitLogs(ctx context.Context, cfg config.Config, service string) (*log.LoggerProvider, error) {
	conf := cfg.Conf().Telemetry

	logs_exporter, err := otlploggrpc.New(ctx,
		otlploggrpc.WithInsecure(),
		otlploggrpc.WithEndpoint(conf.Endpoint),
		otlploggrpc.WithHeaders(map[string]string{
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

	lp := log.NewLoggerProvider(
		log.WithProcessor(log.NewBatchProcessor(logs_exporter)),
		log.WithResource(res),
	)

	return lp, nil
}

// DatabaseConnect applies telemetry to a database/sql driver
func DatabaseConnect(ctx context.Context, driverName string, dataSourceName string) (*sqlx.DB, error) {
	db, err := otelsql.Open(driverName, dataSourceName,
		otelsql.WithSpanOptions(otelsql.SpanOptions{DisableErrSkip: true}),
		otelsql.WithAttributesGetter(addSQLParameters),
	)
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

func addSQLParameters(ctx context.Context, method otelsql.Method, query string, args []driver.NamedValue) []attribute.KeyValue {
	attrs := make([]attribute.KeyValue, len(args))
	for i, arg := range args {
		name := arg.Name
		if arg.Name == "" {
			name = strconv.Itoa(arg.Ordinal)
		}
		attrs[i].Key = attribute.Key("db.operation.parameter." + name)
		attrs[i].Value = databaseToValue(arg.Value)
	}
	return attrs
}

var originalNewGrpcServer = rpc.NewGrpcServer

func NewGrpcServer(ctx context.Context, opts ...grpc.ServerOption) *grpc.Server {
	opts = append(opts, grpc.StatsHandler(otelgrpc.NewServerHandler()))
	return originalNewGrpcServer(ctx, opts...)
}

var originalGrpcDial = rpc.GrpcDial

func GrpcDial(addr string, opts ...grpc.DialOption) (*grpc.ClientConn, error) {
	opts = append(opts,
		grpc.WithStatsHandler(otelgrpc.NewClientHandler()),
	)
	return originalGrpcDial(addr, opts...)
}
