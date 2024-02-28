package middleware

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"time"

	radio "github.com/R-a-dio/valkyrie"
	"github.com/R-a-dio/valkyrie/errors"
	"github.com/R-a-dio/valkyrie/templates"
	"github.com/alexedwards/scs/v2"
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

type userContextKey struct{}

func NewAuthentication(storage radio.StorageService, tmpl templates.Executor, sessions *scs.SessionManager) Authentication {
	return &authentication{
		storage:   storage,
		sessions:  sessions,
		templates: tmpl,
	}
}

type Authentication interface {
	UserMiddleware(http.Handler) http.Handler
	LoginMiddleware(http.Handler) http.Handler
	GetLogin(http.ResponseWriter, *http.Request)
	PostLogin(http.ResponseWriter, *http.Request)
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
		if user == nil {
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
			err = errors.E(op, err)
			http.Error(w,
				http.StatusText(http.StatusInternalServerError),
				http.StatusInternalServerError)
			hlog.FromRequest(r).Error().Err(err).Msg("")
			return
		}

		next.ServeHTTP(w, RequestWithUser(r, user))
	})
}

// LoginMiddleware makes all routes require requests to be from logged in users
// with permissions Active (active user account). Otherwise they get redirected
// to a login page
func (a authentication) LoginMiddleware(next http.Handler) http.Handler {
	const op errors.Op = "admin/authentication.LoginMiddleware"

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		username := a.sessions.GetString(ctx, usernameKey)

		// no known username yet, so we're not logged in
		if username == "" {
			if r.Method == http.MethodPost {
				// if it's a POST try and see if they're trying to login
				a.PostLogin(w, r)
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
			hlog.FromRequest(r).Error().Err(err).Msg("")
			return
		}

		if !user.UserPermissions.Has(radio.PermActive) {
			// user isn't active, so log them out
			a.LogoutHandler(w, r)
			return
		}

		// otherwise, use is active so forward them to their destination
		r = RequestWithUser(r, user)
		next.ServeHTTP(w, r)
	})
}

func (a *authentication) GetLogin(w http.ResponseWriter, r *http.Request) {
	const op errors.Op = "admin/authentication.GetLogin"

	err := a.getLogin(w, r, nil)
	if err != nil {
		err = errors.E(op, err)
		hlog.FromRequest(r).Error().Err(err).Msg("")
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
		Input:        InputFromRequest(r),
		ErrorMessage: message,
	}
}

type LoginInput struct {
	Input

	ErrorMessage string
}

func (LoginInput) TemplateBundle() string {
	return "login"
}

func (a *authentication) PostLogin(w http.ResponseWriter, r *http.Request) {
	const op errors.Op = "website/middleware.authentication.PostLogin"

	err := a.postLogin(w, r)
	if err != nil {
		err = errors.E(op, err)
		// failed to login, log an error and give the user a generic error
		// message alongside the login page again
		hlog.FromRequest(r).Error().Err(err).Msg("")

		input := NewLoginInput(r, "invalid credentials")
		err = a.getLogin(w, r, &input)
		if err != nil {
			err = errors.E(op, err)
			hlog.FromRequest(r).Error().Err(err).Msg("failed to send login page")
			return
		}
		return
	}

	// successful login so send them to where they were trying to go
	http.Redirect(w, r, r.URL.String(), http.StatusFound)
}

func (a *authentication) postLogin(w http.ResponseWriter, r *http.Request) error {
	const op errors.Op = "website/middleware.authentication.postLogin"
	var ctx = r.Context()

	err := r.ParseForm()
	if err != nil {
		return errors.E(op, err, "failed to parse form data")
	}

	username := r.PostFormValue("username")
	if username == "" || len(username) > 50 {
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
		hlog.FromRequest(r).Error().Err(err).Msg("")
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

func NewSessionManager(ctx context.Context, storage radio.SessionStorageService) *scs.SessionManager {
	m := scs.New()
	m.Store = NewSessionStore(ctx, storage)
	m.Codec = JSONCodec{}
	m.Lifetime = 150 * 24 * time.Hour
	m.Cookie = scs.SessionCookie{
		Name: "admin",
		Path: "/",
		//SameSite: http.SameSiteStrictMode,
		//Secure: true,
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
func (JSONCodec) Encode(deadline time.Time, values map[string]interface{}) ([]byte, error) {
	aux := &struct {
		Deadline time.Time
		Values   map[string]interface{}
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
func (JSONCodec) Decode(b []byte) (time.Time, map[string]interface{}, error) {
	aux := &struct {
		Deadline time.Time
		Values   map[string]interface{}
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
		zerolog.Ctx(ctx).Error().Msg("UserFromContext: called from handler not behind LoginMiddleware")
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
				hlog.FromRequest(r).Error().Msg("basic auth failure")
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

			us := uss.User(r.Context())
			user, err := us.Get(username)
			if err != nil {
				hlog.FromRequest(r).Error().Err(err).Str("username", username).Msg("database error")
				BasicAuthFailure(w, r)
				return
			}

			if !user.UserPermissions.Has(radio.PermActive) {
				hlog.FromRequest(r).Error().Str("username", username).Msg("inactive user")
				BasicAuthFailure(w, r)
				return
			}

			err = user.ComparePassword(passwd)
			if err != nil {
				hlog.FromRequest(r).Error().Str("username", username).Msg("invalid password")
				BasicAuthFailure(w, r)
				return
			}

			// before we pass it back to the handlers we reset the deadlines because the
			// comparison above is long under some conditions
			v := r.Context().Value(http.ServerContextKey)
			if v != nil {
				srv := v.(*http.Server)
				rc := http.NewResponseController(w)
				_ = rc.SetWriteDeadline(time.Now().Add(srv.WriteTimeout))
				_ = rc.SetReadDeadline(time.Now().Add(srv.ReadTimeout))
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
