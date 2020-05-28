package templates

import (
	"fmt"
	"html/template"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
	"text/tabwriter"
)

type Templates map[string]map[string]Template

type theme struct {
	partials []string
	pages    []string
}

type filecache map[string]string

func (cache *filecache) readFile(filename string) (string, error) {
	if *cache == nil {
		*cache = make(map[string]string)
	}
	content, ok := (*cache)[filename]
	if ok {
		return content, nil
	}

	b, err := ioutil.ReadFile(filename)
	if err != nil {
		return "", err
	}
	content = string(b)
	(*cache)[filename] = content
	return content, nil
}

// LoadTemplates loads the directory specified as several templates
//
// Load Order:
//		dir/*.tmpl
//		dir/default/partials/*.tmpl
//		dir/<theme>/partials/*.tmpl
//		dir/default/<page>.tmpl
//		dir/<theme>/<page>.tmpl
func LoadTemplates(dir string) (Templates, error) {
	// top-level of the directory we're getting should be filled with directories
	// named after themes. We special case "default" here to mean the fallback
	// option if a theme doesn't replace something
	all, err := ioutil.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	// filter out directories
	themeNames := make([]string, 0, len(all))
	for _, fi := range all {
		if fi.IsDir() {
			themeNames = append(themeNames, fi.Name())
		}
	}

	// first we load all .tmpl files at the top level, this is our base
	baseFiles, err := filepath.Glob(filepath.Join(dir, "*.tmpl"))
	if err != nil {
		return nil, err
	}

	// second collect all partials and pages for the themes
	themes := make(map[string]theme, len(themeNames))
	for _, name := range themeNames {
		partials, err := filepath.Glob(filepath.Join(dir, name, "partials", "*.tmpl"))
		if err != nil {
			return nil, err
		}

		pages, err := filepath.Glob(filepath.Join(dir, name, "*.tmpl"))
		if err != nil {
			return nil, err
		}

		themes[name] = theme{partials, pages}
	}

	// setup quick access to default pages by making a map of page -> filename
	defaultTheme, ok := themes["default"]
	if !ok {
		panic("missing default theme")
	}
	// setup quick access to default pages
	defaultPages := make(map[string]string, len(defaultTheme.pages))
	for _, page := range defaultTheme.pages {
		defaultPages[filepath.Base(page)] = page
	}

	// dummy invocation template so we can use .Execute
	dummy := createDummy()
	var cache filecache
	// fourth: matchup our base, defaults and theme templates
	//
	// each template will contain [base, default-partials, theme-partials] and then
	// the default page template if available and the actual page template from
	completed := make(map[string]map[string]Template)
	for name, theme := range themes {
		for _, page := range theme.pages {
			var bundle []string
			bundle = append(bundle, baseFiles...)
			if name != "default" {
				bundle = append(bundle, defaultTheme.partials...)
			}
			bundle = append(bundle, theme.partials...)
			// finish off the bundle with the default page
			d, ok := defaultPages[filepath.Base(page)]
			if ok && name != "default" {
				bundle = append(bundle, d)
			}
			// and the themed page
			bundle = append(bundle, page)

			// now we're ready to construct the template, create a clone of our
			// dummy-invocation and then start reading the files in the bundle to
			// add to the template
			parent := template.Must(dummy.Clone())
			pageTmpl, err := loadTemplate(parent, cache, bundle)
			if err != nil {
				return nil, err
			}

			if completed[name] == nil {
				completed[name] = make(map[string]Template)
			}

			completed[name][filepath.Base(page)] = *pageTmpl
		}
	}

	return completed, nil
	for theme, m := range completed {
		for page, tmpl := range m {
			var names []string
			for _, t := range tmpl.Templates() {
				names = append(names, t.Name())
			}
			fmt.Printf("(%s) %s: %s\n", theme, page, names)
		}
	}
	completed["default"]["search.tmpl"].Definitions()
	return nil, nil
}

func createDummy() *template.Template {
	return template.Must(template.New("invocation").Parse(`{{ template "base" . }}`))
}

func loadTemplate(parent *template.Template, cache filecache, bundle []string) (*Template, error) {
	for _, filename := range bundle {
		contents, err := cache.readFile(filename)
		if err != nil {
			return nil, err
		}
		_, err = parent.Parse(contents)
		if err != nil {
			return nil, err
		}
	}

	return &Template{bundle, parent}, nil
}

// Reload the template from disk and returns the new version
func (t Template) Reload() (*Template, error) {
	dummy := createDummy()
	var cache filecache

	return loadTemplate(dummy, cache, t.Files)
}

func (t Template) ExecuteDev(w io.Writer, data interface{}) error {
	new, err := t.Reload()
	if err != nil {
		return err
	}

	return new.Execute(w, data)
}

type Template struct {
	// Files this template was constructed from, they are loaded in order,
	// starting from the start
	Files []string
	// Tmpl the actual template construct
	*template.Template
}

func (t Template) Definitions() error {
	const noop = "--noop--"
	columns := []string{"filename"}
	var cc = make(map[string]bool)
	type row struct {
		filename string
		names    map[string]bool
	}

	rows := []row{}

	// go through each file
	var cache filecache

	for _, filename := range t.Files {
		contents, err := cache.readFile(filename)
		if err != nil {
			return err
		}

		tmpl, err := template.New(noop).Parse(contents)
		if err != nil {
			return err
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
			if !cc[name] {
				cc[name] = true
				columns = append(columns, name)
			}
		}

		rows = append(rows, r)
	}

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
