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
	"github.com/gorilla/csrf"
	"github.com/rs/zerolog"
	"github.com/spf13/afero"
)

const (
	DefaultDJPriority = 200
	DefaultDJTheme    = templates.DEFAULT_DIR
	DefaultDJRole     = "dj"

	profileNewUser = "user"
	profileNewDJ   = "dj"
)

func newProfileUser(username string) radio.User {
	return radio.User{
		Username: username,
		UserPermissions: radio.UserPermissions{
			radio.PermActive: struct{}{},
		},
	}
}

func newProfileDJ(name string) radio.DJ {
	return radio.DJ{
		Name:     name,
		Priority: DefaultDJPriority,
		Role:     DefaultDJRole,
	}
}

type ProfilePermissionEntry struct {
	Perm     radio.UserPermission
	Checked  bool
	Disabled bool
}

// ProfileForm defines the form we use for the profile page
type ProfileForm struct {
	radio.User

	// CSRFTokenInput is the <input> that should be included in the form for CSRF
	CSRFTokenInput template.HTML
	// PermissionList is the list of permissions we should render
	PermissionList []ProfilePermissionEntry
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

const profileFormAction = "/admin/profile"

func (ProfileForm) FormAction() template.HTMLAttr {
	return profileFormAction
}

func (ProfileForm) FormType() template.HTMLAttr {
	return `enctype="multipart/form-data"`
}

func (f *ProfileForm) AddDJProfileURL() template.URL {
	uri, err := url.Parse(profileFormAction)
	if err != nil {
		return ""
	}

	query := uri.Query()
	query.Set("username", f.Username)
	query.Set("new", profileNewDJ)
	uri.RawQuery = query.Encode()
	return template.URL(uri.String())
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
	if !user.IsValid() {
		panic("admin request with no user")
	}
	// the user we're viewing
	toView := *user

	// if admin, they can see other users, check if that is the case
	if user.UserPermissions.Has(radio.PermAdmin) {
		username := r.FormValue("username")
		if username != "" {
			// lookup provided username and use that for the form instead
			other, err := s.Storage.User(ctx).Get(username)
			if err != nil {
				return errors.E(op, err)
			}
			toView = *other
		}
	}

	input, err := NewProfileInput(toView, r)
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

func (s *State) postProfile(w http.ResponseWriter, r *http.Request) (*ProfileForm, error) {
	const op errors.Op = "website/admin.postProfile"

	ctx := r.Context()
	// current user of this session
	currentUser := middleware.UserFromContext(ctx)
	if !currentUser.IsValid() {
		panic("admin request with no user")
	}
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
	if errors.IsE(err, http.ErrNotMultipart) {
		err = nil
	}
	if err != nil {
		return nil, errors.E(op, errors.InvalidForm, err)
	}

	// check if a new user or dj is being made
	if r.FormValue("new") != "" {
		// only admins are allowed to do this
		if !currentUser.UserPermissions.Has(radio.PermAdmin) {
			return nil, errors.E(op, errors.AccessDenied)
		}

		return s.postNewProfile(r)
	}

	// not a new user, but might still be an admin editing someone else so
	// check if the username is different
	if currentUser.UserPermissions.Has(radio.PermAdmin) {
		username := r.FormValue("username")
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

	// check for a password change
	if cf := form.PasswordChangeForm; cf.New != "" {
		new, err := postProfilePassword(cf, currentUser.UserPermissions.Has(radio.PermAdmin))
		if err != nil {
			return form, errors.E(op, err)
		}
		form.Password = new
	}

	// check for a image change, but only if the user has a dj entry
	if toEdit.DJ.ID != 0 && r.MultipartForm != nil {
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
	}

	// apply any permission change, only admins can change this (for now)
	if currentUser.UserPermissions.Has(radio.PermAdmin) {
		form.User.UserPermissions = form.newPermissions
		form.PermissionList = generatePermissionList(*currentUser, form.User)
	}

	// update the user in the database
	_, err = s.Storage.User(ctx).Update(form.User)
	if err != nil {
		return form, errors.E(op, err)
	}

	// tell the manager to update any state
	err = s.Manager.UpdateFromStorage(ctx)
	if err != nil {
		// non-critical error, don't return it to the caller
		err = errors.E(op, err)
		zerolog.Ctx(ctx).Error().Err(err).Msg("failed to update storage")
	}

	return form, nil
}

// postNewProfileUser creates a new user from a username and a password
func (s *State) postNewProfileUser(r *http.Request, username string) (*ProfileForm, error) {
	const op errors.Op = "website/admin.postNewProfileUser"
	ctx := r.Context()

	user := newProfileUser(username)

	// parse the rest of the form
	form, err := NewProfileForm(user, r)
	if err != nil {
		return nil, errors.E(op, err)
	}

	// NewProfileForm will have emptied our UserPermissions because the new user
	// form doesn't send any, so just overwrite it here
	form.UserPermissions = user.UserPermissions

	// generate a password for the user
	newPassword, err := postProfilePassword(form.PasswordChangeForm, true)
	if err != nil {
		return form, errors.E(op, err)
	}
	form.Password = newPassword

	// and then create the user
	uid, err := s.Storage.User(ctx).Create(form.User)
	if err != nil {
		return form, errors.E(op, err)
	}
	form.User.ID = uid
	return form, nil
}

// postNewProfileDJ creates a new dj for an existing user username with default
// values.
func (s *State) postNewProfileDJ(r *http.Request, username string) (*ProfileForm, error) {
	const op errors.Op = "website/admin.postNewProfileDJ"
	ctx := r.Context()

	user, err := s.Storage.User(ctx).Get(username)
	if err != nil {
		return nil, errors.E(op, err, errors.InternalServer)
	}

	// parse the rest of the form
	form, err := NewProfileForm(*user, r)
	if err != nil {
		return nil, errors.E(op, err)
	}

	// create a DJ with default values
	dj := newProfileDJ(form.User.Username)

	// and then create the dj
	djid, err := s.Storage.User(ctx).CreateDJ(form.User, dj)
	if err != nil {
		return form, errors.E(op, err)
	}
	dj.ID = djid      // apply the id
	form.User.DJ = dj // then add it to the form we're returning
	return form, nil
}

func (s *State) postNewProfile(r *http.Request) (*ProfileForm, error) {
	const op errors.Op = "website/admin.postNewProfile"

	newMode := r.FormValue("new")       // what we're making a new thing of
	username := r.FormValue("username") // the user we're making a new thing for

	switch newMode {
	case profileNewUser:
		return s.postNewProfileUser(r, username)
	case profileNewDJ:
		return s.postNewProfileDJ(r, username)
	default:
		return nil, errors.E(op, errors.InvalidArgument, errors.Info(newMode))
	}
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
	defer func(f afero.File, filename string) {
		f.Close()
		// cleanup file, but after a successful rename it shouldn't exist anymore
		// so this is just for if we exit early
		err := fsys.Remove(filename)
		if err != nil && !errors.IsE(err, fs.ErrNotExist) {
			// TODO: probably log any errors so we don't have millions
			// of temp files leftover at some point
			log.Println("dj image upload removal failure:", err)
		}

	}(out, out.Name())

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

	values := r.PostForm

	// initial check to see if we're actually editing the expected user
	if values.Get("username") != user.Username {
		return nil, errors.E(op, errors.AccessDenied, "username does not match")
	}

	form := newProfileForm(user, r)
	form.Update(values)
	if form.IsAdmin { // only allow updating permissions by admin
		form.UserPermissions = form.newPermissions
	}
	return &form, nil
}

func newProfileForm(user radio.User, r *http.Request) ProfileForm {
	requestUser := middleware.UserFromContext(r.Context())
	if !requestUser.IsValid() {
		panic("admin request with no user")
	}

	return ProfileForm{
		User:           user,
		CSRFTokenInput: csrf.TemplateField(r),
		PermissionList: generatePermissionList(*requestUser, user),
		IsAdmin:        requestUser.UserPermissions.Has(radio.PermAdmin),
		IsSelf:         requestUser.Username == user.Username,
		CurrentIP:      template.JS(r.RemoteAddr),
		PasswordChangeForm: ProfilePasswordChangeForm{
			For: user,
		},
	}
}

func generatePermissionList(by, other radio.User) []ProfilePermissionEntry {
	var entries []ProfilePermissionEntry
	for _, perm := range radio.AllUserPermissions() {
		entries = append(entries, ProfilePermissionEntry{
			Perm:     perm,
			Checked:  other.UserPermissions.HasExplicit(perm),
			Disabled: !by.UserPermissions.HasEdit(perm),
		})
	}
	return entries
}

func (pf *ProfileForm) Update(form url.Values) {
	if form.Has("username") {
		pf.Username = form.Get("username")
	}
	if form.Has("ip") {
		pf.IP = form.Get("ip")
	}
	if form.Has("email") {
		pf.Email = form.Get("email")
	}
	if form.Has("dj.visible") {
		pf.DJ.Visible = form.Get("dj.visible") != ""
	}
	if form.Has("dj.name") {
		pf.DJ.Name = form.Get("dj.name")
	}
	if prio, err := strconv.Atoi(form.Get("dj.priority")); err == nil {
		pf.DJ.Priority = prio
	}
	if form.Has("dj.regex") {
		pf.DJ.Regex = form.Get("dj.regex")
	}
	if form.Has("dj.theme.name") {
		pf.DJ.Theme = form.Get("dj.theme.name")
	}
	if form.Has("dj.text") {
		pf.DJ.Text = form.Get("dj.text")
	}

	// password handling
	pf.PasswordChangeForm.Current = form.Get("password.current")
	pf.PasswordChangeForm.New = form.Get("password.new")
	pf.PasswordChangeForm.Repeated = form.Get("password.repeated")

	pf.newPermissions = make(radio.UserPermissions)
	for _, perm := range form["permissions"] {
		pf.newPermissions[radio.UserPermission(perm)] = struct{}{}
	}
}

func (pf *ProfileForm) ToValues() url.Values {
	values := url.Values{}

	// user fields
	values.Set("username", pf.Username)
	values.Set("ip", pf.IP)
	values.Set("email", pf.Email)
	for perm, _ := range pf.UserPermissions {
		values.Add("permissions", string(perm))
	}

	// dj fields
	if pf.DJ.ID != 0 {
		if pf.DJ.Visible {
			values.Set("dj.visible", "true")
		}
		if pf.DJ.Name != "" {
			values.Set("dj.name", pf.DJ.Name)
		}
		if pf.DJ.Text != "" {
			values.Set("dj.text", pf.DJ.Text)
		}
		values.Set("dj.priority", strconv.FormatInt(int64(pf.DJ.Priority), 10))
		values.Set("dj.regex", pf.DJ.Regex)
		if pf.DJ.Theme != "" {
			values.Set("dj.theme.name", pf.DJ.Theme)
		}
	}

	// password fields
	values.Set("password.new", pf.PasswordChangeForm.New)
	values.Set("password.current", pf.PasswordChangeForm.Current)
	values.Set("password.repeated", pf.PasswordChangeForm.Repeated)
	return values
}
