package admin

import (
	"context"
	"net/http"
	"reflect"
	"time"
	"unsafe"

	radio "github.com/R-a-dio/valkyrie"
	"github.com/R-a-dio/valkyrie/errors"
	"github.com/alexedwards/scs/v2"

	"golang.org/x/crypto/bcrypt"
)

const (
	sessionKey     = "admin-session"
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

func (a authentication) getSession(r *http.Request) *radio.Session {
	return a.sessions.Get(r.Context(), sessionKey).(*radio.Session)
}

func (a authentication) setSession(r *http.Request, session *radio.Session) {
	a.sessions.Put(r.Context(), sessionKey, session)
}

func (a authentication) LoginMiddleware(next http.Handler) http.Handler {
	const op errors.Op = "admin/authentication.LoginMiddleware"

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		session := a.getSession(r)
		if session.UserPermissions.Has(radio.PermActive) {
			next.ServeHTTP(w, r)
			return
		}

		if r.Method != "POST" {
			a.GetLogin(w, r)
			return
		}

		err := a.PostLogin(w, r)
		if err == nil {
			// login successful, send them back to the page they requested at first
			http.Redirect(w, r, r.URL.String(), 302)
			return
		}

		// record that we failed a login, then return back to the login page
		a.sessions.Put(r.Context(), failedLoginKey, true)
		// either way we're going to send them back to the login page again
		http.Redirect(w, r, r.URL.String(), 302)
	})
}

func (a *authentication) GetLogin(w http.ResponseWriter, r *http.Request) {
	const op errors.Op = "admin/authentication.GetLogin"
}

func (a *authentication) PostLogin(w http.ResponseWriter, r *http.Request) error {
	const op errors.Op = "admin/authentication.PostLogin"

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

	user, err := a.storage.User(r.Context()).Get(username)
	if err != nil {
		return errors.E(op, err)
	}

	err = bcrypt.CompareHashAndPassword([]byte(user.Password), []byte(password))
	if err != nil {
		return errors.E(op, err, errors.InvalidArgument)
	}

	session := a.getSession(r)
	session.Username = &user.Username
	session.UserPermissions = user.UserPermissions
	a.setSession(r, session)
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

	// put the new session in the extra map, because it is used by the package to
	// implement the other values
	session.Extra[sessionKey] = &session
	return PtrCodec.encode(&session), true, nil
}

// Commit implements scs.Store
func (ss SessionStore) Commit(token string, b []byte, expiry time.Time) error {
	session := PtrCodec.decode(b)
	// delete the session from the extra values map, otherwise it would be recursive
	delete(session.Extra, sessionKey)
	return ss.storage.Sessions(ss.ctx).Save(*session)
}

type ptrCodec struct{}

// PtrCodec is an instance of ptrCodec, safe to use concurrently
var PtrCodec = ptrCodec{}

// encode puts a *radio.Session into a byte slice by putting the pointer in the
// array ptr field of the slice. The returned []byte is empty and unusable as a
// normal slice. To retrieve the session again call decode on the slice.
func (ptrCodec) encode(s *radio.Session) []byte {
	var b []byte
	tmp := (*reflect.SliceHeader)(unsafe.Pointer(&b))
	tmp.Data = uintptr(unsafe.Pointer(s))
	tmp.Len = 0
	tmp.Cap = 0
	return b
}

func (ptrCodec) Encode(deadline time.Time, values map[string]interface{}) ([]byte, error) {
	s := values[sessionKey].(*radio.Session)
	s.Expiry = deadline
	s.Extra = values

	return PtrCodec.encode(s), nil
}

// decode pulls out a *radio.Session from the []byte given. Where the session
// was previously encoded into it by the encode method
func (ptrCodec) decode(b []byte) *radio.Session {
	tmp := (*reflect.SliceHeader)(unsafe.Pointer(&b))
	return (*radio.Session)(unsafe.Pointer(tmp.Data))
}

func (ptrCodec) Decode(b []byte) (time.Time, map[string]interface{}, error) {
	s := PtrCodec.decode(b)
	return s.Expiry, s.Extra, nil
}
