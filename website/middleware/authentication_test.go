package middleware

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	radio "github.com/R-a-dio/valkyrie"
	"github.com/R-a-dio/valkyrie/errors"
	"github.com/R-a-dio/valkyrie/mocks"
	"github.com/go-chi/chi/v5"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/crypto/bcrypt"
)

type basicAuthTest struct {
	Name       string
	Code       int
	Username   string
	Password   string
	GetFuncRet *radio.User
	GetFuncErr error
}

func TestBasicAuth(t *testing.T) {
	passwd := "a very important password"
	hash, err := bcrypt.GenerateFromPassword([]byte(passwd), 14)
	require.NoError(t, err)
	require.NotEmpty(t, hash)

	var basicAuthCases = []basicAuthTest{
		{
			Name:     "as source",
			Code:     200,
			Username: "source",
			Password: passwd,
			GetFuncRet: &radio.User{
				Username: "source",
				UserPermissions: radio.UserPermissions{
					radio.PermActive: struct{}{},
				},
				Password: string(hash),
			},
		},
		{
			Name:     "wrong password",
			Code:     401,
			Username: "source",
			Password: "wrong password",
			GetFuncRet: &radio.User{
				Username: "source",
				UserPermissions: radio.UserPermissions{
					radio.PermActive: struct{}{},
				},
				Password: string(hash),
			},
		},
		{
			Name:     "username in password",
			Code:     200,
			Username: "source",
			Password: "test|" + passwd,
			GetFuncRet: &radio.User{
				Username: "test",
				UserPermissions: radio.UserPermissions{
					radio.PermActive: struct{}{},
				},
				Password: string(hash),
			},
		},
		{
			Name:     "username in password but wrong password",
			Code:     401,
			Username: "source",
			Password: "test|wrong password",
			GetFuncRet: &radio.User{
				Username: "test",
				UserPermissions: radio.UserPermissions{
					radio.PermActive: struct{}{},
				},
				Password: string(hash),
			},
		},
		{
			Name:     "username doesn't exist",
			Code:     401,
			Username: "dontexist",
			Password: passwd,
			GetFuncRet: &radio.User{
				Username: "test",
				UserPermissions: radio.UserPermissions{
					radio.PermActive: struct{}{},
				},
				Password: string(hash),
			},
		},
		{
			Name:     "no basic auth supplied",
			Code:     401,
			Username: "",
			Password: "",
		},
		{
			Name:     "username isn't active account",
			Code:     401,
			Username: "source",
			Password: passwd,
			GetFuncRet: &radio.User{
				Username:        "source",
				UserPermissions: radio.UserPermissions{},
				Password:        string(hash),
			},
		},
	}

	for _, test := range basicAuthCases {
		test := test
		t.Run(test.Name, func(t *testing.T) {
			us := &mocks.UserStorageMock{
				GetFunc: func(name string) (*radio.User, error) {
					if test.GetFuncRet == nil {
						return nil, test.GetFuncErr
					}
					if test.GetFuncRet.Username != name {
						return nil, errors.E(errors.UserUnknown)
					}
					return test.GetFuncRet, test.GetFuncErr
				},
			}

			logger := zerolog.New(os.Stdout)
			req := httptest.NewRequest("GET", "/main.mp3", nil)
			if test.Username != "" && test.Password != "" {
				req.SetBasicAuth(test.Username, test.Password)
			}
			req = req.WithContext(logger.WithContext(context.Background()))
			w := httptest.NewRecorder()

			r := chi.NewRouter()
			r.Use(BasicAuth(us))
			r.Get("/*", func(w http.ResponseWriter, r *http.Request) {
				user := UserFromContext(r.Context())
				assert.Equal(t, test.GetFuncRet, user)
			})
			r.ServeHTTP(w, req)

			assert.Equal(t, test.Code, w.Code)
		})
	}
}
