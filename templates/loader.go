package templates

import (
	"fmt"
	"html/template"
	"io/ioutil"
	"os"
	"path/filepath"
)

type theme struct {
	partials []string
	pages    []string
}

// LoadTemplates loads the directory specified as several templates
//
// Load Order:
//		dir/*.tmpl
//		dir/default/partials/*.tmpl
//		dir/<theme>/partials/*.tmpl
//		dir/default/<page>.tmpl
//		dir/<theme>/<page>.tmpl
func LoadTemplates(dir string) (map[string]string, error) {
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

	// third: grab the default theme
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
	dummy := template.Must(template.New("dummy-invocation").Parse(`{{ template "base" . }}`))
	// fourth: matchup our base, defaults and theme templates
	//
	// each template will contain [base, default-partials, theme-partials] and then
	// the default page template if available and the actual page template from
	completed := make(map[string]map[string]*template.Template)
	for name, theme := range themes {
		var bundle []string
		bundle = append(bundle, baseFiles...)
		bundle = append(bundle, defaultTheme.partials...)
		bundle = append(bundle, theme.partials...)
		// now go through each page
		for _, page := range theme.pages {
			d, ok := defaultPages[filepath.Base(page)]
			if ok {
				bundle = append(bundle, d)
			}
			bundle = append(bundle, page)

			pageTmpl := template.Must(dummy.Clone())
			pageTmpl, err := pageTmpl.ParseFiles(bundle...)
			if err != nil {
				return nil, err
			}

			if completed[name] == nil {
				completed[name] = make(map[string]*template.Template)
			}

			completed[name][filepath.Base(page)] = pageTmpl
		}
	}

	err = completed["default"]["search.tmpl"].ExecuteTemplate(os.Stdout, "base", nil)
	err = completed["default"]["search.tmpl"].Execute(os.Stdout, nil)
	fmt.Println(err)
	fmt.Println(completed)
	for theme, m := range completed {
		for page, tmpl := range m {
			var names []string
			for _, t := range tmpl.Templates() {
				names = append(names, t.Name())
			}
			fmt.Printf("(%s) %s: %s\n", theme, page, names)
		}
	}
	return nil, nil
}
