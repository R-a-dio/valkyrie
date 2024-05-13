package admin

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"maps"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	radio "github.com/R-a-dio/valkyrie"
	"github.com/R-a-dio/valkyrie/config"
	"github.com/R-a-dio/valkyrie/errors"
	"github.com/R-a-dio/valkyrie/mocks"
	"github.com/R-a-dio/valkyrie/website/middleware"
	"github.com/leanovate/gopter"
	"github.com/leanovate/gopter/arbitrary"
	"github.com/leanovate/gopter/gen"
	"github.com/leanovate/gopter/prop"
	"github.com/spf13/afero"
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
	// Password we expect the user to have in the form
	ExpectedPassword string
	// Error we expect back, checked by errors.IsE
	Error error

	TxFunc func(*testing.T) radio.StorageTx

	CreateRet radio.UserID
	CreateErr error

	CreateDJRet radio.DJID
	CreateDJErr error

	GetRet radio.User
	GetErr error

	UpdateRet radio.User
	UpdateErr error

	DJImage *strings.Reader
}

var adminUser = &radio.User{
	Username: "the admin",
	UserPermissions: radio.UserPermissions{
		radio.PermActive: struct{}{},
		radio.PermAdmin:  struct{}{},
	},
}

var profileTestUserRawPassword = "hackme"

var profileTestUser = &radio.User{
	Username: "profile-test",
	UserPermissions: radio.UserPermissions{
		radio.PermActive: struct{}{},
		radio.PermDJ:     struct{}{},
	},
	Password: mustGenerate(profileTestUserRawPassword),
}

var profileTestUserWithDJ = mutateUserPtr(*profileTestUser, func(u radio.User) radio.User {
	u.DJ = radio.DJ{
		ID:   5,
		Name: u.Username,
	}
	return u
})

func mustGenerate(passwd string) string {
	h, err := radio.GenerateHashFromPassword(passwd)
	if err != nil {
		panic("failed password generation in test: " + err.Error())
	}
	return h
}

var profileTests = []profileTest{
	{
		// new user creation done by an admin, should work
		Name: "NewUserCreation",
		Path: "/admin/profile?new=" + profileNewUser,
		User: *adminUser,
		Form: ProfileForm{
			User: radio.User{
				Username: "newuser",
				UserPermissions: radio.UserPermissions{
					radio.PermActive: struct{}{},
				},
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
		Path: "/admin/profile?new=" + profileNewUser,
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
		ExpectedForm: nil,
		Error:        errors.E(errors.AccessDenied),
	},
	{
		Name: "NewDJProfileCreation",
		Path: "/admin/profile?new=" + profileNewDJ,
		User: *adminUser,
		Form: ProfileForm{
			User: *profileTestUser,
		},
		ExpectedForm: &ProfileForm{
			User: mutateUser(*profileTestUser, func(u radio.User) radio.User {
				u.DJ = newProfileDJ(u.Username)
				u.DJ.ID = 70 // should be same as CreateDJRet
				return u
			}),
		},
		TxFunc:      mocks.CommitTx,
		GetRet:      *profileTestUser,
		GetErr:      nil,
		CreateDJRet: 70,
		CreateDJErr: nil,
	},
	{
		Name: "NewDJProfileCreationNotAdmin",
		Path: "/admin/profile?new=" + profileNewDJ,
		User: *profileTestUser,
		Form: ProfileForm{
			User: *profileTestUser,
		},
		ExpectedForm: nil,
		TxFunc:       mocks.CommitTx,
		Error:        errors.E(errors.AccessDenied),
	},
	{
		// permissions update executed by an admin, should work
		Name: "UpdatePermissionsAsAdmin",
		User: *adminUser,
		Form: ProfileForm{
			User: radio.User{
				Username: profileTestUser.Username,
				Password: profileTestUser.Password,
				UserPermissions: radio.UserPermissions{
					radio.PermActive: struct{}{},
					// remove PermDJ
				},
			},
		},
		ExpectedForm: profileSameAsInput,
		TxFunc:       mocks.CommitTx,
		GetRet:       *profileTestUser,
		GetErr:       nil,
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
		GetErr: nil,
	},
	{
		// users should be able to update their own password assuming they know
		// their current password.
		Name: "UpdatePasswordAsUser",
		User: *profileTestUser,
		Form: ProfileForm{
			User: *profileTestUser,
			PasswordChangeForm: ProfilePasswordChangeForm{
				Current:  profileTestUserRawPassword,
				New:      "donthackme",
				Repeated: "donthackme",
			},
		},
		ExpectedForm: &ProfileForm{
			User: *profileTestUser,
		},
		ExpectedPassword: "donthackme",
		TxFunc:           mocks.CommitTx,
		GetRet:           *profileTestUser,
		GetErr:           nil,
	},
	{
		// should only be able to change passwords if New and Repeated match
		Name: "UpdatePasswordAsUserWithWrongRepeated",
		User: *profileTestUser,
		Form: ProfileForm{
			User: *profileTestUser,
			PasswordChangeForm: ProfilePasswordChangeForm{
				Current:  profileTestUserRawPassword,
				New:      "donthackme",
				Repeated: "wrong", // doesn't match New
			},
		},
		ExpectedForm: &ProfileForm{
			User: *profileTestUser,
		},
		ExpectedPassword: "hackme",
		TxFunc:           mocks.CommitTx,
		GetRet:           *profileTestUser,
		GetErr:           nil,
		Error:            errors.E(errors.InvalidForm),
	},
	{
		// should only be able to change passwords if Current is correct
		Name: "UpdatePasswordAsUserWithWrongCurrent",
		User: *profileTestUser,
		Form: ProfileForm{
			User: *profileTestUser,
			PasswordChangeForm: ProfilePasswordChangeForm{
				Current:  "notthepassword",
				New:      "donthackme",
				Repeated: "donthackme",
			},
		},
		ExpectedForm: &ProfileForm{
			User: *profileTestUser,
		},
		ExpectedPassword: "hackme",
		TxFunc:           mocks.CommitTx,
		GetRet:           *profileTestUser,
		GetErr:           nil,
		Error:            errors.E(errors.AccessDenied),
	},
	{
		// should only be able to change passwords if Current is actually given
		Name: "UpdatePasswordAsUserWithNoCurrent",
		User: *profileTestUser,
		Form: ProfileForm{
			User: *profileTestUser,
			PasswordChangeForm: ProfilePasswordChangeForm{
				New:      "donthackme",
				Repeated: "donthackme",
			},
		},
		ExpectedForm: &ProfileForm{
			User: *profileTestUser,
		},
		ExpectedPassword: "hackme",
		TxFunc:           mocks.CommitTx,
		GetRet:           *profileTestUser,
		GetErr:           nil,
		Error:            errors.E(errors.InvalidForm),
	},
	{
		// admins should be able to update passwords for other users
		Name: "UpdatePasswordAsAdmin",
		User: *adminUser,
		Form: ProfileForm{
			User: *profileTestUserWithDJ,
			PasswordChangeForm: ProfilePasswordChangeForm{
				New:      "donthackme",
				Repeated: "donthackme",
			},
		},
		ExpectedForm: &ProfileForm{
			User: *profileTestUserWithDJ,
		},
		ExpectedPassword: "donthackme",
		TxFunc:           mocks.CommitTx,
		GetRet:           *profileTestUserWithDJ,
		GetErr:           nil,
		DJImage:          strings.NewReader("just some data"),
	},
}

func mutateUser(user radio.User, fn func(radio.User) radio.User) radio.User {
	return fn(user)
}

func mutateUserPtr(user radio.User, fn func(radio.User) radio.User) *radio.User {
	user = fn(user)
	return &user
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
			if test.ExpectedPassword == "" {
				test.ExpectedPassword = profileTestUserRawPassword
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
						assert.Equal(t, test.ExpectedForm.Username, user.Username)
						return test.UpdateRet, test.UpdateErr
					},
					CreateDJFunc: func(user radio.User, dJ radio.DJ) (radio.DJID, error) {
						assert.Equal(t, test.ExpectedForm.Username, user.Username)
						return test.CreateDJRet, test.CreateDJErr
					},
				}
			}

			// setup config and state
			cfg := config.TestConfig()

			state := &State{
				Storage: storage,
				Config:  cfg,
				FS:      afero.NewMemMapFs(),
			}

			// setup the form
			var body io.Reader
			formWeSend := test.Form
			body = strings.NewReader(formWeSend.ToValues().Encode())
			ct := "application/x-www-form-urlencoded"
			// if we're testing a multipart upload instead, we have to create it
			// which is somehow a pain in Go
			if test.DJImage != nil {
				buf := new(bytes.Buffer)
				w := multipart.NewWriter(buf)
				// add our normal text fields to it
				for k, v := range formWeSend.ToValues() {
					for _, vv := range v {
						fw, err := w.CreateFormField(k)
						require.NoError(t, err)
						_, err = io.WriteString(fw, vv)
						require.NoError(t, err)
					}
				}
				// add the dj image field
				fw, err := w.CreateFormFile("dj.image", "test.png")
				require.NoError(t, err)
				_, err = io.Copy(fw, test.DJImage)
				require.NoError(t, err)

				// close the multipart writer so it flushes everything and
				// creates the trailing header
				w.Close()

				body, ct = buf, w.FormDataContentType()
			}

			// setup the request
			req := httptest.NewRequest(http.MethodPost, test.Path, body)
			req.Header.Add("Content-Type", ct)
			req = middleware.RequestWithUser(req, &test.User)
			w := httptest.NewRecorder()

			// do the request
			form, err := state.postProfile(w, req)

			if test.Error != nil { // test should error
				if assert.Error(t, err, "test should have errored") {
					assert.ErrorIs(t, err, test.Error)
				}
				if test.ExpectedForm != nil {
					assert.NotNil(t, form)
					checkForm(t, test, state, form)
				} else {
					assert.Nil(t, form)
				}
				return
			}

			// test should not error
			if assert.NoError(t, err, "test should not have errored") {
				if test.ExpectedForm != nil {
					assert.NotNil(t, form)
					checkForm(t, test, state, form)
				} else {
					assert.Nil(t, form)
				}
				return
			}
		})
	}
}

func checkForm(t *testing.T, test profileTest, state *State, got *ProfileForm) {
	expected := test.ExpectedForm
	fs := afero.NewBasePathFs(state.FS, state.Conf().Website.DJImagePath)

	// password should match
	assert.NoError(t, got.User.ComparePassword(test.ExpectedPassword))
	// username should match
	assert.Equal(t, expected.Username, got.Username)
	// and permissions
	assert.Equal(t, expected.UserPermissions, got.UserPermissions)
	// for DJ comparison we first need to know if we're testing with a
	// DJ image upload or not
	if test.DJImage != nil {
		// if we did test with an image upload, then we need to check the DJ
		// image field
		assert.NotZero(t, got.DJ.Image)
		// filename on disk is just the ID
		imageName := fmt.Sprintf("%d", got.DJ.ID)
		// see that it exists
		ok, err := afero.Exists(fs, imageName)
		assert.NoError(t, err, "should not error")
		assert.True(t, ok, "file should exist: %s %s", imageName, got.DJ.Image)
		// then make sure the image filename we store has the DJID in it
		assert.True(t, strings.HasPrefix(got.DJ.Image, imageName), "%s does not contain %s", got.DJ.Image, imageName)
		// then make sure the contents we uploaded are in the file
		contents, err := afero.ReadFile(fs, imageName)
		if assert.NoError(t, err) {
			_, err = test.DJImage.Seek(0, io.SeekStart)
			require.NoError(t, err)
			expected, err := io.ReadAll(test.DJImage)
			if assert.NoError(t, err) {
				assert.Equal(t, expected, contents)
			}
		}
		got.DJ.Image = "" // then zero it so comparison succeeds below
	}
	assert.Equal(t, expected.DJ, got.DJ)
}

func TestProfileFormRoundTrip(t *testing.T) {
	a := arbitrary.DefaultArbitraries()
	p := gopter.NewProperties(nil)

	profileUserGen := gen.Struct(reflect.TypeOf(radio.User{}), map[string]gopter.Gen{
		"Username":        gen.AnyString(),
		"Email":           gen.AnyString(),
		"IP":              gen.AnyString(),
		"UserPermissions": genForType[radio.UserPermissions](a),
		"DJ": gen.Struct(reflect.TypeOf(radio.DJ{}), map[string]gopter.Gen{
			"ID": genForType[radio.DJID](a).SuchThat(func(v radio.DJID) bool {
				return v != 0
			}),
			"Visible":  gen.Bool(),
			"Name":     gen.AnyString(),
			"Priority": gen.Int(),
			"Regex":    gen.AnyString(),
			"Text":     gen.AnyString(),
		}),
	})

	p.Property("profile form should roundtrip", prop.ForAll(
		func(u radio.User) bool {
			in := ProfileForm{
				User: u,
			}
			var out ProfileForm

			out.Update(in.ToValues())
			// null the djid, since we don't actually roundtrip it but it is required
			// by the ToValues to be set
			in.DJ.ID = 0

			return in.Username == out.Username &&
				in.Email == out.Email &&
				in.IP == out.IP &&
				in.DJ == out.DJ &&
				in.PasswordChangeForm.Current == out.PasswordChangeForm.Current &&
				in.PasswordChangeForm.New == out.PasswordChangeForm.New &&
				in.PasswordChangeForm.Repeated == out.PasswordChangeForm.Repeated &&
				maps.Equal(in.UserPermissions, out.newPermissions)
		}, profileUserGen,
	))
	p.TestingRun(t, gopter.ConsoleReporter(false))
}

func genForType[T any](a *arbitrary.Arbitraries) gopter.Gen {
	return a.GenForType(reflect.TypeFor[T]())
}
