package templates

import (
	"context"
	"net/http"
	"strings"
	"time"

	radio "github.com/R-a-dio/valkyrie"
	"github.com/R-a-dio/valkyrie/util"
	"github.com/rs/zerolog/hlog"
)

type themeKey struct{}

const ThemeCookieName = "theme"
const ThemeAdminCookieName = "admin-theme"

func ThemeCtx(storage radio.StorageService) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			theme := DEFAULT_DIR
			cookieName := ThemeCookieName
			if strings.HasPrefix(r.URL.Path, "/admin") {
				theme = DEFAULT_ADMIN_DIR
				cookieName = ThemeAdminCookieName
			}

			if cookie, err := r.Cookie(cookieName); err == nil {
				theme = cookie.Value
			}
			if tmp := r.URL.Query().Get("theme"); tmp != "" {
				theme = tmp
			}

			ctx := SetTheme(r.Context(), theme, false)
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

func SetTheme(ctx context.Context, theme string, override bool) context.Context {
	if !override {
		if exists := ctx.Value(themeKey{}); exists != nil {
			return ctx
		}
	}
	return context.WithValue(ctx, themeKey{}, theme)
}

func SetThemeHandler(cookieName string, resolve func(string) string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// get the theme value from the url query, we do not want to call ParseForm
		// because otherwise we populate a whole bunch of fields on the request that
		// we might want to proxy back into the server later
		theme := resolve(r.URL.Query().Get("theme"))

		http.SetCookie(w, &http.Cookie{
			Name:     cookieName,
			Value:    theme,
			Path:     "/",
			SameSite: http.SameSiteStrictMode,
			Expires:  time.Now().Add(time.Hour * 24 * 400),
			HttpOnly: true,
		})

		r = util.RedirectBack(r)

		if !util.IsHTMX(r) {
			// not a htmx request so probably no-js, send a http redirect to refresh
			// the page instead with the new cookie set
			http.Redirect(w, r, r.URL.String(), http.StatusFound)
			w.WriteHeader(http.StatusOK)
			return
		}

		// remove the header indicating we are using htmx, since we want a full-page reload
		r.Header.Del("Hx-Request")
		// and change the theme so the new page actually uses our new theme set
		r = r.WithContext(SetTheme(r.Context(), theme, true))

		err := util.RedirectToServer(w, r)
		if err != nil {
			hlog.FromRequest(r).Error().Err(err).Msg("SetThemeHandler")
			w.WriteHeader(http.StatusOK)
			return
		}
	})
}
