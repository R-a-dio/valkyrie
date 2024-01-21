package templates

import (
	"bytes"
	"io"

	"github.com/R-a-dio/valkyrie/errors"
	"github.com/R-a-dio/valkyrie/util/pool"
)

type Executor struct {
	site *Site
	pool *pool.ResetPool[*bytes.Buffer]
}

func NewExecutor(site *Site) *Executor {
	return &Executor{
		site: site,
		pool: pool.NewResetPool(func() *bytes.Buffer { return new(bytes.Buffer) }),
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

func (e *Executor) ExecuteTemplate(theme, page string, template string, output io.Writer, input any) error {
	const op errors.Op = "templates/Executor.ExecuteTemplate"

	tmpl, err := e.site.Template(theme, page)
	if err != nil {
		return errors.E(op, err)
	}

	b := e.pool.Get()
	defer e.pool.Put(b)

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
