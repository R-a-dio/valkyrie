package navbar

import (
	"html/template"

	radio "github.com/R-a-dio/valkyrie"
	"github.com/elliotchance/orderedmap/v3"
)

func New(attrs template.HTMLAttr, items ...Item) NavBar {
	om := orderedmap.NewOrderedMap[string, Item]()
	for _, item := range items {
		om.Set(item.Name, item)
	}

	return NavBar{items: om, Attr: attrs}
}

// NavBar consists of the items that should be contained in a navigation bar
// on the website, items can be iterated by using the following template range
// {{range .NavBar.Values}} the dot will be set to each NavBarItem in order.
//
// an example of rendering the whole navbar would be:
//
//	{{range .NavBar.Values}}<a {{.Attr}}>{{.Name}}</a>{{end}}
//
// if you want to use a non-default ordering you need to call Get manually
// but remember that this might break if a name changes at a later point and
// so use of Get should always use {{with .NavBar.Get("name")}} to make sure
// the item actually exists
type NavBar struct {
	items *orderedmap.OrderedMap[string, Item]
	// user is an optional user this navbar is being rendered for,
	// this is used in the permission checks on iteration
	user *radio.User
	// Attr contains html attributes that should be applied
	// to the parent of the navbar items
	Attr template.HTMLAttr
}

// Values returns a sequence of all the items in this navbar
//
// TODO: change to iter.Seq once we switch to 1.24
func (nb NavBar) Values() []Item {
	v := make([]Item, 0, nb.items.Len())
	for item := range nb.items.Values() {
		if len(item.Perm) == 0 {
			v = append(v, item)
			continue
		}

		// no user so there would never be permission
		if nb.user == nil {
			continue
		}

		if nb.user.UserPermissions.Has(item.Perm) {
			v = append(v, item)
			continue
		}
	}

	return v
}

func (nb NavBar) WithUser(user *radio.User) NavBar {
	new := nb
	new.user = user
	return new
}

// Get returns an item by its name, returns nil if the name does not exist
func (nb NavBar) Get(name string) *Item {
	nbi, ok := nb.items.Get(name)
	if !ok {
		return nil
	}
	return &nbi
}

func NewProtectedItem(name string, perm radio.UserPermission, attr template.HTMLAttr) Item {
	return Item{
		Name: name,
		Attr: attr,
		Perm: perm,
	}
}

func NewItem(name string, attr template.HTMLAttr) Item {
	return Item{
		Name: name,
		Attr: attr,
		Perm: "",
	}
}

// Item is a single item that should be shown on a navigation bar, the
// simplest way to render these in a template would be:
//
//	`<a {{.Attr}}>{{.Name}}</a>`
type Item struct {
	// Name is the name to display to the user
	Name string
	// Attr is one (or more) html attributes that should be on the <a>
	// element of this navbar item, this includes the href attribute
	Attr template.HTMLAttr
	// Perm is the permission required to see this item
	Perm radio.UserPermission
}

func Attrs(entries ...template.HTMLAttr) template.HTMLAttr {
	if len(entries)%2 != 0 {
		panic("uneven number of arguments to Attrs")
	}

	var res template.HTMLAttr
	for i := 0; i < len(entries); i = i + 2 {
		res += ` ` + entries[i] + `="` + entries[i+1] + `"`
	}
	return res
}
