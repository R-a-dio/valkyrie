package admin

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"text/template"
	"time"

	radio "github.com/R-a-dio/valkyrie"
	"github.com/R-a-dio/valkyrie/errors"
	"github.com/alexedwards/scs/v2"

	"golang.org/x/crypto/bcrypt"
)

const (
	usernameKey    = "admin-username"
	failedLoginKey = "admin-failed-login"
)

func NewAuthentication(storage radio.StorageService, sessions *scs.SessionManager) authentication {
	return authentication{
		storage:  storage,
		sessions: sessions,
	}
}

type authentication struct {
	storage  radio.StorageService
	sessions *scs.SessionManager
}

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
				log.Println(err)
				return
			}

			if user.UserPermissions.Has(radio.PermActive) {
				// user is indeed active, so forward them to their actual destination
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

		log.Println(err)
		// record that we failed a login, then return back to the login page
		a.sessions.Put(ctx, failedLoginKey, true)
		// either way we're going to send them back to the login page again
		http.Redirect(w, r, r.URL.String(), 302)
	})
}

func (a *authentication) GetLogin(w http.ResponseWriter, r *http.Request) {
	const op errors.Op = "admin/authentication.GetLogin"

	isFailed := a.sessions.PopBool(r.Context(), failedLoginKey)

	log.Println("parsing login template")
	tmpl, err := template.ParseFiles(`Z:\git\valkyrie\templates\admin\login.tmpl`)
	if err != nil {
		log.Println(err)
		return
	}

	log.Println("outputting login template")
	err = tmpl.ExecuteTemplate(w, "login", isFailed)
	if err != nil {
		log.Println(err)
	}

	_ = isFailed
}

func (a *authentication) PostLogin(w http.ResponseWriter, r *http.Request) error {
	const op errors.Op = "admin/authentication.PostLogin"
	var ctx = r.Context()

	err := r.ParseForm()
	if err != nil {
		return err
	}

	username := r.PostFormValue("username")
	if username == "" || len(username) > 50 {
		return errors.E(op, errors.InvalidArgument)
	}

	password := r.PostFormValue("password")
	if password == "" {
		return errors.E(op, errors.InvalidArgument)
	}

	user, err := a.storage.User(ctx).Get(username)
	if err != nil {
		return errors.E(op, err)
	}

	if !user.UserPermissions.Has(radio.PermActive) {
		// inactive user account, don't allow login attempts
		return errors.E(op, errors.InvalidArgument)
	}

	err = bcrypt.CompareHashAndPassword([]byte(user.Password), []byte(password))
	if err != nil {
		return errors.E(op, err, errors.InvalidArgument)
	}

	a.sessions.Put(ctx, usernameKey, username)
	return nil
}

func (a *authentication) LogoutHandler(w http.ResponseWriter, r *http.Request) {
	const op errors.Op = "admin/authentication.GetLogout"

	err := a.sessions.Destroy(r.Context())
	if err != nil {
		// failed to logout
		// TODO: handle errors
		return
	}

	// success, redirect to the home page (or try to)
	home := *r.URL
	home.Path = ""
	http.Redirect(w, r, home.String(), 302)
	return
}

// NewSessionStore returns a new SessionStore that uses the storage provided
func NewSessionStore(ctx context.Context, storage radio.SessionStorageService) SessionStore {
	return SessionStore{ctx, storage}
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
	if err != nil {
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
