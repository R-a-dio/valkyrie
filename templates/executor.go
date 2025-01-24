package templates

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"slices"

	radio "github.com/R-a-dio/valkyrie"
	"github.com/R-a-dio/valkyrie/errors"
	"github.com/R-a-dio/valkyrie/util"
	"github.com/R-a-dio/valkyrie/util/pool"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
)

var bufferPool = pool.NewResetPool(func() *bytes.Buffer {
	// use NewBuffer with an allocated zero length slice such that
	// a template with no output using .Bytes doesn't see a literal nil
	// value, this matters because our SSE package omits data entries if
	// a literal nil is passed to it, but not if it's a zero-length slice
	return bytes.NewBuffer([]byte{})
})

type TemplateSelectable interface {
	TemplateBundle() string
	TemplateName() string
}

type Executor interface {
	Execute(w io.Writer, r *http.Request, input TemplateSelectable) error
	ExecuteTemplate(ctx context.Context, theme radio.ThemeName, page, template string, output io.Writer, input any) error
	ExecuteAll(input TemplateSelectable) (map[radio.ThemeName][]byte, error)
	ExecuteAllAdmin(input TemplateSelectable) (map[radio.ThemeName][]byte, error)
}

type executor struct {
	site *Site
}

func newExecutor(site *Site) Executor {
	return &executor{
		site: site,
	}
}

func (e *executor) Execute(w io.Writer, r *http.Request, input TemplateSelectable) error {
	var ctx = r.Context()
	theme := GetTheme(ctx)

	// switch to a partial-page if we're asking for a full-page and it's htmx
	templateName := input.TemplateName()
	if util.IsHTMX(r) && templateName == "full-page" {
		templateName = "partial-page"
	}

	return e.ExecuteTemplate(ctx, theme, input.TemplateBundle(), templateName, w, input)
}

// ExecuteTemplate selects a theme, page and template and feeds it the input given and writing the template output
// to the output writer. Output is buffered until template execution is done before writing to output.
func (e *executor) ExecuteTemplate(ctx context.Context, theme radio.ThemeName, page string, template string, output io.Writer, input any) error {
	const op errors.Op = "templates/Executor.ExecuteTemplate"

	// tracing support
	ctx, span := otel.Tracer("templates").Start(ctx, "template")
	if span.IsRecording() {
		span.SetAttributes(
			attribute.String("theme", string(theme)),
			attribute.String("page", page),
			attribute.String("template", template),
		)
	}
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

// ExecuteAll executes the template selected in all public themes
func (e *executor) ExecuteAll(input TemplateSelectable) (map[radio.ThemeName][]byte, error) {
	const op errors.Op = "templates/Executor.ExecuteAll"

	res, err := e.executeAll(input, e.site.ThemeNames())
	if err != nil {
		return nil, errors.E(op, err)
	}
	return res, nil
}

func (e *executor) executeAll(input TemplateSelectable, themes []radio.ThemeName) (map[radio.ThemeName][]byte, error) {
	const op errors.Op = "templates/Executor.executeAll"

	var out = make(map[radio.ThemeName][]byte)

	b := bufferPool.Get()
	defer bufferPool.Put(b)

	for _, theme := range themes {
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

// ExecuteAllAdmin executes the template selected in all admin themes
func (e *executor) ExecuteAllAdmin(input TemplateSelectable) (map[radio.ThemeName][]byte, error) {
	const op errors.Op = "templates/Executor.ExecuteAllAdmin"

	res, err := e.executeAll(input, e.site.ThemeNamesAdmin())
	if err != nil {
		return nil, errors.E(op, err)
	}
	return res, nil
}
