package telemetry

import (
	"fmt"
	"net/http"
	"strings"
	"time"

	"go.opentelemetry.io/otel/attribute"
)

func HeadersToAttributes(headers http.Header) []attribute.KeyValue {
	res := make([]attribute.KeyValue, 0, len(headers))
	for name, value := range headers {
		name = "http.request.header." + strings.ToLower(name)
		res = append(res, attribute.StringSlice(name, value))
	}
	return res
}

func requestToOtelAttributes(req *http.Request) []attribute.KeyValue {
	res := HeadersToAttributes(req.Header)
	for name, value := range req.PostForm {
		res = append(res, attribute.StringSlice(strings.ToLower(name), value))
	}
	return res
}

func databaseToValue(value any) attribute.Value {
	switch value := value.(type) {
	case int64:
		return attribute.Int64Value(value)
	case float64:
		return attribute.Float64Value(value)
	case bool:
		return attribute.BoolValue(value)
	case []byte:
		return attribute.StringValue(string(value))
	case string:
		return attribute.StringValue(value)
	case time.Time:
		return attribute.StringValue(value.Format(time.RFC3339))
	}

	// should be unreachable if this only gets input from a sql driver.Value, but handle it
	// anyway by just turning whatever value we got into a string with fmt
	return attribute.StringValue(fmt.Sprintf("%v", value))
}
