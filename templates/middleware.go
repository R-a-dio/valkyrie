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
const ThemeDefault = "default-dark"
const ThemeAdminDefault = "admin-dark"

func cookieEncode(theme string, overwrite_dj, overwrite_holiday bool) string {
	switch {
	case overwrite_dj && overwrite_holiday:
		return theme + ":11"
	case !overwrite_dj && overwrite_holiday:
		return theme + ":01"
	case overwrite_dj && !overwrite_holiday:
		return theme + ":10"
	default:
		return theme + ":00"
	}
}

func cookieDecode(value string) (theme string, overwrite_dj, overwrite_holiday bool) {
	start := len(value) - 3
	if start < 0 {
		return value, false, false
	}
	if value[start] != ':' {
		return value, false, false
	}

	return value[:start], value[start+1] == '1', value[start+2] == '1'
}

type ThemeValues struct {
	resolver func(string) string
	holiday  util.TypedValue[radio.ThemeName]
	dj       util.TypedValue[radio.ThemeName]
}

func NewThemeValues(resolver func(string) string) *ThemeValues {
	if resolver == nil {
		resolver = func(s string) string { return s }
	}

	return &ThemeValues{
		resolver: resolver,
	}
}

func (tv *ThemeValues) resolve(theme radio.ThemeName) radio.ThemeName {
	if theme == "" {
		return theme
	}
	// resolve the theme that was passed in
	resolved := tv.resolver(theme)
	if resolved != theme {
		// if input and output are not the same it means the theme didn't exist
		// and we shouldn't use it, so just unset it
		return ""
	}

	return resolved
}

func (tv *ThemeValues) StoreHoliday(theme radio.ThemeName) {
	tv.holiday.Store(tv.resolve(theme))
}

func (tv *ThemeValues) StoreDJ(theme radio.ThemeName) {
	tv.dj.Store(tv.resolve(theme))
}

// ThemeCtx adds a theme entry into the context of the request, that is acquirable by
// calling GetTheme on the request context.
//
// What theme to insert is a priority system that looks like this:
//  1. user-picked (with overwrite-holiday enabled)
//  2. holiday-theme
//  3. user-picked (with overwrite-dj enabled)
//  4. dj-theme
//  5. user-picked
//  6. default-theme
func ThemeCtx(tv *ThemeValues) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// figure out what default and cookie to use
			theme, cookieName := ThemeDefault, ThemeCookieName

			isAdmin := strings.HasPrefix(r.URL.Path, "/admin")
			if isAdmin {
				// different for the admin route
				theme, cookieName = ThemeAdminDefault, ThemeAdminCookieName
			}

			// retrieve our cookie
			if cookie, err := r.Cookie(cookieName); err == nil {
				theme = cookie.Value
			}

			// then run the theme through the decider, this will handle holiday themes, dj themes and the
			// user configured stuff from the cookie
			if !isAdmin { // but only if we're not loading admin pages, the special themes wont have support for those
				theme = tv.decide(theme)
			}

			// or if the user set a theme in the url query (?theme=<thing>) we use that and ignore
			// the cookie setting completely
			if tmp := r.URL.Query().Get("theme"); tmp != "" {
				theme = tmp
			}

			ctx := SetTheme(r.Context(), theme, false)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

func (tv *ThemeValues) decide(value string) radio.ThemeName {
	name, overwrite_dj, overwrite_holiday := cookieDecode(value)

	if overwrite_holiday {
		return name
	}

	if holidayTheme := tv.holiday.Load(); holidayTheme != "" {
		return holidayTheme
	}

	if overwrite_dj {
		return name
	}

	if djTheme := tv.dj.Load(); djTheme != "" {
		return djTheme
	}

	return name
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

// SetTheme sets a theme in the context given, does nothing if a theme already exists
// unless override is set to true
func SetTheme(ctx context.Context, theme string, override bool) context.Context {
	if !override {
		if exists := ctx.Value(themeKey{}); exists != nil {
			return ctx
		}
	}
	return context.WithValue(ctx, themeKey{}, theme)
}

func SetThemeHandler(cookieName string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// get the theme value from the url query, we do not want to call ParseForm
		// because otherwise we populate a whole bunch of fields on the request that
		// we might want to proxy back into the server later
		query := r.URL.Query()
		theme := query.Get("theme")
		overwrite_dj := query.Has("overwrite-dj")
		overwrite_holiday := query.Has("overwrite-holiday")

		http.SetCookie(w, &http.Cookie{
			Name:     cookieName,
			Value:    cookieEncode(theme, overwrite_dj, overwrite_holiday),
			Path:     "/",
			SameSite: http.SameSiteStrictMode,
			Expires:  time.Now().Add(time.Hour * 24 * 400),
			HttpOnly: true,
		})

		// redirect the request back to where we came from, this isn't guaranteed to work but
		// should have a pretty high chance of working
		r, ok := util.RedirectBack(r)
		if !ok {
			// somehow failed to redirect, return user to home page
			r = util.RedirectPath(r, "/")
		}

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

		// then redirect the request internally to the top of the stack
		err := util.RedirectToServer(w, r)
		if err != nil {
			hlog.FromRequest(r).Error().Err(err).Msg("failed to redirect SetThemeHandler request")
			w.WriteHeader(http.StatusOK)
			return
		}
	})
}
