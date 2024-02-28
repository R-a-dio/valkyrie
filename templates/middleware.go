package templates

import (
	"context"
	"net/http"

	radio "github.com/R-a-dio/valkyrie"
)

type themeKey struct{}

func ThemeCtx(storage radio.StorageService) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ctx := r.Context()
			ctx = SetTheme(ctx, "default-light")
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

func AdminThemeCtx() func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ctx := SetTheme(r.Context(), "admin-light")
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// GetTheme returns the theme from the given context.
// panics if no ThemeKey is found, so make sure ThemeCtx is used
func GetTheme(ctx context.Context) string {
	v := ctx.Value(themeKey{})
	if v == nil {
		panic("GetTheme called without ThemeCtx used")
	}

	theme, ok := v.(string)
	if !ok {
		panic("non-string themeKey found in context")
	}

	return theme
}

func SetTheme(ctx context.Context, theme string) context.Context {
	return context.WithValue(ctx, themeKey{}, theme)
}
