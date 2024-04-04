package admin

import (
	"crypto/sha256"
	"fmt"
	"html/template"
	"io"
	"io/fs"
	"log"
	"mime/multipart"
	"net/http"
	"net/url"
	"os"
	"strconv"

	radio "github.com/R-a-dio/valkyrie"
	"github.com/R-a-dio/valkyrie/errors"
	"github.com/R-a-dio/valkyrie/templates"
	"github.com/R-a-dio/valkyrie/website/middleware"
	"github.com/spf13/afero"
)

const (
	DefaultDJPriority = 200
	DefaultTheme      = templates.DEFAULT_DIR
)

// ProfileForm defines the form we use for the profile page
type ProfileForm struct {
	radio.User

	// IsAdmin indicates if we're setting up the admin-only form
	IsAdmin bool
	// IsSelf indicates if we're setting up a form for ourselves, can only
	// be false if IsAdmin is true
	IsSelf bool
	// UserIP is the users current IP, as a template.JS because we include
	// it in a button to set the IP field
	CurrentIP template.JS
	// PasswordChangeForm holds the fields required for changing a password
	PasswordChangeForm ProfilePasswordChangeForm
	// Errors that occured while parsing the form
	Errors map[string]string

	// newPermissions holds the permissions list received from the client
	newPermissions radio.UserPermissions
}

func (ProfileForm) TemplateBundle() string {
	return "profile"
}

func (ProfileForm) TemplateName() string {
	return "form_profile"
}

// GetProfile is mounted under /admin/profile and shows the currently logged
// in users profile
func (s *State) GetProfile(w http.ResponseWriter, r *http.Request) {
	err := s.getProfile(w, r)
	if err != nil {
		s.errorHandler(w, r, err, "")
		return
	}
}

type ProfileInput struct {
	middleware.Input

	Form ProfileForm
}

func (ProfileInput) TemplateBundle() string {
	return "profile"
}

func NewProfileInput(forUser radio.User, r *http.Request) (*ProfileInput, error) {
	input := ProfileInput{
		Input: middleware.InputFromRequest(r),
		Form:  newProfileForm(forUser, r),
	}

	return &input, nil
}

type ProfilePasswordChangeForm struct {
	// For is the user we're changing the password for
	For radio.User
	// Current is the current password for the user
	Current string
	// New is the new password for the user
	New string
	// Repeated is New repeated
	Repeated string
}

func (s *State) getProfile(w http.ResponseWriter, r *http.Request) error {
	const op errors.Op = "website/admin.getProfile"
	ctx := r.Context()

	user := middleware.UserFromContext(ctx)

	// if admin, they can see other users, check if that is the case
	if user.UserPermissions.Has(radio.PermAdmin) {
		username := r.FormValue("username")
		if username != "" {
			// lookup provided username and use that for the form instead
			other, err := s.Storage.User(ctx).Get(username)
			if err != nil {
				return errors.E(op, err)
			}
			user = other
		}
	}

	input, err := NewProfileInput(*user, r)
	if err != nil {
		return errors.E(op, err)
	}

	err = s.TemplateExecutor.Execute(w, r, input)
	if err != nil {
		return errors.E(op, err)
	}
	return nil
}

// PostProfile implements the profile form POST parsing and handling
//
// The expected form as input is defined in templates/partials/form_admin_profile
func (s *State) PostProfile(w http.ResponseWriter, r *http.Request) {
	form, err := s.postProfile(w, r)
	if err != nil {
		s.errorHandler(w, r, err, "failed post profile")
		return
	}

	err = s.TemplateExecutor.Execute(w, r, form)
	if err != nil {
		s.errorHandler(w, r, err, "template failure")
		return
	}

	http.Redirect(w, r, r.URL.String(), http.StatusSeeOther)
}

func (s *State) postProfile(w http.ResponseWriter, r *http.Request) (templates.TemplateSelectable, error) {
	const op errors.Op = "website/admin.postProfile"

	ctx := r.Context()
	// current user of this session
	currentUser := middleware.UserFromContext(ctx)
	// the user we're editing
	toEdit := currentUser

	// now first parse the form
	err := r.ParseForm()
	if err != nil {
		return nil, errors.E(op, errors.InvalidForm, err)
	}

	err = r.ParseMultipartForm(16 * 1024)
	// the above will error if the form submitted wasn't multipart/form-data but
	// that isn't a critical error so continue if that occurs
	if err != nil && !errors.IsE(err, http.ErrNotMultipart) {
		return nil, errors.E(op, errors.InvalidForm, err)
	}

	// now if the user is admin, we need to check if we are working on
	// another user
	if currentUser.UserPermissions.Has(radio.PermAdmin) {
		username := r.PostFormValue("username")
		if username == "" { // double check that there is an actual user
			return nil, errors.E(op, errors.InvalidForm, "no username in form")
		}
		// check if the username differs from what we were planning to edit
		if username != toEdit.Username {
			// grab the other user
			toEdit, err = s.Storage.User(ctx).Get(username)
			if err != nil {
				return nil, errors.E(op, err, errors.InternalServer)
			}
		}
	}

	// parse the request form values into our form struct
	form, err := NewProfileForm(*toEdit, r)
	if err != nil {
		return nil, errors.E(op, err)
	}

	// check if we're asking to make a new user
	if r.PostFormValue("new") != "" {
		return s.postNewProfile(w, r, form)
	}

	// check for a password change
	if cf := form.PasswordChangeForm; cf.New != "" {
		new, err := postProfilePassword(cf, currentUser.UserPermissions.Has(radio.PermAdmin))
		if err != nil {
			return form, errors.E(op, err)
		}
		form.Password = new
	}

	// check for a image change
	if f := r.MultipartForm.File["dj.image"]; len(f) > 0 {
		imagePath, err := postProfileImage(
			afero.NewBasePathFs(s.FS, s.Conf().Website.DJImagePath),
			form.DJ.ID,
			s.Conf().Website.DJImageMaxSize,
			f[0],
		)
		if err != nil {
			return form, errors.E(op, err)
		}
		form.DJ.Image = imagePath
	}

	// apply any permission change, only admins can change this (for now)
	if currentUser.UserPermissions.Has(radio.PermAdmin) {
		form.User.UserPermissions = form.newPermissions
	}

	// update the user in the database
	updatedUser, err := s.Storage.User(ctx).Update(form.User)
	if err != nil {
		return form, errors.E(op, err)
	}

	// updatedUser and form.User should already be matching, but just to support
	// UpdateUser actually changing something we apply it to our form before
	// returning
	form.User = updatedUser
	return form, nil
}

func (s *State) postNewProfile(w http.ResponseWriter, r *http.Request, form *ProfileForm) (templates.TemplateSelectable, error) {
	const op errors.Op = "website/admin.postNewProfile"
	ctx := r.Context()

	// generate a password for the user
	newPassword, err := postProfilePassword(form.PasswordChangeForm, true)
	if err != nil {
		return form, errors.E(op, err)
	}
	form.Password = newPassword

	// add the active permissions to the new user
	form.UserPermissions[radio.PermActive] = struct{}{}

	uid, err := s.Storage.User(ctx).Create(form.User)
	if err != nil {
		return form, errors.E(op, err)
	}
	form.User.ID = uid
	return form, nil
}

func postProfilePassword(form ProfilePasswordChangeForm, isAdmin bool) (string, error) {
	const op errors.Op = "website/admin.postProfilePassword"

	// check if New and Repeated match
	if form.New != form.Repeated {
		return "", errors.E(op, errors.InvalidForm,
			errors.Info("password.repeated"),
			"repeated password did not match new password",
		)
	}

	// check if we have the current password for the user
	if !isAdmin {
		// no current password given at all
		if form.Current == "" {
			return "", errors.E(op, errors.InvalidForm,
				errors.Info("password.current"),
				"current password is required to change password",
			)
		}

		// compare the given password to the actual current password
		err := form.For.ComparePassword(form.Current)
		if err != nil {
			return "", errors.E(op, errors.AccessDenied,
				errors.Info("password.current"),
				"invalid password",
			)
		}
	}

	// everything good, generate the new password
	hashed, err := radio.GenerateHashFromPassword(form.New)
	if err != nil {
		// error, failed to generate bcrypt from password
		// no idea what could cause this to error, so we just throw up
		return "", errors.E(op, errors.InternalServer, err)
	}

	return hashed, nil
}

func postProfileImage(fsys afero.Fs, id radio.DJID, maxSize int64, header *multipart.FileHeader) (string, error) {
	const op errors.Op = "website/admin.postProfileImage"

	if header.Size > maxSize {
		return "", errors.E(op, errors.InvalidForm)
	}

	in, err := header.Open()
	if err != nil {
		return "", errors.E(op, errors.InternalServer, err)
	}
	defer in.Close()

	out, err := afero.TempFile(fsys, "", "upload")
	if err != nil {
		return "", errors.E(op, errors.InternalServer, err)
	}
	defer func(f afero.File) {
		f.Close()
		// cleanup file, but after a successful rename it shouldn't exist anymore
		// so this is just for if we exit early
		err := fsys.Remove(f.Name())
		if err != nil && !errors.IsE(err, fs.ErrNotExist) {
			// TODO: probably log any errors so we don't have millions
			// of temp files leftover at some point
			log.Println("dj image upload removal failure:", err)
		}

	}(out)

	hash := sha256.New()

	// copy it to the final destination, but with a temporary name so we can hash
	// it and use that in the final name
	_, err = io.Copy(io.MultiWriter(hash, out), in)
	if err != nil {
		return "", errors.E(op, errors.InternalServer, err)
	}
	// successfully copied it to the final destination, just doesn't have its final
	// name yet
	if err = out.Close(); err != nil {
		return "", errors.E(op, errors.InternalServer, err)
	}

	// grab the final sha256 hash, we only use the first few bytes of it because we
	// don't really want extremely long filenames
	sum := hash.Sum(nil)[:8]
	// we store the file on-disk with just the DJ ID, but in the database with the
	// ID prefixed and a hash affixed, so we can use cloudflare cacheing
	imageFilename := fmt.Sprintf("%d", id)
	imageFilenameDB := fmt.Sprintf("%d-%x.png", id, sum)
	imagePath := imageFilename

	// and rename the file to its final resting place
	err = fsys.Rename(out.Name(), imagePath)
	if err != nil && !os.IsExist(err) {
		return "", errors.E(op, errors.InternalServer, err)
	}

	return imageFilenameDB, nil
}

func NewProfileForm(user radio.User, r *http.Request) (*ProfileForm, error) {
	const op errors.Op = "website/admin.NewProfileForm"

	// convert to url.Values for the helper methods
	values := url.Values(r.MultipartForm.Value)

	// initial check to see if we're actually editing the expected user
	if values.Get("username") != user.Username {
		return nil, errors.E(op, errors.AccessDenied)
	}

	form := newProfileForm(user, r)
	form.Update(values)
	return &form, nil
}

func newProfileForm(user radio.User, r *http.Request) ProfileForm {
	requestUser := middleware.UserFromContext(r.Context())
	return ProfileForm{
		User:      user,
		IsAdmin:   requestUser.UserPermissions.Has(radio.PermAdmin),
		IsSelf:    requestUser.Username == user.Username,
		CurrentIP: template.JS(r.RemoteAddr),
		PasswordChangeForm: ProfilePasswordChangeForm{
			For: user,
		},
	}
}

func (pf *ProfileForm) Update(form url.Values) {
	pf.Username = form.Get("username")
	pf.IP = form.Get("ip")
	pf.Email = form.Get("email")
	pf.DJ.Visible = form.Get("dj.visible") != ""
	pf.DJ.Name = form.Get("dj.name")
	if prio, err := strconv.Atoi(form.Get("dj.priority")); err == nil {
		pf.DJ.Priority = prio
	}
	pf.DJ.Regex = form.Get("dj.regex")
	pf.DJ.Theme.Name = form.Get("dj.theme.name")

	// password handling
	pf.PasswordChangeForm.Current = form.Get("password.current")
	pf.PasswordChangeForm.New = form.Get("password.new")
	pf.PasswordChangeForm.Repeated = form.Get("password.repeated")

	pf.newPermissions = make(radio.UserPermissions)
	for _, perm := range form["permissions"] {
		pf.newPermissions[radio.UserPermission(perm)] = struct{}{}
	}
}
