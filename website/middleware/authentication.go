package middleware

import (
	"context"
	"encoding/json"
	"html/template"
	"net/http"
	"strings"
	"time"

	radio "github.com/R-a-dio/valkyrie"
	"github.com/R-a-dio/valkyrie/errors"
	"github.com/R-a-dio/valkyrie/templates"
	"github.com/alexedwards/scs/v2"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/httprate"
	"github.com/gorilla/csrf"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/hlog"
)

// keys used in sessions
//
// they can't be typed because of the sessions API wanting strings
const (
	usernameKey           = "admin-username"
	failedLoginKey        = "admin-failed-login"
	failedLoginMessageKey = "admin-failed-login-message"
)

const MAX_USERNAME_LENGTH = 50

type userContextKey struct{}

func NewAuthentication(storage radio.StorageService, tmpl templates.Executor, sessions *scs.SessionManager) Authentication {
	return &authentication{
		storage:   storage,
		sessions:  sessions,
		templates: tmpl,
	}
}

type Authentication interface {
	// UserMiddleware is a middleware that adds the current user to the request
	// context if available. Retrievable by using UserFromContext.
	UserMiddleware(http.Handler) http.Handler
	// LoginMiddleware forces users to login before being able to use the handlers
	// behind this middleware, a users account has to be in an Active state for it
	// to be allowed access.
	LoginMiddleware(http.Handler) http.Handler
	// GetLogin returns the login page.
	GetLogin(http.ResponseWriter, *http.Request)
	// PostLogin handles login form submission.
	PostLogin(http.ResponseWriter, *http.Request)
	// LogoutHandler logs the user out if they are currently logged in.
	LogoutHandler(http.ResponseWriter, *http.Request)
}

type authentication struct {
	storage   radio.StorageService
	sessions  *scs.SessionManager
	templates templates.Executor
}

func RequirePermission(perm radio.UserPermission, handler http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user := UserFromContext(r.Context())
		if !user.IsValid() {
			// no user, clearly doesn't have permission
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}
		if !user.UserPermissions.Has(perm) {
			// user doesn't have required perm
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}

		handler(w, r)
	}
}

// UserMiddleware adds the currently logged in radio.User to the request if available
func (a authentication) UserMiddleware(next http.Handler) http.Handler {
	const op errors.Op = "admin/authentication.UserMiddleware"

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()

		username := a.sessions.GetString(ctx, usernameKey)

		// no known username
		if username == "" {
			next.ServeHTTP(w, RequestWithUser(r, nil))
			return
		}

		user, err := a.storage.User(ctx).Get(username)
		if err != nil {
			// username doesn't exist, log the error
			err = errors.E(op, err)
			hlog.FromRequest(r).Warn().Ctx(ctx).Err(err).Msg("failed to retrieve user from session")
			// then send them on their way as unauthenticated
			next.ServeHTTP(w, RequestWithUser(r, nil))
			return
		}

		r = RequestWithUser(r, user)
		// also add no-cache headers to any authenticated request
		middleware.NoCache(next).ServeHTTP(w, r)
	})
}

// LoginMiddleware makes all routes require requests to be from logged in users
// with permissions Active (active user account). Otherwise they get redirected
// to a login page
func (a authentication) LoginMiddleware(next http.Handler) http.Handler {
	const op errors.Op = "admin/authentication.LoginMiddleware"

	// setup ratelimiting to the POST handler
	limiter := httprate.Limit(3, 1*time.Minute,
		httprate.WithKeyByIP(), // IP address rate limiting
		httprate.WithKeyFuncs(func(r *http.Request) (string, error) {
			return r.FormValue("username"), nil // login username rate limiting
		}),
		httprate.WithLimitHandler(func(w http.ResponseWriter, r *http.Request) { // log things hitting the limit
			hlog.FromRequest(r).Warn().Ctx(r.Context()).
				Str("username", r.FormValue("username")).
				Str("ip", r.RemoteAddr).
				Msg("hit ratelimiter")
		}),
	)

	postHandler := limiter(http.HandlerFunc(a.PostLogin))

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		username := a.sessions.GetString(ctx, usernameKey)

		// no known username yet, so we're not logged in
		if username == "" {
			if r.Method == http.MethodPost {
				// if it's a POST try and see if they're trying to login
				postHandler.ServeHTTP(w, r)
			} else {
				// otherwise send them to the login form
				a.GetLogin(w, r)
			}
			return
		}

		// we have a username, this suggests we are logged in, try and retrieve
		// the permissions for the user and check if it's an active account
		user, err := a.storage.User(ctx).Get(username)
		if err != nil {
			if errors.Is(errors.UserUnknown, err) {
				// unknown user, we force log them out
				a.LogoutHandler(w, r)
				return
			}

			err = errors.E(op, err)
			// other unknown error, just internal server and log stuff
			http.Error(w,
				http.StatusText(http.StatusInternalServerError),
				http.StatusInternalServerError)
			hlog.FromRequest(r).Error().Ctx(ctx).Err(err).Msg("")
			return
		}

		if !user.UserPermissions.Has(radio.PermActive) {
			// user isn't active, so log them out
			a.LogoutHandler(w, r)
			return
		}

		// otherwise, user is active so forward them to their destination
		r = RequestWithUser(r, user)
		// also add no-cache headers to any authenticated request
		middleware.NoCache(next).ServeHTTP(w, r)
	})
}

func (a *authentication) GetLogin(w http.ResponseWriter, r *http.Request) {
	const op errors.Op = "admin/authentication.GetLogin"

	err := a.getLogin(w, r, nil)
	if err != nil {
		err = errors.E(op, err)
		hlog.FromRequest(r).Error().Ctx(r.Context()).Err(err).Msg("")
		return
	}
}

func (a *authentication) getLogin(w http.ResponseWriter, r *http.Request, input *LoginInput) error {
	const op errors.Op = "website/middleware.authentication.getLogin"

	if input == nil {
		tmp := NewLoginInput(r, "")
		input = &tmp
	}

	err := a.templates.Execute(w, r, input)
	if err != nil {
		return errors.E(op, err)
	}
	return nil
}

func NewLoginInput(r *http.Request, message string) LoginInput {
	return LoginInput{
		Input:          InputFromRequest(r),
		CSRFTokenInput: csrf.TemplateField(r),
		ErrorMessage:   message,
	}
}

type LoginInput struct {
	Input
	CSRFTokenInput template.HTML
	ErrorMessage   string
}

func (LoginInput) TemplateBundle() string {
	return "login"
}

func (a *authentication) PostLogin(w http.ResponseWriter, r *http.Request) {
	const op errors.Op = "website/middleware.authentication.PostLogin"

	err := a.postLogin(r)
	if err != nil {
		err = errors.E(op, err)
		// failed to login, log an error and give the user a generic error
		// message alongside the login page again
		hlog.FromRequest(r).Error().Ctx(r.Context()).Err(err).Msg("")

		input := NewLoginInput(r, "invalid credentials")
		err = a.getLogin(w, r, &input)
		if err != nil {
			err = errors.E(op, err)
			hlog.FromRequest(r).Error().Ctx(r.Context()).Err(err).Msg("failed to send login page")
			return
		}
		return
	}

	// successful login so send them to where they were trying to go
	http.Redirect(w, r, r.URL.String(), http.StatusFound)
}

func (a *authentication) postLogin(r *http.Request) error {
	const op errors.Op = "website/middleware.authentication.postLogin"
	var ctx = r.Context()

	err := r.ParseForm()
	if err != nil {
		return errors.E(op, err, "failed to parse form data")
	}

	username := r.PostFormValue("username")
	if username == "" || len(username) > MAX_USERNAME_LENGTH {
		return errors.E(op, errors.LoginError, "empty or long username")
	}

	password := r.PostFormValue("password")
	if password == "" {
		return errors.E(op, errors.LoginError, "empty password")
	}

	user, err := a.storage.User(ctx).Get(username)
	if err != nil {
		// if it was an unknown username, turn it into a generic invalid argument
		// error instead so the user doesn't get an internal server error
		if errors.Is(errors.UserUnknown, err) {
			return errors.E(op, err, errors.LoginError)
		}
		return errors.E(op, err)
	}

	if !user.UserPermissions.Has(radio.PermActive) {
		// inactive user account, don't allow login attempts
		return errors.E(op, errors.LoginError, "inactive user")
	}

	err = user.ComparePassword(password)
	if err != nil {
		return errors.E(op, err, errors.LoginError, "invalid password")
	}

	// success put their username in the session so we know they're logged in
	a.sessions.Put(ctx, usernameKey, username)
	return nil
}

func (a *authentication) LogoutHandler(w http.ResponseWriter, r *http.Request) {
	const op errors.Op = "admin/authentication.GetLogout"

	err := a.sessions.Destroy(r.Context())
	if err != nil {
		err = errors.E(op, err)
		hlog.FromRequest(r).Error().Ctx(r.Context()).Err(err).Msg("")
		http.Error(w,
			http.StatusText(http.StatusInternalServerError),
			http.StatusInternalServerError)
		return
	}

	// success, redirect to the home page
	http.Redirect(w, r, "/", http.StatusFound)
}

// NewSessionStore returns a new SessionStore that uses the storage provided
func NewSessionStore(ctx context.Context, storage radio.SessionStorageService) SessionStore {
	return SessionStore{ctx, storage}
}

func NewSessionManager(ctx context.Context, storage radio.SessionStorageService, secure bool) *scs.SessionManager {
	m := scs.New()
	m.Store = NewSessionStore(ctx, storage)
	m.Codec = JSONCodec{}
	m.Lifetime = 150 * 24 * time.Hour
	m.Cookie.Name = "admin"
	m.Cookie.Path = "/"
	if secure {
		m.Cookie.Secure = true
		m.Cookie.SameSite = http.SameSiteStrictMode
	}
	return m
}

// SessionStore implements scs.Store by using a radio.SessionStorageService as its
// backing storage.
type SessionStore struct {
	ctx     context.Context
	storage radio.SessionStorageService
}

// Delete implements scs.Store
func (ss SessionStore) Delete(token string) error {
	return ss.storage.Sessions(ss.ctx).Delete(radio.SessionToken(token))
}

// Find implements scs.Store
func (ss SessionStore) Find(token string) ([]byte, bool, error) {
	session, err := ss.storage.Sessions(ss.ctx).Get(radio.SessionToken(token))
	if err != nil && !errors.Is(errors.SessionUnknown, err) {
		return nil, false, err
	}

	if session.Expiry.Before(time.Now()) {
		return nil, false, nil
	}

	return session.Data, true, nil
}

// Commit implements scs.Store
func (ss SessionStore) Commit(token string, b []byte, expiry time.Time) error {
	session := radio.Session{
		Token:  radio.SessionToken(token),
		Expiry: expiry,
		Data:   b,
	}
	return ss.storage.Sessions(ss.ctx).Save(session)
}

// JSONCodec implements scs.Codec
type JSONCodec struct{}

// Encode implements scs.Codec
func (JSONCodec) Encode(deadline time.Time, values map[string]any) ([]byte, error) {
	aux := &struct {
		Deadline time.Time
		Values   map[string]any
	}{
		Deadline: deadline,
		Values:   values,
	}

	b, err := json.Marshal(aux)
	if err != nil {
		return nil, err
	}

	return b, nil
}

// Decode implements scs.Codec
func (JSONCodec) Decode(b []byte) (time.Time, map[string]any, error) {
	aux := &struct {
		Deadline time.Time
		Values   map[string]any
	}{}

	err := json.Unmarshal(b, aux)
	if err != nil {
		return time.Time{}, nil, err
	}

	return aux.Deadline, aux.Values, nil
}

// UserFromContext returns the user stored in the context, a user is available after
// the LoginMiddleware
func UserFromContext(ctx context.Context) *radio.User {
	u, ok := ctx.Value(userContextKey{}).(*radio.User)
	if !ok {
		zerolog.Ctx(ctx).Error().Ctx(ctx).Msg("UserFromContext: called from handler not behind LoginMiddleware")
		return nil
	}
	return u
}

// RequestWithUser adds a user to a requests context and returns the new updated
// request after, user can be retrieved by UserFromContext
func RequestWithUser(r *http.Request, u *radio.User) *http.Request {
	ctx := context.WithValue(r.Context(), userContextKey{}, u)
	return r.WithContext(ctx)
}

// BasicAuth lets users login through the HTTP Basic Authorization header
//
// This should ONLY be used for situations where a human cannot input the
// login info somehow. Namely for icecast source clients.
func BasicAuth(uss radio.UserStorageService) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			username, passwd, ok := r.BasicAuth()
			if !ok || username == "" || passwd == "" {
				hlog.FromRequest(r).Error().Ctx(r.Context()).Msg("basic auth failure")
				BasicAuthFailure(w, r)
				return
			}

			if username == "source" {
				// we support a weird feature where you can submit
				// the username as part of the password by separating
				// them with a '|' so split that
				if strings.Contains(passwd, "|") {
					username, passwd, _ = strings.Cut(passwd, "|")
				}
			}

			// turns out some DJ software doesn't give a rats ass about our response, and they
			// just send us a request over a socket and then close the thing instantly. This means
			// our context is already closed at this point in time, and so our database access gets
			// interrupted due to it.
			ctx := r.Context()
			if err := ctx.Err(); err != nil {
				zerolog.Ctx(ctx).Warn().Ctx(ctx).Err(err).Str("username", username).Msg("context cancellation edgecase")
			}
			// Remove that cancel here for just this one access.
			us := uss.User(context.WithoutCancel(ctx))
			user, err := us.Get(username)
			if err != nil {
				zerolog.Ctx(ctx).Error().Ctx(ctx).Err(err).Str("username", username).Msg("database error")
				BasicAuthFailure(w, r)
				return
			}

			if !user.UserPermissions.Has(radio.PermActive) {
				zerolog.Ctx(ctx).Error().Ctx(ctx).Str("username", username).Msg("inactive user")
				BasicAuthFailure(w, r)
				return
			}

			err = user.ComparePassword(passwd)
			if err != nil {
				zerolog.Ctx(ctx).Error().Ctx(ctx).Str("username", username).Msg("invalid password")
				BasicAuthFailure(w, r)
				return
			}

			// before we pass it back to the handlers we reset the deadlines because the
			// comparison above is long with the race detector enabled
			v := r.Context().Value(http.ServerContextKey)
			if v != nil {
				srv := v.(*http.Server)
				rc := http.NewResponseController(w)
				if srv.WriteTimeout > 0 {
					_ = rc.SetWriteDeadline(time.Now().Add(srv.WriteTimeout))
				}
				if srv.ReadTimeout > 0 {
					_ = rc.SetReadDeadline(time.Now().Add(srv.ReadTimeout))
				}
			}

			next.ServeHTTP(w, RequestWithUser(r, user))
		})
	}
}

// BasicAuthFailure writes a http StatusUnauthorized response with extra text
// that some icecast source clients expect
func BasicAuthFailure(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("WWW-Authenticate", `Basic realm="R/a/dio"`)
	w.WriteHeader(http.StatusUnauthorized)
	_, _ = w.Write([]byte("401 Unauthorized\n"))
}
