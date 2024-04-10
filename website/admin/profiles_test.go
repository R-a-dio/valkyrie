package admin

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	radio "github.com/R-a-dio/valkyrie"
	"github.com/R-a-dio/valkyrie/config"
	"github.com/R-a-dio/valkyrie/mocks"
	"github.com/R-a-dio/valkyrie/website/middleware"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPostNewProfile(t *testing.T) {
	storage := &mocks.StorageServiceMock{}
	storage.UserFunc = func(contextMoqParam context.Context) radio.UserStorage {
		return &mocks.UserStorageMock{
			CreateFunc: func(user radio.User) (radio.UserID, error) {
				assert.Equal(t, "test", user.Username)
				assert.NotZero(t, user.Password, "user should have a password")
				assert.NotEqual(t, "yes", user.Password, "shouldn't store password in plaintext")
				return 5, nil
			},
		}
	}

	user := radio.User{
		Username: "Wessie",
		UserPermissions: radio.UserPermissions{
			radio.PermActive: struct{}{},
			radio.PermAdmin:  struct{}{},
		},
	}

	cfg, err := config.LoadFile()
	require.NoError(t, err)

	state := State{
		Storage: storage,
		Config:  cfg,
	}

	formWeSend := ProfileForm{
		User: radio.User{
			Username: "test",
		},
		PasswordChangeForm: ProfilePasswordChangeForm{
			New:      "yes",
			Repeated: "yes",
		},
	}

	body := strings.NewReader(formWeSend.ToValues().Encode())
	req := httptest.NewRequest(http.MethodPost, "/admin/profile?new=true", body)
	req.Header.Add("Content-Type", "application/x-www-form-urlencoded")
	req = middleware.RequestWithUser(req, &user)
	w := httptest.NewRecorder()

	form, err := state.postProfile(w, req)
	if assert.NoError(t, err) {
		assert.NotNil(t, form)
		assert.Equal(t, form.Username, formWeSend.Username)
	}
}
