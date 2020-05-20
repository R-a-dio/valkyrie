package admin

import (
	"context"
	"encoding/json"
	"net/http"
	"reflect"
	"time"
	"unsafe"

	radio "github.com/R-a-dio/valkyrie"
	"github.com/R-a-dio/valkyrie/errors"
	"github.com/alexedwards/scs/v2"

	"golang.org/x/crypto/bcrypt"
)

const sessionKey = "admin-session"

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
		session := a.sessions.Get(r.Context(), sessionKey).(*radio.Session)
		if !session.UserPermissions.Has(radio.PermActive) {
			a.GetLogin(w, r)
		} else {
			next.ServeHTTP(w, r)
		}
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
		return errors.E(op, err)
	}

	return nil
}

func (a *authentication) LogoutHandler(w http.ResponseWriter, r *http.Request) {
	const op errors.Op = "admin/authentication.GetLogout"

	err := a.sessions.Destroy(r.Context())
	if err != nil {
		return
	}

	return
}

// NewSessionStore returns a new SessionStore that uses the storage provided
func NewSessionStore(ctx context.Context, storage radio.StorageService) SessionStore {
	return SessionStore{ctx, storage}
}

// SessionStore implements scs.Store
type SessionStore struct {
	ctx     context.Context
	storage radio.StorageService
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

	return PtrCodec.encode(&session), true, nil
}

// Commit implements scs.Store
func (ss SessionStore) Commit(token string, b []byte, expiry time.Time) error {
	session := PtrCodec.decode(b)
	return ss.storage.Sessions(ss.ctx).Save(*session)
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
	return s.Expiry, map[string]interface{}{
		sessionKey: s,
	}, nil
}
