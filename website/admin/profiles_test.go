package admin

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	radio "github.com/R-a-dio/valkyrie"
	"github.com/R-a-dio/valkyrie/config"
	"github.com/R-a-dio/valkyrie/errors"
	"github.com/R-a-dio/valkyrie/mocks"
	"github.com/R-a-dio/valkyrie/website/middleware"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type profileTest struct {
	// Name to use for the test
	Name string
	// URL path + queries, defaults to "/admin/profile"
	Path string
	// User to use for the request
	User radio.User
	// Form to send
	Form ProfileForm
	// Form we expect back
	ExpectedForm *ProfileForm
	// Error we expect back, checked by errors.IsE
	Error error

	TxFunc func(*testing.T) radio.StorageTx

	CreateRet radio.UserID
	CreateErr error

	GetRet radio.User
	GetErr error

	UpdateRet radio.User
	UpdateErr error
}

var adminUser = &radio.User{
	Username: "the admin",
	UserPermissions: radio.UserPermissions{
		radio.PermActive: struct{}{},
		radio.PermAdmin:  struct{}{},
	},
}

var profileTestUser = &radio.User{
	Username: "profile-test",
	UserPermissions: radio.UserPermissions{
		radio.PermActive: struct{}{},
		radio.PermDJ:     struct{}{},
	},
}

var profileTests = []profileTest{
	{
		// new user creation done by an admin, should work
		Name: "NewUserCreation",
		Path: "/admin/profile?new=true",
		User: *adminUser,
		Form: ProfileForm{
			User: radio.User{
				Username: "newuser",
			},
			PasswordChangeForm: ProfilePasswordChangeForm{
				New:      "hackme",
				Repeated: "hackme",
			},
		},
		ExpectedForm: profileSameAsInput,
		TxFunc:       mocks.CommitTx,
		CreateRet:    50,
	},
	{
		// new user creation done by a non-admin, should not be allowed
		Name: "NewUserCreationNotAdmin",
		Path: "/admin/profile?new=true",
		User: *profileTestUser,
		Form: ProfileForm{
			User: radio.User{
				Username: "newuser",
			},
			PasswordChangeForm: ProfilePasswordChangeForm{
				New:      "hackme",
				Repeated: "hackme",
			},
		},
		ExpectedForm: profileSameAsInput,
		Error:        errors.E(errors.AccessDenied),
	},
	{
		// permissions update executed by an admin, should work
		Name: "UpdatePermissionsAsAdmin",
		User: *adminUser,
		Form: ProfileForm{
			User: radio.User{
				Username: profileTestUser.Username,
				UserPermissions: radio.UserPermissions{
					radio.PermActive: struct{}{},
					// remove PermDJ
				},
			},
		},
		ExpectedForm: profileSameAsInput,
		TxFunc:       mocks.CommitTx,
		GetRet:       *profileTestUser,
	},
	{
		// permissions update by the user themselves, should not work,
		// however we don't actually error and instead just silently ignore
		// the permission update.
		Name: "UpdatePermissionsAsUser",
		User: *profileTestUser,
		Form: ProfileForm{
			User: radio.User{
				Username: profileTestUser.Username,
				UserPermissions: radio.UserPermissions{
					radio.PermActive: struct{}{},
					radio.PermDJ:     struct{}{},
					// try add PermDev
					radio.PermDev: struct{}{},
				},
			},
		},
		ExpectedForm: &ProfileForm{
			User: *profileTestUser,
		},
		TxFunc: mocks.CommitTx,
		GetRet: *profileTestUser,
	},
}

// sentinel value to apply the Form field to ExpectedForm field in profileTests
var profileSameAsInput = &ProfileForm{}

func TestPostProfile(t *testing.T) {
	for _, test := range profileTests {
		t.Run(test.Name, func(t *testing.T) {
			// setup test defaults
			if test.User.Username == "" {
				test.User = *genericUser
			}
			if test.Path == "" {
				test.Path = "/admin/profile"
			}
			if test.ExpectedForm == profileSameAsInput {
				test.ExpectedForm = &test.Form
			}

			// setup storage mocks
			storage := &mocks.StorageServiceMock{}
			storage.UserFunc = func(contextMoqParam context.Context) radio.UserStorage {
				return &mocks.UserStorageMock{
					CreateFunc: func(user radio.User) (radio.UserID, error) {
						assert.Equal(t, test.Form.Username, user.Username)
						if test.CreateErr != nil {
							return 0, test.CreateErr
						}
						return test.CreateRet, nil
					},
					GetFunc: func(name string) (*radio.User, error) {
						assert.Equal(t, test.Form.Username, name)
						if test.GetErr != nil {
							return nil, test.GetErr
						}
						return &test.GetRet, nil
					},
					UpdateFunc: func(user radio.User) (radio.User, error) {
						assert.Equal(t, test.ExpectedForm.User, user)
						return test.UpdateRet, test.UpdateErr
					},
				}
			}

			// setup config and state
			cfg, err := config.LoadFile()
			require.NoError(t, err)

			state := State{
				Storage: storage,
				Config:  cfg,
			}

			// setup the form
			formWeSend := test.Form
			body := strings.NewReader(formWeSend.ToValues().Encode())

			// setup the request
			req := httptest.NewRequest(http.MethodPost, test.Path, body)
			req.Header.Add("Content-Type", "application/x-www-form-urlencoded")
			req = middleware.RequestWithUser(req, &test.User)
			w := httptest.NewRecorder()

			// do the request
			form, err := state.postProfile(w, req)

			if test.Error != nil { // test should error
				if assert.Error(t, err, "test should have errored") {
					assert.ErrorIs(t, err, test.Error)
				}
				return
			}

			// test should not error
			if assert.NoError(t, err, "test should not have errored") {
				assert.NotNil(t, form)
				assert.Equal(t, test.ExpectedForm.Username, form.Username)
			}
		})
	}
}
