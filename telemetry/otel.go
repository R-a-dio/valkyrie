package telemetry

import (
	"context"

	"github.com/R-a-dio/valkyrie/config"
	"github.com/R-a-dio/valkyrie/rpc"
	"github.com/R-a-dio/valkyrie/storage/mariadb"
	"github.com/R-a-dio/valkyrie/website"
	"github.com/XSAM/otelsql"
	"github.com/jmoiron/sqlx"
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

	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(propagation.TraceContext{}, propagation.Baggage{}))
	// done setting up, swap global functions to inject telemetry
	mariadb.DatabaseConnectFunc = DatabaseConnect
	website.NewRouter = NewRouter
	rpc.NewGrpcServer = NewGrpcServer
	rpc.GrpcDial = GrpcDial

	closeFn := func() {
		tp.Shutdown(context.Background())
		mp.Shutdown(context.Background())
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
	return grpc.Dial(addr, opts...)
}
