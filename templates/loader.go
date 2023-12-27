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
	"text/tabwriter"

	"github.com/R-a-dio/valkyrie/errors"
)

const (
	// the extension used for template files
	TEMPLATE_EXT = ".tmpl"
	// the directory name used for shared templates inside of a subdirectory
	PARTIAL_DIR = "partials"
	// directory name of the default templates
	DEFAULT_DIR = "default"
)

// Site is an overarching struct containing all the themes of the website.
type Site struct {
	loader Loader

	themes Themes
	cache  map[string]*template.Template
}

func (s *Site) Reload() error {
	const op errors.Op = "templates/Reload"

	themes, err := s.loader.Load()
	if err != nil {
		return errors.E(op, err)
	}
	s.themes = themes
	return nil
}

type TemplateSelector interface {
	Template(theme, page string) (*template.Template, error)
}

func (s *Site) Executor() *Executor {
	return NewExecutor(s)
}

// Template returns a Template associated with the theme and page name given.
//
// If theme does not exist it uses the default-theme
func (s *Site) Template(theme, page string) (*template.Template, error) {
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

	return nil, errors.E(op, errors.TemplateUnknown)
}

func (s *Site) Theme(name string) ThemeBundle {
	if ps, ok := s.themes[name]; ok {
		return ps
	}
	return s.themes[DEFAULT_DIR]
}

func (s *Site) ResolveThemeName(name string) string {
	if _, ok := s.themes[name]; ok {
		return name
	}
	return DEFAULT_DIR
}

func (s *Site) makeCache() error {
	const op errors.Op = "templates/makeCache"

	cache := make(map[string]*template.Template)
	for themeName, theme := range s.themes {
		for name, bundle := range theme.pages {
			key := themeName + "/" + name
			tmpl, err := bundle.Template()
			if err != nil {
				return errors.E(op, err)
			}
			cache[key] = tmpl
		}
	}

	s.cache = cache
	return nil
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
		loader: NewLoader(fsys),
		//bufferPool: NewPool(func() *bytes.Buffer { return new(bytes.Buffer) }),
		cache: make(map[string]*template.Template),
	}

	tmpl.themes, err = tmpl.loader.Load()
	if err != nil {
		return nil, errors.E(op, err)
	}

	return &tmpl, nil
}

func NewLoader(fsys fs.FS) Loader {
	return Loader{fs: fsys}
}

type Loader struct {
	// fs is the filesystem we're loading files from
	fs fs.FS

	// baseTemplates contain the files at the base of the directory hierarchy and are included
	// in all bundles
	baseTemplates []string
	// defaultTheme is a mapping of page-name to default-template-bundle for easy backfilling of
	// undefined pages in themes
	defaultTheme map[string]*TemplateBundle
	// defaultPartials contain the files in the DEFAULT_DIR/partials directory and are included
	// in all themes as partials
	defaultPartials []string
}

// TemplateBundle contains all the filenames required to construct a template instance
// for the page
type TemplateBundle struct {
	// loader used to actually load the relative-filenames below
	loader *Loader
	// the following fields contain all the filenames of the templates we're parsing
	// into a html/template.Template. They're listed in load-order, last one wins.
	base            []string
	defaultPartials []string
	partials        []string
	defaultPage     string
	page            string

	cache *template.Template
}

// Files returns all the files in this bundle sorted in load-order
func (tb *TemplateBundle) Files() []string {
	s := make([]string, 0, len(tb.base)+len(tb.defaultPartials)+len(tb.partials)+2)
	s = append(s, tb.base...)
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

	tmpl, err := createRoot().ParseFS(tb.loader.fs, tb.Files()...)
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
	name  string
	pages map[string]*TemplateBundle
}

func (tb ThemeBundle) Page(name string) (*TemplateBundle, error) {
	const op errors.Op = "templates/ThemeBundle.Page"

	tlb, ok := tb.pages[name]
	if !ok {
		return nil, errors.E(op, errors.TemplateUnknown)
	}

	return tlb, nil
}

func (l *Loader) Load() (Themes, error) {
	const op errors.Op = "templates/Loader.Load"

	bt, err := readDirFilterString(l.fs, ".", isTemplate)
	if err != nil {
		return nil, errors.E(op, err)
	}
	l.baseTemplates = bt

	// find our default directory
	defaultBundle, err := l.loadSubDir(DEFAULT_DIR)
	if err != nil {
		return nil, errors.E(op, err)
	}
	// sanity check that we have atleast 1 bundle
	if len(defaultBundle) == 0 {
		return nil, errors.E(op, "default bundle empty")
	}

	// grab the partials from the first bundle
	for _, v := range defaultBundle {
		l.defaultPartials = v.partials
		break
	}

	// plant the defaults in the loader so the other themes can use them
	l.defaultTheme = defaultBundle

	// read the rest of the directories
	subdirs, err := readDirFilterString(l.fs, ".", func(e fs.DirEntry) bool { return e.IsDir() })
	if err != nil {
		return nil, errors.E(op, err)
	}

	themes := Themes{
		DEFAULT_DIR: ThemeBundle{DEFAULT_DIR, defaultBundle},
	}
	for _, subdir := range subdirs {
		if subdir == DEFAULT_DIR { // skip the default dir since we already loaded it earlier
			continue
		}
		bundles, err := l.loadSubDir(subdir)
		if err != nil {
			return nil, errors.E(op, err)
		}

		themes[subdir] = ThemeBundle{
			name:  subdir,
			pages: bundles,
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
func (l *Loader) loadSubDir(dir string) (map[string]*TemplateBundle, error) {
	const op errors.Op = "templates/Loader.loadSubDir"

	var bundle = TemplateBundle{
		loader:          l,
		base:            l.baseTemplates,
		defaultPartials: l.defaultPartials,
	}

	// read the partials subdirectory
	partialDir := path.Join(dir, PARTIAL_DIR)

	entries, err := readDirFilter(l.fs, partialDir, isTemplate)
	if err != nil && !errors.IsE(err, fs.ErrNotExist) {
		return nil, errors.E(op, err)
	}

	var partials = make([]string, 0, len(entries))
	for _, entry := range entries {
		partials = append(partials, path.Join(partialDir, entry.Name()))
	}

	bundle.partials = partials

	// read the actual directory
	entries, err = readDirFilter(l.fs, dir, isTemplate)
	if err != nil {
		return nil, errors.E(op, err)
	}

	var bundles = make(map[string]*TemplateBundle, len(entries))
	for _, entry := range entries {
		name := entry.Name()
		// create a bundle for each page in this directory
		bundle := bundle
		defaultPage := l.defaultTheme[noExt(name)]
		if defaultPage != nil {
			bundle.defaultPage = defaultPage.page
		}
		bundle.page = path.Join(dir, name)

		bundles[noExt(name)] = &bundle
	}

	// if there are no defaults to handle, we're done
	if l.defaultTheme == nil {
		return bundles, nil
	}

	// otherwise check for missing pages, these are pages defined
	// in the default theme but not in this current theme. Copy over
	// the default pages if they're missing.
	for name, page := range l.defaultTheme {
		_, ok := bundles[name]
		if ok {
			continue
		}
		bundles[name] = page
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
