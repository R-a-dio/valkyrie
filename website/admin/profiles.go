package admin

import (
	"fmt"
	"html/template"
	"log"
	"mime/multipart"
	"net/http"
	"net/url"
	"strconv"

	radio "github.com/R-a-dio/valkyrie"
	"github.com/R-a-dio/valkyrie/errors"
	"golang.org/x/crypto/bcrypt"

	"github.com/gorilla/schema"
)

var profileDecoder = schema.NewDecoder()

// ProfileForm defines the form we use for the profile page
type ProfileForm struct {
	radio.User
	// separate struct for password change handling
	Change ProfileFormChange
}

type ProfileFormChange struct {
	Password         string
	NewPassword      string
	RepeatedPassword string
}

// GetProfile is mounted under /admin/profile and shows the currently logged
// in users profile
func (a admin) GetProfile(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	// current user of this session
	user := UserFromContext(ctx)

	isAdmin := user.UserPermissions.Has(radio.PermAdmin)
	var availablePermissions []string

	// output is the user we want to show the profile page of, in non-admin
	// cases this will always be the current user
	output := user
	if isAdmin {
		// admin can change permissions, so load all available ones
		availablePermissions = []string{"test"}
	}

	// if it's an admin user they can look at all user accounts, so we
	// take an ?username=<username> query argument
	username := r.FormValue("username")
	if username != "" && isAdmin {
		other, err := a.storage.User(ctx).Get(username)
		if err != nil {
			log.Println(err)
			return
		}

		output = other
	}

	isNew := r.Form["new"] != nil && isAdmin && username == ""
	if isNew {
		output = &radio.User{}
	}

	isNewProfile := r.Form["newprofile"] != nil && isAdmin && !isNew

	profileInput := struct {
		IsAdmin              bool
		IsNew                bool
		IsNewProfile         bool
		CurrentIP            template.JS
		AvailablePermissions []string
		User                 *radio.User
		DJ                   radio.DJ
		Theme                radio.Theme
		AvailableThemes      []string
	}{
		IsAdmin:              isAdmin,
		IsNew:                isNew,
		IsNewProfile:         isNewProfile,
		CurrentIP:            template.JS(r.RemoteAddr),
		AvailablePermissions: availablePermissions,
		User:                 output,
		DJ:                   output.DJ,
		Theme:                output.DJ.Theme,
		AvailableThemes:      []string{},
	}

	err := a.templates["admin"]["profile.tmpl"].ExecuteDev(w, profileInput)
	if err != nil {
		log.Println(err)
		return
	}

	return
}

func (a admin) PostProfile(w http.ResponseWriter, r *http.Request) {
	err := a.postProfile(w, r)
	if err == nil {
		http.Redirect(w, r, r.URL.String(), 303)
		return
	}

	log.Println(err)
	// we expect 3 types of errors
	switch {
	case errors.Is(errors.InvalidForm, err):
		// form input was invalid, slap them back to rendering the form
		// with an error to indicate what was wrong
	case errors.Is(errors.InternalServer, err):
		// something broke internally, we don't know what, just tell them
		// when we try to recover to a profile page
	case errors.Is(errors.AccessDenied, err):
		// user wasn't allowed to do the thing they tried to do
	default:
		// unknown error?
	}
	return

}

func (a admin) postProfile(w http.ResponseWriter, r *http.Request) error {
	// TODO: handle new accounts
	const op errors.Op = "website/admin.postProfile"

	ctx := r.Context()
	// current user of this session
	user := UserFromContext(ctx)
	// is the session user an admin, in which case (s)he can edit other users
	isAdmin := user.UserPermissions.Has(radio.PermAdmin)
	userStorage := a.storage.User(ctx)

	err := r.ParseMultipartForm(16 * 1024)
	if err != nil {
		return errors.E(op, errors.InternalServer, err)
	}

	var form ProfileForm

	// parse the form, all the data in here is untrusted so we need to check it after
	err = profileDecoder.Decode(&form, r.MultipartForm.Value)
	if err != nil {
		return errors.E(op, errors.InternalServer, err)
	}

	// first thing to check is to see if we're working with the session user,
	// if we're not, the session user needs to be admin to touch other users
	if form.Username != user.Username && !isAdmin {
		return errors.E(op, errors.AccessDenied)
	}

	var isNewAccount = false
	// most of the fields can be updated as-is after user input, but we copy stuff
	// into a "legit" copy from the database first
	//
	// in the non-admin case we can use the session user, and if an admin is
	// involved and isn't editing their own user we need a database copy of the
	// other user
	var new radio.User = *user
	if isAdmin && form.Username != user.Username {
		fresh, err := userStorage.Get(form.Username)
		if err != nil && errors.Is(errors.UserUnknown, err) {
			// unknown user, and we're admin, so a new account creation
			isNewAccount = true
			// fill in the username for the user
			fresh = &radio.User{
				Username: form.Username,
			}
			// we mock the change struct for new accounts
			form.Change = ProfileFormChange{
				NewPassword:      form.Change.Password,
				RepeatedPassword: form.Change.Password,
			}
		} else if err != nil {
			// some kind of database error
			return errors.E(op, errors.InternalServer, err)
		}
		new = *fresh
	}

	// handle password change, any user can do this to themselves, admins can do
	// it to anyone
	if c := form.Change; c.NewPassword != "" {
		err := postProfilePassword(&new, isAdmin, c)
		if err != nil {
			// error, something failed in password change handling
			return errors.E(op, err)
		}
	} else if isNewAccount {
		// new account, but no password supplied.
		return errors.E(op, errors.InvalidForm,
			errors.Info("Change.Password"),
			"required field",
		)
	}

	isNewProfile := new.DJ.ID == 0
	// if the user has no DJ profile we're basically done here, unless
	// an admin is trying to create a profile.
	if isNewProfile && !isAdmin {
		_, err = userStorage.UpdateUser(new)
		if err != nil {
			return errors.E(op, errors.InternalServer, err)
		}
		return nil
	}

	// copy over fields, and supply defaults if it's a new profile with no input
	new.IP = form.IP
	new.DJ.Visible = form.DJ.Visible
	new.DJ.Regex = form.DJ.Regex

	// only set a default priority if it's a new profile, otherwise 0 is a valid
	// priority
	if isNewProfile && form.DJ.Priority == 0 {
		new.DJ.Priority = 200
	} else {
		new.DJ.Priority = form.DJ.Priority
	}

	if isNewProfile && form.DJ.Name == "" {
		// required field
		return errors.E(op, errors.InvalidForm,
			errors.Info("DJ.Name"),
			"required field",
		)
	} else {
		new.DJ.Name = form.DJ.Name
	}

	if isNewProfile && form.DJ.Theme.Name == "" {
		new.DJ.Theme.Name = "default"
	} else {
		new.DJ.Theme.Name = form.DJ.Theme.Name
	}

	// now handle dj image changes
	if f := r.MultipartForm.File["DJ.Image"]; len(f) > 0 {
		err := postProfileImage(&new, f[0])
		if err != nil {
			// error, something failed in image handling
			return errors.E(op, err)
		}
	}

	// TODO: handle permissions

	res, err := userStorage.UpdateUser(new)
	if err != nil {
		return errors.E(op, errors.InternalServer, err)
	}
	fmt.Printf("result: %#v\ninput: %#v\nform: %#v\n", res, new, form)
	return nil
}

func postProfilePassword(new *radio.User, isAdmin bool, form ProfileFormChange) error {
	const op errors.Op = "website/admin.postProfilePassword"

	if form.NewPassword != form.RepeatedPassword {
		// error, because new and repeat don't match
		return errors.E(op, errors.InvalidForm,
			errors.Info("Change.NewPassword"),
			"repeated password did not match new password",
		)
	}

	if form.Password == "" && !isAdmin {
		// error, because no password given
		return errors.E(op, errors.InvalidForm,
			errors.Info("Change.Password"),
			"empty password",
		)
	}

	if !isAdmin { // only need to check it if we're not admin
		err := bcrypt.CompareHashAndPassword(
			[]byte(new.Password),
			[]byte(form.Password),
		)
		if err != nil {
			// error, current password doesn't match actual password
			return errors.E(op, errors.InvalidForm,
				errors.Info("Change.Password"),
				"invalid password",
			)
		}
	}

	hashed, err := bcrypt.GenerateFromPassword([]byte(form.NewPassword), bcryptCost)
	if err != nil {
		// error, failed to generate bcrypt from password
		// no idea what could cause this to error, so we just throw up
		return errors.E(op, errors.InternalServer, err)
	}

	new.Password = string(hashed)
	return nil
}

func postProfileImage(new *radio.User, header *multipart.FileHeader) error {

	return nil
}

type extraProfileInfo struct {
	// password change fields
	current  string
	new      string
	repeated string

	// image change fields
	image *multipart.FileHeader
}

func userFromPostForm(form *multipart.Form) (radio.User, extraProfileInfo) {
	v, files := url.Values(form.Value), form.File
	var user radio.User
	var info extraProfileInfo

	// username, should be immutable but we read it anyway
	user.Username = v.Get("username")
	// password change info, should generally be empty
	info.current = v.Get("password")
	info.new = v.Get("new-password")
	info.repeated = v.Get("repeated-password")
	// ip address for stream access
	user.IP = v.Get("ip-address")

	user.DJ.Name = v.Get("dj-name")
	if f := files["dj-image"]; len(f) > 0 {
		info.image = f[0]
	}

	// visibilty on staff page, this is only present if checked
	user.DJ.Visible = v["visible"] != nil
	{
		// ordering on the staff page, ignore the value if not a number
		p := v.Get("priority")
		priority, err := strconv.Atoi(p)
		if err == nil {
			user.DJ.Priority = priority
		}
	}
	user.DJ.Regex = v.Get("regex")
	user.DJ.Theme.Name = v.Get("theme")
	return user, info
}
