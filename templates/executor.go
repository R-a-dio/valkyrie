package templates

import (
	"bytes"
	"io"
	"sync"

	"github.com/R-a-dio/valkyrie/errors"
)

type Executor struct {
	site *Site
	pool *Pool[*bytes.Buffer]
}

func NewExecutor(site *Site) *Executor {
	return &Executor{
		site: site,
		pool: NewPool(func() *bytes.Buffer { return new(bytes.Buffer) }),
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

type Resetable interface {
	Reset()
}

// Pool is a sync.Pool wrapped with a generic Resetable interface, the pool calls
// Reset before returning an item to the pool.
type Pool[T Resetable] struct {
	p sync.Pool
}

func NewPool[T Resetable](newFn func() T) *Pool[T] {
	return &Pool[T]{
		sync.Pool{
			New: func() interface{} { return newFn() },
		},
	}
}

func (p *Pool[T]) Get() T {
	return p.p.Get().(T)
}

func (p *Pool[T]) Put(v T) {
	v.Reset()
	p.p.Put(v)
}
