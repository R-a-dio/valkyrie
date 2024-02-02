package telemetry

import (
	"context"

	"github.com/R-a-dio/valkyrie/rpc"
	"github.com/R-a-dio/valkyrie/storage/mariadb"
	"github.com/R-a-dio/valkyrie/website"
	"github.com/XSAM/otelsql"
	"github.com/jmoiron/sqlx"
	"go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	"go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.24.0"
	"google.golang.org/grpc"
)

func Init(ctx context.Context, service string) (*trace.TracerProvider, error) {
	exporter, err := otlptracegrpc.New(ctx, otlptracegrpc.WithInsecure(),
		otlptracegrpc.WithEndpoint(":5081"),
		otlptracegrpc.WithHeaders(map[string]string{
			"Authorization": "",
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
		trace.WithBatcher(exporter),
		trace.WithResource(res),
	)

	otel.SetTracerProvider(tp)
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(propagation.TraceContext{}, propagation.Baggage{}))

	// done setting up, swap global functions to inject telemetry
	mariadb.DatabaseConnectFunc = DatabaseConnect
	website.NewRouter = NewRouter
	rpc.NewGrpcServer = NewGrpcServer
	rpc.GrpcDial = GrpcDial

	return tp, err
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
