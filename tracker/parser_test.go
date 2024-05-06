package tracker

import (
	"bytes"
	"html/template"
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
		"Start": gen.Time(),
	}))
	p := gopter.NewProperties(nil)

	var buf bytes.Buffer

	p.Property("roundtrip", a.ForAll(func(in []radio.Listener) bool {
		buf.Reset()
		err := icecastListClientTmpl.ExecuteTemplate(&buf, "listclients", in)
		require.NoError(t, err)

		out, err := ParseListClients(&buf)
		require.NoError(t, err)
		if len(in) != len(out) {
			return false
		}

		var ok = true
		for i := range in {
			ok = ok && assert.EqualExportedValues(t, in[i], out[i])
		}
		return ok
	}))

	p.TestingRun(t)
}

var icecastListClientTmpl = template.Must(
	template.New("listclients").
		Funcs(template.FuncMap{
			"Since": func(t time.Time) uint64 {
				return uint64(time.Since(t) / time.Second)
			},
		}).
		Parse(`
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE html PUBLIC "-//W3C//DTD XHTML 1.0 Strict//EN" "http://www.w3.org/TR/xhtml1/DTD/xhtml1-strict.dtd">
<html xmlns="http://www.w3.org/1999/xhtml">
  <head>
    <meta http-equiv="Content-Type" content="text/html; charset=UTF-8" />
    <title>Icecast Streaming Media Server</title>
    <link rel="stylesheet" type="text/css" href="/style.css" />
    <meta name="viewport" content="width=device-width, initial-scale=1.0, user-scalable=yes" />
  </head>
  <body>
    <h1>Icecast2 Admin</h1>
    <div id="menu">
      <ul>
        <li>
          <a href="stats.xsl">Admin Home</a>
        </li>
        <li>
          <a href="listmounts.xsl">Mountpoint List</a>
        </li>
        <li>
          <a href="/status.xsl">Public Home</a>
        </li>
      </ul>
    </div>
    <h2>Listener Stats</h2>
    <div class="roundbox">
      <div class="mounthead">
        <h3>Mountpoint /main.mp3</h3>
        <div class="right">
          <ul class="mountlist">
            <li>
              <a class="play" href="/main.mp3.m3u">M3U</a>
            </li>
            <li>
              <a class="play" href="/main.mp3.xspf">XSPF</a>
            </li>
            <li>
              <a class="play" href="/main.mp3.vclt">VCLT</a>
            </li>
          </ul>
        </div>
      </div>
      <div class="mountcont">
        <ul class="nav">
          <li class="active">
            <a href="listclients.xsl?mount=/main.mp3">List Clients</a>
          </li>
          <li>
            <a href="moveclients.xsl?mount=/main.mp3">Move Listeners</a>
          </li>
          <li>
            <a href="updatemetadata.xsl?mount=/main.mp3">Update Metadata</a>
          </li>
          <li>
            <a href="killsource.xsl?mount=/main.mp3">Kill Source</a>
          </li>
        </ul>
        <div class="scrolltable">
          <table class="colortable">
            <thead>
              <tr>
                <td>IP</td>
                <td>Sec. connected</td>
                <td>User Agent</td>
                <td>Action</td>
              </tr>
            </thead>
            <tbody>
			  {{range .}}<tr>
                <td>{{.IP}}</td>
                <td>{{Since .Start}}</td>
                <td>{{.UserAgent}}</td>
                <td>
                  <a href="killclient.xsl?mount=/main.mp3&amp;id={{.ID}}">Kick</a>
                </td>
              </tr>{{end}}
            </tbody>
          </table>
        </div>
      </div>
    </div>
    <div id="footer">
		Support icecast development at <a href="https://www.icecast.org/">www.icecast.org</a></div>
  </body>
</html>
`))
