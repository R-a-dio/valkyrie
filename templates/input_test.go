package templates_test

import (
	"io"
	"net/http/httptest"
	"reflect"
	"testing"

	"github.com/R-a-dio/valkyrie/templates"
	"github.com/R-a-dio/valkyrie/website/admin"
	v1 "github.com/R-a-dio/valkyrie/website/api/v1"
	"github.com/R-a-dio/valkyrie/website/middleware"
	"github.com/R-a-dio/valkyrie/website/public"
	"github.com/leanovate/gopter"
	"github.com/leanovate/gopter/arbitrary"
	"github.com/leanovate/gopter/prop"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var templateInputs = []templates.TemplateSelectable{
	public.HomeInput{},
	public.ChatInput{},
	public.LastPlayedInput{},
	public.QueueInput{},
	public.NewsInput{},
	public.SubmitInput{},
	public.SubmissionForm{},
	public.FavesInput{},
	public.ScheduleInput{},
	public.StaffInput{},
	public.SearchInput{},
	middleware.LoginInput{},
	v1.NowPlaying{},
	v1.LastPlayed{},
	v1.Queue{},
	v1.SearchInput{},
	admin.HomeInput{},
	admin.PendingInput{},
	admin.PendingForm{},
}

func TestTemplateInputs(t *testing.T) {
	t.Skip("skipping until fixed")
	if testing.Short() {
		t.Skip("skipping test in short mode.")
	}

	tmpl, err := templates.FromDirectory(".")
	require.NoError(t, err)
	tmpl.Production = true

	exec := tmpl.Executor()
	req := httptest.NewRequest("GET", "/", nil)

	a := arbitrary.DefaultArbitraries()
	param := gopter.DefaultTestParameters()
	param.MinSuccessfulTests = 100
	p := gopter.NewProperties(param)

	for _, theme := range tmpl.ThemeNames() {
		req = req.WithContext(templates.SetTheme(req.Context(), theme))
		for _, in := range templateInputs {
			rtyp := reflect.TypeOf(in)
			p.Property(theme+"-"+rtyp.Name(), prop.ForAll(
				func(a any) bool {
					// some types used for TemplateSelectable are just aliases/simple renames
					// and those lose their "proper" type when gopter generates them so we
					// need to convert it back into the type we expect
					v := reflect.ValueOf(a).Convert(rtyp)
					input := v.Interface().(templates.TemplateSelectable)
					return assert.NoError(t, exec.Execute(io.Discard, req, input))
				},
				a.GenForType(rtyp),
			))
		}
	}
	p.TestingRun(t)
}
