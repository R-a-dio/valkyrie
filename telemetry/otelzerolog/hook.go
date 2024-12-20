package otelzerolog

import (
	"encoding/json"
	"fmt"
	"math"
	"reflect"
	"time"

	"github.com/rs/zerolog"
	"go.opentelemetry.io/otel/log"
	"go.opentelemetry.io/otel/log/global"
)

var (
	InstrumentationName    = "github.com/R-a-dio/valkyrie/telemetry/otelzerolog"
	InstrumentationVersion = "0.1.0"
)

func Hook() zerolog.Hook {
	logger := global.GetLoggerProvider().Logger(
		// TODO: make this use proper names and version
		InstrumentationName,
		log.WithInstrumentationVersion(InstrumentationVersion),
	)

	return &hook{logger}
}

type hook struct {
	logger log.Logger
}

func (h hook) Run(e *zerolog.Event, level zerolog.Level, msg string) {
	if !e.Enabled() {
		return
	}

	r := log.Record{}
	ctx := e.GetCtx()
	now := time.Now()

	r.SetBody(log.StringValue(msg))

	r.SetTimestamp(now)
	r.SetObservedTimestamp(now)

	r.SetSeverity(convertLevel(level))
	r.SetSeverityText(level.String())

	logData := make(map[string]interface{})
	// create a string that appends } to the end of the buf variable you access via reflection
	ev := fmt.Sprintf("%s}", reflect.ValueOf(e).Elem().FieldByName("buf"))
	_ = json.Unmarshal([]byte(ev), &logData)

	for k, v := range logData {
		r.AddAttributes(convertToKeyValue(k, v))
	}

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
