package templates

import (
	"bytes"
	"io"
	"slices"

	"github.com/R-a-dio/valkyrie/errors"
	"github.com/R-a-dio/valkyrie/util/pool"
)

var bufferPool = pool.NewResetPool(func() *bytes.Buffer { return new(bytes.Buffer) })

type Executor struct {
	site *Site
}

func NewExecutor(site *Site) *Executor {
	return &Executor{
		site: site,
	}
}

func (e *Executor) ExecuteFull(theme, page string, output io.Writer, input any) error {
	const op errors.Op = "templates/Executor.ExecuteFull"

	err := e.ExecuteTemplate(theme, page, "full-page", output, input)
	if err != nil {
		return errors.E(op, err)
	}
	return nil
}

func (e *Executor) ExecutePartial(theme, page string, output io.Writer, input any) error {
	const op errors.Op = "templates/Executor.ExecutePartial"

	err := e.ExecuteTemplate(theme, page, "partial-page", output, input)
	if err != nil {
		return errors.E(op, err)
	}
	return nil
}

// ExecuteTemplate selects a theme, page and template and feeds it the input given and writing the template output
// to the output writer. Output is buffered until template execution is done before writing to output.
func (e *Executor) ExecuteTemplate(theme, page string, template string, output io.Writer, input any) error {
	const op errors.Op = "templates/Executor.ExecuteTemplate"

	tmpl, err := e.site.Template(theme, page)
	if err != nil {
		return errors.E(op, err)
	}

	b := bufferPool.Get()
	defer bufferPool.Put(b)

	err = tmpl.ExecuteTemplate(b, template, input)
	if err != nil {
		return errors.E(op, err)
	}

	_, err = io.Copy(output, b)
	if err != nil {
		return errors.E(op, err)
	}
	return nil
}

// ExecuteTemplateAll executes the template given feeding the input given for all known themes
func (e *Executor) ExecuteTemplateAll(template string, input any) (map[string][]byte, error) {
	const op errors.Op = "templates/Executor.ExecuteTemplateAll"

	var out = make(map[string][]byte)

	b := bufferPool.Get()
	defer bufferPool.Put(b)

	for _, theme := range e.site.ThemeNames() {
		tmpl, err := e.site.Template(theme, "home")
		if err != nil {
			return nil, errors.E(op, err)
		}

		err = tmpl.ExecuteTemplate(b, template, input)
		if err != nil {
			return nil, errors.E(op, err)
		}

		out[theme] = slices.Clone(b.Bytes())
		b.Reset()
	}
	return out, nil
}
