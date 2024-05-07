package tracker

import (
	"bytes"
	"html/template"
	"math"
	"reflect"
	"strings"
	"testing"
	"time"
	"unicode"

	radio "github.com/R-a-dio/valkyrie"
	"github.com/leanovate/gopter"
	"github.com/leanovate/gopter/arbitrary"
	"github.com/leanovate/gopter/gen"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseListClients(t *testing.T) {
	a := arbitrary.DefaultArbitraries()
	a.RegisterGen(gen.Struct(reflect.TypeFor[radio.Listener](), map[string]gopter.Gen{
		"ID": a.GenForType(reflect.TypeFor[radio.ListenerClientID]()),
		"UserAgent": gen.AnyString().Map(func(in string) string {
			return strings.TrimSpace(strings.Map(func(r rune) rune {
				if unicode.IsGraphic(r) {
					return r
				}
				return -1
			}, in))
		}),
		"IP":    gen.NumString(),
		"Start": gen.TimeRange(time.Now(), time.Nanosecond*(math.MaxUint64/2)),
	}))
	p := gopter.NewProperties(nil)

	var buf bytes.Buffer

	p.Property("roundtrip", a.ForAll(func(in []radio.Listener) bool {
		buf.Reset()
		err := icecastListClientXMLTmpl.ExecuteTemplate(&buf, "listclients", in)
		require.NoError(t, err)

		out, err := ParseListClientsXML(&buf)
		require.NoError(t, err)
		if len(in) != len(out) {
			return false
		}

		var ok = true
		for i := range in {
			ok = ok && assert.Equal(t, in[i].ID, out[i].ID)
			ok = ok && assert.Equal(t, in[i].UserAgent, out[i].UserAgent)
			ok = ok && assert.Equal(t, in[i].IP, out[i].IP)
			ok = ok && assert.WithinDuration(t, in[i].Start, out[i].Start, time.Minute)
		}
		return ok
	}))

	p.TestingRun(t)
}

var icecastListClientXMLTmpl = template.Must(
	template.New("listclients").
		Funcs(template.FuncMap{
			"Since": func(t time.Time) uint64 {
				return uint64(time.Since(t) / time.Second)
			},
		}).
		Parse(`<?xml version="1.0"?>
		<icestats><source mount="/main.mp3"><Listeners>{{len .}}</Listeners>{{range .}}<listener><IP>{{.IP}}</IP><UserAgent>{{.UserAgent}}</UserAgent><Connected>{{Since .Start}}</Connected><ID>{{.ID}}</ID></listener>{{end}}</source></icestats>
`))
