package admin

import (
	"crypto/sha256"
	"fmt"
	"html/template"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"slices"

	radio "github.com/R-a-dio/valkyrie"
	"github.com/R-a-dio/valkyrie/config"
	"github.com/R-a-dio/valkyrie/errors"
	"github.com/R-a-dio/valkyrie/website/middleware"
	"github.com/rs/zerolog/hlog"

	"github.com/gorilla/schema"
)

var profileDecoder = schema.NewDecoder()

// ProfileForm defines the form we use for the profile page
type ProfileForm struct {
	radio.User
	// separate struct for password change handling
	Change ProfileFormChange
	// permissions are separate too because gorilla does not like maps
	Permissions []radio.UserPermission
	// errors that occured while parsing the form
	Error error
}

func toUserPermissionSlice(u radio.UserPermissions) []radio.UserPermission {
	var res []radio.UserPermission
	for perm := range u {
		res = append(res, perm)
	}
	return res
}

func fromUserPermissionSlice(u []radio.UserPermission) radio.UserPermissions {
	var res = radio.UserPermissions{}
	for _, v := range u {
		res[v] = struct{}{}
	}
	return res
}

type ProfileFormChange struct {
	Password         string
	NewPassword      string
	RepeatedPassword string
}

// GetProfile is mounted under /admin/profile and shows the currently logged
// in users profile
func (s *State) GetProfile(w http.ResponseWriter, r *http.Request) {
	err := s.getProfile(w, r, nil)
	if err != nil {
		hlog.FromRequest(r).Error().Err(err).Msg("")
		return
	}
}

type profileInput struct {
	middleware.Input

	IsAdmin              bool
	IsNew                bool
	IsNewProfile         bool
	CurrentIP            template.JS
	AvailablePermissions []radio.UserPermission
	Form                 *ProfileForm
	AvailableThemes      []string
}

func (profileInput) TemplateBundle() string {
	return "profile"
}

func (s *State) getProfile(w http.ResponseWriter, r *http.Request, form *ProfileForm) error {
	ctx := r.Context()
	// current user of this session
	user := middleware.UserFromContext(ctx)

	isAdmin := user.UserPermissions.Has(radio.PermAdmin)
	var availablePermissions []radio.UserPermission
	var err error

	// output is the user we want to show the profile page of, in non-admin
	// cases this will always be the current user
	userToEdit := user
	if isAdmin {
		// admin can change permissions, so load all available ones
		availablePermissions, err = s.Storage.User(ctx).Permissions()
		if err != nil {
			return err
		}
	}

	// if it's an admin user they can look at all user accounts, so we
	// take an ?username=<username> query argument
	username := r.FormValue("username")
	if username != "" && isAdmin {
		other, err := s.Storage.User(ctx).Get(username)
		if err != nil {
			return err
		}

		userToEdit = other
	}

	isNew := r.Form["new"] != nil && isAdmin && username == ""
	if isNew {
		userToEdit = &radio.User{}
	}

	isNewProfile := r.Form["newprofile"] != nil && isAdmin && !isNew

	if form == nil {
		// didn't get passed an existing form, so construct our own
		form = &ProfileForm{
			User:        *userToEdit,
			Permissions: toUserPermissionSlice(userToEdit.UserPermissions),
		}
	}

	// fill in all available permissions for the user if an admin is looking at them
	if isAdmin {
		if form.UserPermissions == nil {
			form.UserPermissions = make(radio.UserPermissions)
		}
		for _, perm := range availablePermissions {
			if slices.Contains(form.Permissions, perm) {
				form.UserPermissions[perm] = struct{}{}
			}
		}
	}

	tmplInput := profileInput{
		Input:                middleware.InputFromRequest(r),
		IsAdmin:              isAdmin,
		IsNew:                isNew,
		IsNewProfile:         isNewProfile,
		CurrentIP:            template.JS(r.RemoteAddr),
		AvailablePermissions: availablePermissions,
		Form:                 form,
		AvailableThemes:      []string{},
	}

	err = s.TemplateExecutor.Execute(w, r, tmplInput)
	if err != nil {
		return err
	}

	return nil
}

// PostProfile implements the profile form POST parsing and handling
//
// The expected form as input is defined in templates/partials/form_admin_profile
func (s *State) PostProfile(w http.ResponseWriter, r *http.Request) {
	form, err := s.postProfile(w, r)
	if err != nil {
		if form != nil {
			form.Error = err
		}

		hlog.FromRequest(r).Error().Err(err).Msg("")
		// we expect 3 types of errors
		switch {
		case errors.Is(errors.InvalidForm, err):
			// form input was invalid, slap them back to rendering the form
			// with an error to indicate what was wrong
			fallthrough
		case errors.Is(errors.InternalServer, err):
			// something broke internally, we don't know what, just tell them
			// when we try to recover to a profile page
			fallthrough
		case errors.Is(errors.AccessDenied, err):
			// user wasn't allowed to do the thing they tried to do
			err := s.getProfile(w, r, form)
			if err != nil {
				// nested errors, probably something broken so just return a 501
				http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
				return
			}
		default:
			// unknown error?
		}
	}

	http.Redirect(w, r, r.URL.String(), http.StatusSeeOther)
}

func (s *State) postProfile(w http.ResponseWriter, r *http.Request) (*ProfileForm, error) {
	const op errors.Op = "website/admin.postProfile"

	ctx := r.Context()
	// current user of this session
	user := middleware.UserFromContext(ctx)
	// is the session user an admin, in which case (s)he can edit other users
	isAdmin := user.UserPermissions.Has(radio.PermAdmin)
	userStorage := s.Storage.User(ctx)

	err := r.ParseMultipartForm(16 * 1024)
	if err != nil {
		return nil, errors.E(op, errors.InternalServer, err)
	}

	var form ProfileForm

	// parse the form, all the data in here is untrusted so we need to check it after
	err = profileDecoder.Decode(&form, r.MultipartForm.Value)
	if err != nil {
		return nil, errors.E(op, errors.InternalServer, err)
	}

	// first thing to check is to see if we're working with the session user,
	// if we're not, the session user needs to be admin to touch other users
	if form.Username != user.Username && !isAdmin {
		return &form, errors.E(op, errors.AccessDenied)
	}

	// change the url to point to the username we're editing
	if form.Username != user.Username {
		q := r.URL.Query()
		q.Del("username")
		q.Add("username", form.Username)
		r.URL.RawQuery = q.Encode()
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
			return &form, errors.E(op, errors.InternalServer, err)
		}
		new = *fresh
	}

	// handle password change, any user can do this to themselves, admins can do
	// it to anyone
	if c := form.Change; c.NewPassword != "" {
		err := postProfilePassword(&new, isAdmin, c)
		if err != nil {
			// error, something failed in password change handling
			return &form, errors.E(op, err)
		}
	} else if isNewAccount {
		// new account, but no password supplied.
		return &form, errors.E(op, errors.InvalidForm,
			errors.Info("Change.Password"),
			"required field",
		)
	}

	// and permissions, these should only be editable if we're admin
	if isAdmin {
		new.UserPermissions = fromUserPermissionSlice(form.Permissions)
	}

	isNewProfile := new.DJ.ID == 0 && r.MultipartForm.Value["DJ.Name"] != nil
	// if the user has no DJ profile we're basically done here, unless
	// an admin is trying to create a profile.
	if isNewAccount || (isNewProfile && !isAdmin) {
		_, err = userStorage.UpdateUser(new)
		if err != nil {
			return &form, errors.E(op, errors.InternalServer, err)
		}
		// we want to go back to where we came from, unless we just made a new
		// account, in which case we want to redirect to the new account
		if isNewAccount {
			q := r.URL.Query()
			q.Del("new")
			q.Add("username", new.Username)
			r.URL.RawQuery = q.Encode()
		}
		return nil, nil
	}

	// copy this the other way so that the template rendering knows we have a DJ
	form.DJ.ID = new.DJ.ID
	form.DJ.Image = new.DJ.Image

	// copy over fields, and supply defaults if it's a new profile with no input
	new.IP = form.IP
	new.DJ.Visible = form.DJ.Visible

	// check if the regex input compiles
	_, err = regexp.Compile(`(?i)` + form.DJ.Regex)
	if err != nil {
		return &form, errors.E(op, errors.InvalidForm,
			errors.Info("DJ.Regex"),
			err,
		)
	}
	new.DJ.Regex = form.DJ.Regex

	// only set a default priority if it's a new profile, otherwise 0 is a valid
	// priority
	if isNewProfile && form.DJ.Priority == 0 {
		new.DJ.Priority = 200
	} else {
		new.DJ.Priority = form.DJ.Priority
	}

	if form.DJ.Name == "" {
		// required field
		return &form, errors.E(op, errors.InvalidForm,
			errors.Info("DJ.Name"),
			"required field",
		)
	}
	new.DJ.Name = form.DJ.Name

	if isNewProfile && form.DJ.Theme.Name == "" {
		new.DJ.Theme.Name = "default"
	} else {
		new.DJ.Theme.Name = form.DJ.Theme.Name
	}

	beforeSave := new // only stored for debugging purpose
	_ = beforeSave
	// now, we only have the DJ image left to handle, but since it uses the DJID
	// to save the image, and we might not have one yet. We're going to store what
	// we have so far. And then store it again afterwards once the image handling
	// is done.
	if isNewProfile {
		new, err = userStorage.UpdateUser(new)
		if err != nil {
			return &form, errors.E(op, errors.InternalServer, err)
		}
	}

	// now handle dj image changes, then save again after
	if f := r.MultipartForm.File["DJ.Image"]; len(f) > 0 {
		err := postProfileImage(s.Config, &new, f[0])
		if err != nil {
			// error, something failed in image handling
			return &form, errors.E(op, err)
		}
	}

	new, err = userStorage.UpdateUser(new)
	if err != nil {
		return &form, errors.E(op, errors.InternalServer, err)
	}

	if isNewProfile {
		q := r.URL.Query()
		q.Del("newprofile")
		r.URL.RawQuery = q.Encode()
	}

	return nil, nil
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
		err := new.ComparePassword(form.Password)
		if err != nil {
			// error, current password doesn't match actual password
			return errors.E(op, errors.InvalidForm,
				errors.Info("Change.Password"),
				"invalid password",
			)
		}
	}

	hashed, err := radio.GenerateHashFromPassword(form.NewPassword)
	if err != nil {
		// error, failed to generate bcrypt from password
		// no idea what could cause this to error, so we just throw up
		return errors.E(op, errors.InternalServer, err)
	}

	new.Password = hashed
	return nil
}

func postProfileImage(cfg config.Config, new *radio.User, header *multipart.FileHeader) error {
	const op errors.Op = "website/admin.postProfileImage"

	imageDir := cfg.Conf().Website.DJImagePath

	if header.Size > cfg.Conf().Website.DJImageMaxSize {
		return errors.E(op, errors.InvalidForm)
	}

	// make the parent directories if they don't exist
	err := os.MkdirAll(imageDir, 0755)
	if err != nil {
		return errors.E(op, errors.InternalServer, err)
	}

	in, err := header.Open()
	if err != nil {
		return errors.E(op, errors.InternalServer, err)
	}
	defer in.Close()

	out, err := os.CreateTemp(imageDir, "upload")
	if err != nil {
		return errors.E(op, errors.InternalServer, err)
	}
	defer func(f *os.File) {
		out.Close()
		// cleanup file, but after a successful rename it shouldn't exist anymore
		// so this is just for if we exit early
		os.Remove(f.Name())
		// TODO: probably log any errors so we don't have millions
		// of temp files leftover at some point
	}(out)

	hash := sha256.New()

	// copy it to the final destination, but with a temporary name so we can hash
	// it and use that in the final name
	_, err = io.Copy(io.MultiWriter(hash, out), in)
	if err != nil {
		return errors.E(op, errors.InternalServer, err)
	}
	// successfully copied it to the final destination, just doesn't have its final
	// name yet
	if err = out.Close(); err != nil {
		return errors.E(op, errors.InternalServer, err)
	}

	// grab the final sha256 hash, we only use the first few bytes of it because we
	// don't really want extremely long filenames
	sum := hash.Sum(nil)[:8]
	// we store the file on-disk with just the DJ ID, but in the database with the
	// ID prefixed and a hash affixed, so we can use cloudflare cacheing
	imageFilename := fmt.Sprintf("%d", new.DJ.ID)
	imageFilenameDB := fmt.Sprintf("%d-%x.png", new.DJ.ID, sum)
	imagePath := filepath.Join(imageDir, imageFilename)

	// and rename the file to the final resting place
	err = os.Rename(out.Name(), imagePath)
	if err != nil && !os.IsExist(err) {
		return errors.E(op, errors.InternalServer, err)
	}

	new.DJ.Image = imageFilenameDB
	return nil
}
