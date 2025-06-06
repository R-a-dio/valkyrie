package otelzerolog

import (
	"encoding/json"
	"fmt"
	"math"
	"reflect"
	"time"

	"github.com/R-a-dio/valkyrie/util/pool"
	"github.com/rs/zerolog"
	"go.opentelemetry.io/otel/log"
	"go.opentelemetry.io/otel/log/global"
	"go.opentelemetry.io/otel/trace"
)

func Hook(instrumentation_name, instrumentation_version string) zerolog.Hook {
	logger := global.GetLoggerProvider().Logger(
		instrumentation_name,
		log.WithInstrumentationVersion(instrumentation_version),
	)

	return &hook{
		logger: logger,
		pool: pool.NewPool(func() map[string]any {
			return make(map[string]any, 10)
		}),
	}
}

type hook struct {
	logger log.Logger
	pool   *pool.Pool[map[string]any]
}

func (h hook) Run(e *zerolog.Event, zerolevel zerolog.Level, msg string) {
	if !e.Enabled() { // check if zerolog is enabled
		return
	}

	ctx := e.GetCtx()
	level := convertLevel(zerolevel)

	if !h.logger.Enabled(ctx, log.EnabledParameters{Severity: level}) {
		// check if opentelemetry logging is enabled
		return
	}

	r := log.Record{}
	now := time.Now()
	r.SetSeverity(level)
	r.SetSeverityText(zerolevel.String())

	r.SetBody(log.StringValue(msg))

	r.SetTimestamp(now)
	r.SetObservedTimestamp(now)

	logData := h.pool.Get()
	// create a string that appends } to the end of the buf variable you access via reflection
	ev := fmt.Sprintf("%s}", reflect.ValueOf(e).Elem().FieldByName("buf"))
	_ = json.Unmarshal([]byte(ev), &logData)

	attrs := make([]log.KeyValue, 0, len(logData)+2)
	for k, v := range logData {
		attrs = append(attrs, convertToKeyValue(k, v))
	}

	// clear the map we used, and put it back into our pool
	clear(logData)
	h.pool.Put(logData)

	// add SpanId and TraceId if applicable
	spanCtx := trace.SpanFromContext(ctx).SpanContext()
	if spanCtx.HasSpanID() {
		attrs = append(attrs, log.KeyValue{
			Key:   "SpanId",
			Value: log.StringValue(spanCtx.SpanID().String()),
		})
	}
	if spanCtx.HasTraceID() {
		attrs = append(attrs, log.KeyValue{
			Key:   "TraceId",
			Value: log.StringValue(spanCtx.TraceID().String()),
		})
	}

	r.AddAttributes(attrs...)
	h.logger.Emit(ctx, r)
}

func convertLevel(level zerolog.Level) log.Severity {
	switch level {
	case zerolog.DebugLevel:
		return log.SeverityDebug
	case zerolog.InfoLevel:
		return log.SeverityInfo
	case zerolog.WarnLevel:
		return log.SeverityWarn
	case zerolog.ErrorLevel:
		return log.SeverityError
	case zerolog.PanicLevel:
		return log.SeverityFatal1
	case zerolog.FatalLevel:
		return log.SeverityFatal2
	default:
		return log.SeverityUndefined
	}
}

func convertToKeyValue(key string, value any) log.KeyValue {
	return log.KeyValue{
		Key:   key,
		Value: convertToValue(value),
	}
}

func convertArray(value []any) log.Value {
	values := make([]log.Value, 0, len(value))
	for _, v := range value {
		values = append(values, convertToValue(v))
	}
	return log.SliceValue(values...)
}

func convertMap(value map[string]any) log.Value {
	kvs := make([]log.KeyValue, 0, len(value))
	for k, v := range value {
		kvs = append(kvs, convertToKeyValue(k, v))
	}
	return log.MapValue(kvs...)
}

func convertToValue(value any) log.Value {
	switch value := value.(type) {
	case bool:
		return log.BoolValue(value)
	case float64:
		if _, frac := math.Modf(value); frac == 0.0 {
			return log.Int64Value(int64(value))
		}
		return log.Float64Value(value)
	case string:
		return log.StringValue(value)
	case []any:
		return convertArray(value)
	case map[string]any:
		return convertMap(value)
	}

	// should be unreachable if this only gets input from encoding/json, but handle it
	// anyway by just turning whatever value we got into a string with fmt
	return log.StringValue(fmt.Sprintf("%v", value))
}
