package telemetry

import (
	"context"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"
)

type cleanFunctionCase struct {
	value  string
	expect string
}

func TestGetFunctionName(t *testing.T) {
	table := []cleanFunctionCase{
		{"github.com/go-chi/chi/v5/middleware.RealIP", "RealIP"},
		{"github.com/R-a-dio/valkyrie/website.Execute.NewHandler.func2", "NewHandler"},
		{"github.com/R-a-dio/valkyrie/website.Execute.RemoteAddrHandler.func3", "RemoteAddrHandler"},
		{"github.com/R-a-dio/valkyrie/website.Execute.UserAgentHandler.func4", "UserAgentHandler"},
		{"github.com/R-a-dio/valkyrie/website.Execute.RequestIDHandler.func5", "RequestIDHandler"},
		{"github.com/R-a-dio/valkyrie/website.Execute.URLHandler.func6", "URLHandler"},
		{"github.com/R-a-dio/valkyrie/website.Execute.MethodHandler.func7", "MethodHandler"},
		{"github.com/R-a-dio/valkyrie/website.Execute.ProtoHandler.func8", "ProtoHandler"},
		{"github.com/R-a-dio/valkyrie/website.Execute.CustomHeaderHandler.func9", "CustomHeaderHandler"},
		{"github.com/R-a-dio/valkyrie/website/middleware.Authentication.UserMiddleware-fm", "UserMiddleware"},
	}

	for _, c := range table {
		v := cleanFunctionName(c.value)
		if v != c.expect {
			t.Errorf("%s != %s", v, c.expect)
		}
	}
}

func TestDisableTracing(t *testing.T) {
	ctx := context.Background()

	r := httptest.NewRequestWithContext(ctx, "", "/", nil)
	require.True(t, filterSSEEndpoints(r))

	r = httptest.NewRequestWithContext(ctx, "", "/v1/sse?test", nil)
	require.False(t, filterSSEEndpoints(r))
}

func BenchmarkDisableTracing(b *testing.B) {
	ctx := context.Background()
	rTrue := httptest.NewRequestWithContext(ctx, "", "/schedule", nil)
	rFalse := httptest.NewRequestWithContext(ctx, "", "/v1/sse", nil)

	b.Run("noskip", func(b *testing.B) {
		for range b.N {
			filterSSEEndpoints(rTrue)
		}
	})

	b.Run("skip", func(b *testing.B) {
		for range b.N {
			filterSSEEndpoints(rFalse)
		}
	})
}
