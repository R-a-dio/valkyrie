// Package templates handles the website templating system.
//
// This supports several 'themes' and partials for template-reuse
package templates

import (
	"fmt"
	"html/template"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"text/tabwriter"

	"github.com/R-a-dio/valkyrie/errors"
	"golang.org/x/exp/maps"
)

const (
	// the extension used for template files
	TEMPLATE_EXT = ".tmpl"
	// the directory for static assets
	ASSETS_DIR = "assets"
	// the directory name used for partial templates, these are under <theme>/partials
	PARTIAL_DIR = "partials"
	// the directory name for form templates, these are under <theme>/forms
	FORMS_DIR = "forms"
	// directory name of the default templates
	DEFAULT_DIR = "default-light"
	// directory name of the default admin templates
	DEFAULT_ADMIN_DIR = "admin-light"
	// the prefix used on themes that are for the admin panel
	ADMIN_PREFIX = "admin-"
)

// Site is an overarching struct containing all the themes of the website.
type Site struct {
	Production bool

	fs fs.FS

	mu     sync.RWMutex
	themes Themes
	cache  map[string]*template.Template
}

func (s *Site) Reload() error {
	const op errors.Op = "templates/Reload"

	s.mu.Lock()
	defer s.mu.Unlock()

	themes, err := LoadThemes(s.fs)
	if err != nil {
		return errors.E(op, err)
	}
	s.themes = themes
	return nil
}

type TemplateSelector interface {
	Template(theme, page string) (*template.Template, error)
}

func (s *Site) Executor() Executor {
	return newExecutor(s)
}

func (s *Site) ThemeNames() []string {
	s.mu.RLock()
	defer s.mu.RUnlock()

	keys := maps.Keys(s.themes)
	slices.Sort(keys)
	return keys
}

// Template returns a Template associated with the theme and page name given.
//
// If theme does not exist it uses the default-theme
func (s *Site) Template(theme, page string) (*template.Template, error) {
	if s.Production {
		return s.prodTemplate(theme, page)
	}
	return s.devTemplate(theme, page)
}

// devTemplate is the Template implementation used during development such that
// all files are reread and reparsed on every invocation.
func (s *Site) devTemplate(theme, page string) (*template.Template, error) {
	const op errors.Op = "templates/devTemplate"

	if err := s.Reload(); err != nil {
		return nil, errors.E(op, err)
	}

	pb, err := s.Theme(theme).Page(page)
	if err != nil {
		return nil, errors.E(op, err)
	}

	tmpl, err := pb.Template()
	if err != nil {
		return nil, errors.E(op, err)
	}

	return tmpl, nil
}

// prodTemplate is the Template implementation used for production, this implementation
// caches a *template.Template after its first use
func (s *Site) prodTemplate(theme, page string) (*template.Template, error) {
	const op errors.Op = "templates/prodTemplate"

	// resolve theme name so that it's either an existing theme or default
	theme = s.ResolveThemeName(theme)
	// merge theme and page into a key we can use for our cache map
	key := theme + "/" + page

	if tmpl, ok := s.cache[key]; ok {
		return tmpl, nil
	}

	pb, err := s.Theme(theme).Page(page)
	if err != nil {
		return nil, errors.E(op, err)
	}

	tmpl, err := pb.Template()
	if err != nil {
		return nil, errors.E(op, err)
	}

	s.cache[key] = tmpl
	return tmpl, nil
}

func (s *Site) Theme(name string) ThemeBundle {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if ps, ok := s.themes[name]; ok {
		return ps
	}
	return s.themes[DEFAULT_DIR]
}

func (s *Site) ResolveThemeName(name string) string {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if _, ok := s.themes[name]; ok {
		return name
	}
	return DEFAULT_DIR
}

func FromDirectory(dir string) (*Site, error) {
	const op errors.Op = "templates/FromDirectory"

	fsys := os.DirFS(dir)
	s, err := FromFS(fsys)
	if err != nil {
		return nil, errors.E(op, err)
	}
	return s, nil
}

func FromFS(fsys fs.FS) (*Site, error) {
	const op errors.Op = "templates/FromFS"

	var err error
	tmpl := Site{
		fs:    fsys,
		cache: make(map[string]*template.Template),
	}

	tmpl.themes, err = LoadThemes(fsys)
	if err != nil {
		return nil, errors.E(op, err)
	}

	return &tmpl, nil
}

// TemplateBundle contains all the filenames required to construct a template instance
// for the page
type TemplateBundle struct {
	// fs to load the relative-filenames below
	fs fs.FS
	// the following fields contain all the filenames of the templates we're parsing
	// into a html/template.Template. They're listed in load-order, last one wins.
	base            []string
	defaultForms    []string
	forms           []string
	defaultPartials []string
	partials        []string
	defaultPage     string
	page            string
}

func (tb *TemplateBundle) Dump() string {
	var res strings.Builder

	res.WriteString("===================================\n")
	res.WriteString("base templates:\n")
	for i, filename := range tb.base {
		fmt.Fprintf(&res, "	%d: %s\n", i, filename)
	}
	res.WriteString("default forms:\n")
	for i, filename := range tb.defaultForms {
		fmt.Fprintf(&res, "	%d: %s\n", i, filename)
	}
	res.WriteString("forms:\n")
	for i, filename := range tb.forms {
		fmt.Fprintf(&res, "	%d: %s\n", i, filename)
	}
	res.WriteString("default partials:\n")
	for i, filename := range tb.defaultPartials {
		fmt.Fprintf(&res, "	%d: %s\n", i, filename)
	}
	res.WriteString("partials:\n")
	for i, filename := range tb.partials {
		fmt.Fprintf(&res, "	%d: %s\n", i, filename)
	}
	res.WriteString("default page:\n")
	fmt.Fprintf(&res, "	%s\n", tb.defaultPage)
	res.WriteString("page:\n")
	fmt.Fprintf(&res, "	%s\n", tb.page)
	return res.String()
}

// Files returns all the files in this bundle sorted in load-order
func (tb *TemplateBundle) Files() []string {
	s := make([]string, 0, len(tb.base)+len(tb.defaultForms)+len(tb.forms)+len(tb.defaultPartials)+len(tb.partials)+2)
	s = append(s, tb.base...)
	s = append(s, tb.defaultForms...)
	s = append(s, tb.forms...)
	s = append(s, tb.defaultPartials...)
	s = append(s, tb.partials...)
	if tb.defaultPage != "" {
		s = append(s, tb.defaultPage)
	}
	if tb.page != "" {
		s = append(s, tb.page)
	}
	return s
}

// Template returns a *html.Template with all files contained in this bundle
func (tb *TemplateBundle) Template() (*template.Template, error) {
	const op errors.Op = "templates/TemplateBundle.Template"

	tmpl, err := createRoot().ParseFS(tb.fs, tb.Files()...)
	if err != nil {
		return nil, errors.E(op, errors.TemplateParseError, err)
	}
	return tmpl, nil
}

// createRoot creates a root template that adds global utility functions to
// all other template files.
func createRoot() *template.Template {
	return template.New("root").Funcs(fnMap)
}

type Themes map[string]ThemeBundle

// ThemeBundle
type ThemeBundle struct {
	name   string
	pages  map[string]*TemplateBundle
	assets fs.FS
}

func (tb ThemeBundle) Page(name string) (*TemplateBundle, error) {
	const op errors.Op = "templates/ThemeBundle.Page"

	tlb, ok := tb.pages[name]
	if !ok {
		return nil, errors.E(op, errors.TemplateUnknown, errors.Info(fmt.Sprintf("page: %s", name)))
	}

	return tlb, nil
}

type noopFS struct{}

func (noopFS) Open(string) (fs.File, error) {
	return nil, fs.ErrNotExist
}

func (tb ThemeBundle) Assets() fs.FS {
	if tb.assets == nil {
		return noopFS{}
	}
	return tb.assets
}

type loadState struct {
	fs fs.FS

	baseTemplates   []string
	defaultPartials []string
	defaultForms    []string
	defaultBundle   map[string]*TemplateBundle
}

func LoadThemes(fsys fs.FS) (Themes, error) {
	const op errors.Op = "templates/LoadThemes"

	var state loadState
	var err error

	state.fs = fsys
	state.baseTemplates, err = readDirFilterString(fsys, ".", isTemplate)
	if err != nil {
		return nil, errors.E(op, err)
	}

	// find our default directory
	state.defaultBundle, err = state.loadSubDir(DEFAULT_DIR)
	if err != nil {
		return nil, errors.E(op, err)
	}
	// sanity check that we have atleast 1 bundle
	if len(state.defaultBundle) == 0 {
		return nil, errors.E(op, "default bundle empty")
	}

	// grab the partials from any template bundle
	for _, v := range state.defaultBundle {
		state.defaultPartials = v.partials
		state.defaultForms = v.forms
		break
	}

	// get the assets directory fs
	assets, err := fs.Sub(fsys, path.Join(DEFAULT_DIR, ASSETS_DIR))
	if err != nil && !errors.IsE(err, os.ErrNotExist) {
		return nil, errors.E(op, err)
	}

	// read the rest of the directories
	subdirs, err := readDirFilterString(fsys, ".", func(e fs.DirEntry) bool {
		isExcluded := strings.HasPrefix(e.Name(), ".")
		return !isExcluded && e.IsDir()
	})
	if err != nil {
		return nil, errors.E(op, err)
	}

	// add our default directory that we loaded above as a ThemeBundle
	themes := Themes{
		DEFAULT_DIR: ThemeBundle{DEFAULT_DIR, state.defaultBundle, assets},
	}
	// then read the rest of the themes
	for _, subdir := range subdirs {
		if subdir == DEFAULT_DIR { // skip the default dir since we already loaded it earlier
			continue
		}
		bundles, err := state.loadSubDir(subdir)
		if err != nil {
			return nil, errors.E(op, err)
		}

		assets, err := fs.Sub(fsys, path.Join(subdir, ASSETS_DIR))
		if err != nil && !errors.IsE(err, os.ErrNotExist) {
			return nil, errors.E(op, err)
		}

		themes[subdir] = ThemeBundle{
			name:   subdir,
			pages:  bundles,
			assets: assets,
		}
	}

	return themes, nil
}

// noExt removes the extension of s as returned by filepath.Ext
func noExt(s string) string {
	return strings.TrimSuffix(filepath.Base(s), filepath.Ext(s))
}

// loadSubDir searches a subdirectory of the FS used in the creation of the loader.
//
// it looks for `*.tmpl` files in this subdirectory and in a `partials/` subdirectory
// if one exists. Returns a map of `filename:bundle` where the bundle is a TemplateBundle
// that contains all the filenames required to construct the page named after the filename.
func (ls loadState) loadSubDir(dir string) (map[string]*TemplateBundle, error) {
	const op errors.Op = "templates/loadState.loadSubDir"

	var bundle = TemplateBundle{
		fs:              ls.fs,
		base:            ls.baseTemplates,
		defaultPartials: ls.defaultPartials,
		defaultForms:    ls.defaultForms,
	}

	// read the forms subdirectory
	formDir := path.Join(dir, FORMS_DIR)

	entries, err := readDirFilter(ls.fs, formDir, isTemplate)
	if err != nil && !errors.IsE(err, fs.ErrNotExist) {
		return nil, errors.E(op, err)
	}

	var forms = make([]string, 0, len(entries))
	for _, entry := range entries {
		forms = append(forms, path.Join(formDir, entry.Name()))
	}

	bundle.forms = forms

	// read the partials subdirectory
	partialDir := path.Join(dir, PARTIAL_DIR)

	entries, err = readDirFilter(ls.fs, partialDir, isTemplate)
	if err != nil && !errors.IsE(err, fs.ErrNotExist) {
		return nil, errors.E(op, err)
	}

	var partials = make([]string, 0, len(entries))
	for _, entry := range entries {
		partials = append(partials, path.Join(partialDir, entry.Name()))
	}

	bundle.partials = partials

	// read the actual directory
	entries, err = readDirFilter(ls.fs, dir, isTemplate)
	if err != nil {
		return nil, errors.E(op, err)
	}

	var bundles = make(map[string]*TemplateBundle, len(entries))
	for _, entry := range entries {
		name := entry.Name()
		// create a bundle for each page in this directory
		pageBundle := bundle

		defaultPage := ls.defaultBundle[noExt(name)]
		if defaultPage != nil {
			pageBundle.defaultPage = defaultPage.page
		}
		pageBundle.page = path.Join(dir, name)

		bundles[noExt(name)] = &pageBundle
	}

	// if there are no defaults to handle, we're done
	if ls.defaultBundle == nil {
		return bundles, nil
	}

	// otherwise check for missing pages, these are pages defined
	// in the default theme but not in this current theme. Copy over
	// the default pages if they're missing.
	for name, page := range ls.defaultBundle {
		if _, ok := bundles[name]; ok {
			continue
		}

		pageBundle := bundle
		pageBundle.defaultPage = page.page
		bundles[name] = &pageBundle
	}

	return bundles, nil
}

// readDirFilter is fs.ReadDir but with an added filter function.
func readDirFilter(fsys fs.FS, name string, fn func(fs.DirEntry) bool) ([]fs.DirEntry, error) {
	const op errors.Op = "templates/readDirFilter"

	entries, err := fs.ReadDir(fsys, name)
	if err != nil {
		return nil, errors.E(op, err)
	}

	var fe []fs.DirEntry
	for _, entry := range entries {
		if fn(entry) {
			fe = append(fe, entry)
		}
	}

	return fe, nil
}

// readDirFilterString is readDirFilter but with the returned entries turned into strings
// by using entry.Name()
func readDirFilterString(fsys fs.FS, name string, fn func(fs.DirEntry) bool) ([]string, error) {
	const op errors.Op = "templates/readDirFilterString"

	entries, err := readDirFilter(fsys, ".", fn)
	if err != nil {
		return nil, errors.E(op, err)
	}

	s := make([]string, 0, len(entries))
	for _, entry := range entries {
		s = append(s, entry.Name())
	}

	return s, nil
}

// isTemplate checks if this entry is a template according to our definition
func isTemplate(e os.DirEntry) bool {
	return !e.IsDir() && filepath.Ext(e.Name()) == TEMPLATE_EXT
}

// Definitions prints a table showing what templates are defined in this Template and
// from what file it was loaded. The last template in the table is the one in-use.
func Definitions(fsys fs.FS, files []string) error {
	const op errors.Op = "templates/Definitions"
	const noop = "--noop--"

	columns := []string{"filename"}
	var cc = make(map[string]bool)
	type row struct {
		filename string
		names    map[string]bool
	}

	rows := []row{}

	// go through each file
	//var cache filecache

	for _, filename := range files {
		b, err := fs.ReadFile(fsys, filename)
		if err != nil {
			return err
		}
		contents := string(b)

		tmpl, err := createRoot().New(noop).Parse(contents)
		if err != nil {
			return errors.E(op, err)
		}

		var r row
		r.filename = filename
		r.names = make(map[string]bool)
		for _, a := range tmpl.Templates() {
			name := a.Name()
			if name == noop { // skip our noop
				continue
			}
			r.names[name] = true
			// check if it's a new template we found
			if !cc[name] && name != "root" {
				cc[name] = true
				columns = append(columns, name)
			}
		}

		rows = append(rows, r)
	}

	slices.Sort(columns[1:])

	data := make([][]string, 0, len(rows))

	data = append(data, columns)
	for _, r := range rows {
		s := []string{r.filename}
		for _, c := range columns[1:] {
			if r.names[c] {
				s = append(s, "  X")
			} else {
				s = append(s, "")
			}
		}
		data = append(data, s)
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 3, ' ', tabwriter.Debug)
	for a := range data {
		fmt.Fprintln(w, strings.Join(data[a], "\t"))
	}
	return w.Flush()
}
