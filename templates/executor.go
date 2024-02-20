package templates

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"slices"

	"github.com/R-a-dio/valkyrie/errors"
	"github.com/R-a-dio/valkyrie/util"
	"github.com/R-a-dio/valkyrie/util/pool"
	"go.opentelemetry.io/otel"
)

var bufferPool = pool.NewResetPool(func() *bytes.Buffer { return new(bytes.Buffer) })

type TemplateSelectable interface {
	TemplateBundle() string
	TemplateName() string
}

type Executor interface {
	With(context.Context) Executor
	Execute(w io.Writer, r *http.Request, input TemplateSelectable) error
	ExecuteTemplate(theme, page, template string, output io.Writer, input any) error
	ExecuteAll(input TemplateSelectable) (map[string][]byte, error)
}

type executor struct {
	site *Site
	ctx  context.Context
}

func NewExecutor(site *Site) Executor {
	return &executor{
		site: site,
		ctx:  context.Background(),
	}
}

func (e executor) With(ctx context.Context) Executor {
	e.ctx = ctx
	return &e
}

func (e *executor) Execute(w io.Writer, r *http.Request, input TemplateSelectable) error {
	var ctx = r.Context()
	theme := GetTheme(ctx)

	// switch to a partial-page if we're asking for a full-page and it's htmx
	templateName := input.TemplateName()
	if util.IsHTMX(r) && templateName == "full-page" {
		templateName = "partial-page"
	}

	return e.With(ctx).ExecuteTemplate(theme, input.TemplateBundle(), templateName, w, input)
}

func (e *executor) ExecuteFull(theme, page string, output io.Writer, input any) error {
	const op errors.Op = "templates/Executor.ExecuteFull"

	err := e.ExecuteTemplate(theme, page, "full-page", output, input)
	if err != nil {
		return errors.E(op, err)
	}
	return nil
}

func (e *executor) ExecutePartial(theme, page string, output io.Writer, input any) error {
	const op errors.Op = "templates/Executor.ExecutePartial"

	err := e.ExecuteTemplate(theme, page, "partial-page", output, input)
	if err != nil {
		return errors.E(op, err)
	}
	return nil
}

// ExecuteTemplate selects a theme, page and template and feeds it the input given and writing the template output
// to the output writer. Output is buffered until template execution is done before writing to output.
func (e *executor) ExecuteTemplate(theme, page string, template string, output io.Writer, input any) error {
	const op errors.Op = "templates/Executor.ExecuteTemplate"

	// tracing support
	ctx, span := otel.Tracer("templates").Start(e.ctx, "template")
	defer span.End()

	_, span = otel.Tracer("templates").Start(ctx, "template_load")
	tmpl, err := e.site.Template(theme, page)
	span.End()
	if err != nil {
		return errors.E(op, err)
	}

	b := bufferPool.Get()
	defer bufferPool.Put(b)

	_, span = otel.Tracer("templates").Start(ctx, "template_execute")
	err = tmpl.ExecuteTemplate(b, template, input)
	span.End()
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
func (e *executor) ExecuteAll(input TemplateSelectable) (map[string][]byte, error) {
	const op errors.Op = "templates/Executor.ExecuteAll"

	var out = make(map[string][]byte)

	b := bufferPool.Get()
	defer bufferPool.Put(b)

	for _, theme := range e.site.ThemeNames() {
		tmpl, err := e.site.Template(theme, input.TemplateBundle())
		if err != nil {
			return nil, errors.E(op, err)
		}

		err = tmpl.ExecuteTemplate(b, input.TemplateName(), input)
		if err != nil {
			return nil, errors.E(op, err)
		}

		out[theme] = slices.Clone(b.Bytes())
		b.Reset()
	}
	return out, nil
}
