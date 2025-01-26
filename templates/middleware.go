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

func cookieEncode(theme radio.ThemeName, overwrite_dj, overwrite_holiday bool) string {
	switch {
	case overwrite_dj && overwrite_holiday:
		return string(theme + ":11")
	case !overwrite_dj && overwrite_holiday:
		return string(theme + ":01")
	case overwrite_dj && !overwrite_holiday:
		return string(theme + ":10")
	default:
		return string(theme + ":00")
	}
}

func cookieDecode(value string) (theme radio.ThemeName, overwrite_dj, overwrite_holiday bool) {
	start := len(value) - 3
	if start < 0 {
		return radio.ThemeName(value), false, false
	}
	if value[start] != ':' {
		return radio.ThemeName(value), false, false
	}

	return radio.ThemeName(value[:start]), value[start+1] == '1', value[start+2] == '1'
}

type ThemeValues struct {
	resolver func(radio.ThemeName) radio.ThemeName
	holiday  util.TypedValue[radio.ThemeName]
	dj       util.TypedValue[radio.ThemeName]
}

func NewThemeValues(resolver func(radio.ThemeName) radio.ThemeName) *ThemeValues {
	if resolver == nil {
		resolver = func(s radio.ThemeName) radio.ThemeName { return s }
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

func (tv *ThemeValues) LoadHoliday() radio.ThemeName {
	return tv.holiday.Load()
}

func (tv *ThemeValues) StoreDJ(theme radio.ThemeName) {
	tv.dj.Store(tv.resolve(theme))
}

// ThemeCtx adds a theme entry into the context of the request, that is acquirable by
// calling GetTheme on the request context.
//
// What theme to insert is a priority system that looks like this:
//  1. user-picked (if holiday-theme is set and overwrite-holiday enabled)
//  2. holiday-theme
//  3. user-picked (if dj-theme is set and overwrite-dj enabled)
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

			var themeResolved radio.ThemeName
			// then run the theme through the decider, this will handle holiday themes, dj themes and the
			// user configured stuff from the cookie
			if !isAdmin { // but only if we're not loading admin pages, the special themes wont have support for those
				themeResolved = tv.decide(theme)
			} else {
				themeResolved = tv.resolve(radio.ThemeName(theme))
			}

			// or if the user set a theme in the url query (?theme=<thing>) we use that and ignore
			// the cookie setting completely
			if tmp := r.URL.Query().Get("theme"); tmp != "" {
				themeResolved = tv.resolve(radio.ThemeName(tmp))
			}

			// now it's technically possible for a user to have selected an admin theme here and
			// that means they will get the admin templates to render without a user account
			//
			// so we check if we are requesting a public route and if what we resolved to was an admin theme
			if !isAdmin && IsAdminTheme(themeResolved) {
				// if both of these are true, we yeet the user back to the default public theme
				themeResolved = ThemeDefault
			}

			ctx := SetTheme(r.Context(), themeResolved, false)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

func (tv *ThemeValues) decide(value string) radio.ThemeName {
	name, overwrite_dj, overwrite_holiday := cookieDecode(value)

	if holidayTheme := tv.holiday.Load(); holidayTheme != "" {
		if overwrite_holiday {
			return name
		}
		return holidayTheme
	}

	if djTheme := tv.dj.Load(); djTheme != "" {
		if overwrite_dj {
			return name
		}
		return djTheme
	}

	return name
}

// GetTheme returns the theme from the given context.
// panics if no ThemeKey is found, so make sure ThemeCtx is used
func GetTheme(ctx context.Context) radio.ThemeName {
	v := ctx.Value(themeKey{})
	if v == nil {
		panic("GetTheme called without ThemeCtx used")
	}

	theme, ok := v.(radio.ThemeName)
	if !ok {
		panic("non-string themeKey found in context")
	}

	return theme
}

// SetTheme sets a theme in the context given, does nothing if a theme already exists
// unless override is set to true
func SetTheme(ctx context.Context, theme radio.ThemeName, override bool) context.Context {
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
		theme := radio.ThemeName(query.Get("theme")) // TODO: use a resolver here?
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
			hlog.FromRequest(r).Error().Ctx(r.Context()).Err(err).Msg("failed to redirect SetThemeHandler request")
			w.WriteHeader(http.StatusOK)
			return
		}
	})
}
