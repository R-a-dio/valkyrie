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
)

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
	if username := r.FormValue("username"); username != "" && isAdmin {
		// if it's an admin user they can look at all user accounts, so we
		// take an ?username=<username> query argument

		other, err := a.storage.User(ctx).Get(username)
		if err != nil {
			log.Println(err)
			return
		}

		output = other

		// an admin user can also change someones permissions, so we need to load
		// the available permissions
		availablePermissions = []string{"test"}
	}

	profileInput := struct {
		IsAdmin              bool
		CurrentIP            template.JS
		AvailablePermissions []string
		User                 *radio.User
		DJ                   radio.DJ
		Theme                radio.Theme
		Themes               []string
	}{
		IsAdmin:              isAdmin,
		CurrentIP:            template.JS(r.RemoteAddr),
		AvailablePermissions: availablePermissions,
		User:                 output,
		DJ:                   output.DJ,
		Theme:                output.DJ.Theme,
		Themes:               []string{},
	}

	err := a.templates["admin"]["profile.tmpl"].ExecuteDev(w, profileInput)
	if err != nil {
		log.Println(err)
		return
	}

	return
}

func (a admin) PostProfile(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	// current user of this session
	user := UserFromContext(ctx)
	isAdmin := user.UserPermissions.Has(radio.PermAdmin)

	err := r.ParseMultipartForm(16 * 1024)
	if err != nil {
		log.Println(err)
		return
	}

	// parse the form, all the data in here is untrusted so we need to check it after
	new, pwinfo := userFromPostForm(r.MultipartForm)

	// first thing to check is to see if we're working with the session user,
	// if we're not, the session user needs to be admin to touch other users
	if user.Username != new.Username && !isAdmin {
		http.Error(w, http.StatusText(403), 403)
		return
	}

	fmt.Printf("%#v %#v\n", new, pwinfo)
	http.Redirect(w, r, r.URL.String(), 200)
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
