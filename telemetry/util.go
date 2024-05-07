package telemetry

import (
	"net/http"
	"strings"

	"go.opentelemetry.io/otel/attribute"
)

func HeadersToAttributes(headers http.Header) []attribute.KeyValue {
	var res []attribute.KeyValue

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
