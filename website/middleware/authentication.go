package middleware

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	radio "github.com/R-a-dio/valkyrie"
	"github.com/R-a-dio/valkyrie/errors"
	"github.com/R-a-dio/valkyrie/templates"
	"github.com/alexedwards/scs/v2"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/hlog"

	"golang.org/x/crypto/bcrypt"
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
	PostLogin(http.ResponseWriter, *http.Request) error
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

		// no known username yet, so we're not logged in, give them a login box
		if username == "" && r.Method != "POST" {
			a.GetLogin(w, r)
			return
		}

		// we have a username, this suggests we are logged in, try and retrieve
		// the permissions for the user and check if it's an active account
		if username != "" {
			user, err := a.storage.User(ctx).Get(username)
			if err != nil {
				if errors.Is(errors.UserUnknown, err) {
					// unknown user, we force log them out
					a.LogoutHandler(w, r)
					return
				}

				// other unknown error, just internal server and log stuff
				http.Error(w,
					http.StatusText(http.StatusInternalServerError),
					http.StatusInternalServerError)
				hlog.FromRequest(r).Error().Err(err).Msg("")
				return
			}

			if user.UserPermissions.Has(radio.PermActive) {
				// user is indeed active, so forward them to their actual destination
				r = RequestWithUser(r, user)
				next.ServeHTTP(w, r)
				return
			}

			// user account is inactive, force log them out
			a.LogoutHandler(w, r)
			return
		}

		err := a.PostLogin(w, r)
		if err == nil {
			// login successful, send them back to the page they requested at first
			http.Redirect(w, r, r.URL.String(), 302)
			return
		}

		// record that we failed a login, and give a very generic error so that
		// bruteforcing isn't made easier by having us tell them what was wrong
		a.sessions.Put(ctx, failedLoginKey, true)
		var message string
		if errors.Is(errors.InvalidArgument, err) {
			message = "invalid credentials"
		} else {
			message = "internal server error"
		}
		a.sessions.Put(ctx, failedLoginMessageKey, message)
		hlog.FromRequest(r).Error().Err(err).Msg("")
		// either way we're going to send them back to the login page again
		http.Redirect(w, r, r.URL.String(), 302)
	})
}

func (a *authentication) GetLogin(w http.ResponseWriter, r *http.Request) {
	const op errors.Op = "admin/authentication.GetLogin"

	// check if a previous request failed a login
	failed := a.sessions.PopBool(r.Context(), failedLoginKey)
	var failedMessage string
	if failed {
		failedMessage = a.sessions.PopString(r.Context(), failedLoginMessageKey)
	}

	err := a.templates.Execute(w, r, loginInfo{failed, failedMessage})
	if err != nil {
		hlog.FromRequest(r).Error().Err(err).Msg("")
		return
	}
}

type loginInfo struct {
	Failed  bool
	Message string
}

func (loginInfo) TemplateBundle() string {
	return "admin-login"
}

func (loginInfo) TemplateName() string {
	return "full-page"
}

func (a *authentication) PostLogin(w http.ResponseWriter, r *http.Request) error {
	const op errors.Op = "admin/authentication.PostLogin"
	var ctx = r.Context()

	err := r.ParseForm()
	if err != nil {
		return errors.E(op, err, "failed to parse form data")
	}

	username := r.PostFormValue("username")
	if username == "" || len(username) > 50 {
		return errors.E(op, errors.InvalidArgument, "empty or long username")
	}

	password := r.PostFormValue("password")
	if password == "" {
		return errors.E(op, errors.InvalidArgument, "empty password")
	}

	user, err := a.storage.User(ctx).Get(username)
	if err != nil {
		// if it was an unknown username, turn it into a generic invalid argument
		// error instead so the user doesn't get an internal server error
		if errors.Is(errors.UserUnknown, err) {
			return errors.E(op, err, errors.InvalidArgument)
		}
		return errors.E(op, err)
	}

	if !user.UserPermissions.Has(radio.PermActive) {
		// inactive user account, don't allow login attempts
		return errors.E(op, errors.InvalidArgument, "inactive user")
	}

	err = bcrypt.CompareHashAndPassword([]byte(user.Password), []byte(password))
	if err != nil {
		return errors.E(op, err, errors.InvalidArgument, "invalid password")
	}

	a.sessions.Put(ctx, usernameKey, username)
	return nil
}

func (a *authentication) LogoutHandler(w http.ResponseWriter, r *http.Request) {
	const op errors.Op = "admin/authentication.GetLogout"

	err := a.sessions.Destroy(r.Context())
	if err != nil {
		hlog.FromRequest(r).Error().Err(err).Msg("")
		http.Error(w,
			http.StatusText(http.StatusInternalServerError),
			http.StatusInternalServerError)
		return
	}

	// success, redirect to the home page
	http.Redirect(w, r, "/", 302)
	return
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
