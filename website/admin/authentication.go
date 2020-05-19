package admin

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	radio "github.com/R-a-dio/valkyrie"
	"github.com/R-a-dio/valkyrie/errors"
	"github.com/alexedwards/scs/v2"

	"golang.org/x/crypto/bcrypt"
)

const permissionsKey = "permissions"

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
		perms := a.sessions.Get(r.Context(), permissionsKey).(radio.UserPermissions)
		if !perms.Has(radio.PermActive) {
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
