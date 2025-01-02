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

	radio "github.com/R-a-dio/valkyrie"
	"github.com/R-a-dio/valkyrie/errors"
	"github.com/R-a-dio/valkyrie/util"
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
	// Production indicates if we should reload every page load
	Production bool
	// fnMap holds the functions we add to templates
	fnMap template.FuncMap

	// fs is the source fs for our template files
	fs fs.FS

	themes *util.TypedValue[Themes]
	cache  *util.Map[cacheKey, *template.Template]

	// mu protects the fields below it
	mu               sync.RWMutex
	themeNamesPublic []radio.ThemeName
	themeNamesAdmin  []radio.ThemeName
}

type cacheKey string

func (s *Site) Reload() error {
	const op errors.Op = "templates/Reload"

	if err := s.load(); err != nil {
		return errors.E(op, err)
	}

	return nil
}

func (s *Site) load() error {
	const op errors.Op = "templates/Site.load"

	themes, err := LoadThemes(s.fs, s.fnMap)
	if err != nil {
		return errors.E(op, err)
	}
	s.themes.Store(themes)
	s.populateNames()
	s.cache.Clear()

	return nil
}

func IsAdminTheme(name radio.ThemeName) bool {
	return strings.HasPrefix(string(name), ADMIN_PREFIX)
}

type TemplateSelector interface {
	Template(theme radio.ThemeName, page string) (*template.Template, error)
}

func (s *Site) Executor() Executor {
	return newExecutor(s)
}

func (s *Site) populateNames() {
	// populate the theme name lists, one for public, one for admin
	names := maps.Keys(s.themes.Load())
	slices.Sort(names)

	s.themeNamesAdmin = make([]radio.ThemeName, 0, len(s.themeNamesAdmin))
	s.themeNamesPublic = make([]radio.ThemeName, 0, len(s.themeNamesPublic))
	for _, name := range names {
		if IsAdminTheme(name) {
			s.themeNamesAdmin = append(s.themeNamesAdmin, name)
		} else {
			s.themeNamesPublic = append(s.themeNamesPublic, name)
		}
	}
}

func (s *Site) ThemeNames() []radio.ThemeName {
	s.mu.RLock()
	defer s.mu.RUnlock()

	return s.themeNamesPublic
}

func (s *Site) ThemeNamesAdmin() []radio.ThemeName {
	s.mu.RLock()
	defer s.mu.RUnlock()

	return s.themeNamesAdmin
}

// Template returns a Template associated with the theme and page name given.
//
// If theme does not exist it uses the default-theme
func (s *Site) Template(theme radio.ThemeName, page string) (*template.Template, error) {
	if s.Production {
		return s.prodTemplate(theme, page)
	}
	return s.devTemplate(theme, page)
}

// devTemplate is the Template implementation used during development such that
// all files are reread and reparsed on every invocation.
func (s *Site) devTemplate(theme radio.ThemeName, page string) (*template.Template, error) {
	const op errors.Op = "templates/Site.devTemplate"

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
func (s *Site) prodTemplate(theme radio.ThemeName, page string) (*template.Template, error) {
	const op errors.Op = "templates/Site.prodTemplate"

	// resolve theme name so that it's either an existing theme or default
	theme = s.ResolveThemeName(theme)
	// merge theme and page into a key we can use for our cache map
	key := cacheKey(string(theme) + "/" + page)

	if tmpl, ok := s.cache.Load(key); ok {
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

	s.cache.Store(key, tmpl)
	return tmpl, nil
}

func (s *Site) Theme(name radio.ThemeName) ThemeBundle {
	themes := s.themes.Load()

	if ps, ok := themes[name]; ok {
		return ps
	}
	return themes[DEFAULT_DIR]
}

func (s *Site) ResolveThemeName(name radio.ThemeName) radio.ThemeName {
	themes := s.themes.Load()

	if _, ok := themes[name]; ok {
		return name
	}
	return DEFAULT_DIR
}

func FromDirectory(dir string, state *StatefulFuncs) (*Site, error) {
	const op errors.Op = "templates/FromDirectory"

	fsys := os.DirFS(dir)
	s, err := FromFS(fsys, state)
	if err != nil {
		return nil, errors.E(op, err, dir)
	}
	return s, nil
}

func FromFS(fsys fs.FS, state *StatefulFuncs) (*Site, error) {
	const op errors.Op = "templates/FromFS"

	fnMap := maps.Clone(defaultFunctions)
	if state != nil {
		maps.Copy(fnMap, state.FuncMap())
	}

	var err error
	tmpl := Site{
		fs:     fsys,
		fnMap:  fnMap,
		themes: new(util.TypedValue[Themes]),
		cache:  new(util.Map[cacheKey, *template.Template]),
	}

	if err = tmpl.load(); err != nil {
		return nil, errors.E(op, err)
	}

	return &tmpl, nil
}

// TemplateBundle contains all the filenames required to construct a template instance
// for the page
type TemplateBundle struct {
	// fs to load the relative-filenames below
	fs    fs.FS
	fnMap template.FuncMap
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

	tmpl, err := createRoot(tb.fnMap).ParseFS(tb.fs, tb.Files()...)
	if err != nil {
		return nil, errors.E(op, errors.TemplateParseError, err)
	}
	return tmpl, nil
}

// createRoot creates a root template that adds global utility functions to
// all other template files.
func createRoot(fnMap template.FuncMap) *template.Template {
	return template.New("root").Funcs(fnMap)
}

// Themes is a map of ThemeName to ThemeBundle
type Themes map[radio.ThemeName]ThemeBundle

// ThemeBundle contains the pages that construct a specific theme as a set of TemplateBundle's
type ThemeBundle struct {
	name   radio.ThemeName
	pages  map[string]*TemplateBundle
	assets fs.FS
}

// Page returns the TemplateBundle associated with the page name given
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
	fs    fs.FS
	fnMap template.FuncMap

	baseTemplates []string
	defaults      loadStateDefault
}

type loadStateDefault struct {
	partials []string
	forms    []string
	bundle   map[string]*TemplateBundle
}

func LoadThemes(fsys fs.FS, fnMap template.FuncMap) (Themes, error) {
	const op errors.Op = "templates/LoadThemes"

	var state loadState
	var err error

	state.fs = fsys
	state.fnMap = fnMap

	// first, we're looking for .tmpl files in the main template directory
	// these are included in all other templates as a base
	state.baseTemplates, err = readDirFilterString(fsys, ".", isTemplate)
	if err != nil {
		return nil, errors.E(op, err)
	}

	// then we're going to look for directories that don't start with a dot
	subdirs, err := readDirFilterString(fsys, ".", func(de fs.DirEntry) bool {
		return !strings.HasPrefix(de.Name(), ".") && de.IsDir()
	})
	if err != nil {
		return nil, errors.E(op, err)
	}

	// now each directory will be a separate theme in the final result, but we
	// have 'public' themes and 'admin' themes so split those apart
	var publicDirs, adminDirs []string
	for _, dir := range subdirs {
		if IsAdminTheme(radio.ThemeName(dir)) {
			adminDirs = append(adminDirs, dir)
		} else {
			publicDirs = append(publicDirs, dir)
		}
	}

	// now setup the themes we're going to end up returning later
	var themes = make(Themes)

	// fill it with the public themes
	err = state.loadThemes(themes, DEFAULT_DIR, publicDirs)
	if err != nil {
		return nil, errors.E(op, err)
	}
	// and the admin themes
	err = state.loadThemes(themes, DEFAULT_ADMIN_DIR, adminDirs)
	if err != nil {
		return nil, errors.E(op, err)
	}

	return themes, nil
}

// noExt removes the extension of s as returned by filepath.Ext
func noExt(s string) string {
	return strings.TrimSuffix(filepath.Base(s), filepath.Ext(s))
}

func (ls *loadState) loadThemes(themes Themes, defaultDir string, dirs []string) error {
	const op errors.Op = "templates/loadState.loadThemes"
	var defaults loadStateDefault
	var err error

	// load the default theme
	defaults.bundle, err = ls.loadSubDir(defaultDir)
	if errors.IsE(err, os.ErrNotExist) {
		return errors.E(op, err, errors.Info("default theme does not exist"))
	}
	if err != nil {
		return errors.E(op, err)
	}
	// grab the partials and forms for quicker access
	for _, v := range defaults.bundle {
		defaults.forms = slices.Concat(v.defaultForms, v.forms)
		defaults.partials = slices.Concat(v.defaultPartials, v.partials)
		break
	}
	// set the default in the loadState so it can be used by the other themes
	ls.defaults = defaults

	// and we need the assets directory for the construction of the
	// ThemeBundle
	assetsFs, err := fs.Sub(ls.fs, path.Join(defaultDir, ASSETS_DIR))
	if err != nil && !errors.IsE(err, os.ErrNotExist) {
		return errors.E(op, err)
	}

	// construct the bundle for the default
	themes[radio.ThemeName(defaultDir)] = ThemeBundle{radio.ThemeName(defaultDir), defaults.bundle, assetsFs}

	// and now we have to do it for all the leftover directories
	for _, dir := range dirs {
		if dir == defaultDir {
			// skip the default, since we already loaded it above
			continue
		}

		bundle, err := ls.loadSubDir(dir)
		if err != nil {
			return errors.E(op, err)
		}

		assetsFs, err := fs.Sub(ls.fs, path.Join(dir, ASSETS_DIR))
		if err != nil && !errors.IsE(err, os.ErrNotExist) {
			return errors.E(op, err)
		}

		themes[radio.ThemeName(dir)] = ThemeBundle{radio.ThemeName(dir), bundle, assetsFs}
	}
	return nil
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
		fnMap:           ls.fnMap,
		base:            ls.baseTemplates,
		defaultPartials: ls.defaults.partials,
		defaultForms:    ls.defaults.forms,
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

		defaultPage := ls.defaults.bundle[noExt(name)]
		if defaultPage != nil {
			pageBundle.defaultPage = defaultPage.page
		}
		pageBundle.page = path.Join(dir, name)

		bundles[noExt(name)] = &pageBundle
	}

	// if there are no defaults to handle, we're done
	if ls.defaults.bundle == nil {
		return bundles, nil
	}

	// otherwise check for missing pages, these are pages defined
	// in the default theme but not in this current theme. Copy over
	// the default pages if they're missing.
	for name, page := range ls.defaults.bundle {
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

		tmpl, err := createRoot(defaultFunctions).New(noop).Parse(contents)
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
