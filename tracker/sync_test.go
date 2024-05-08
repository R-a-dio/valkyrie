package tracker

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	radio "github.com/R-a-dio/valkyrie"
	"github.com/R-a-dio/valkyrie/config"
	"github.com/leanovate/gopter"
	"github.com/leanovate/gopter/arbitrary"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetIcecastListClients(t *testing.T) {
	a := arbitrary.DefaultArbitraries()
	a.RegisterGen(genListener(a))
	p := gopter.NewProperties(nil)

	var buf bytes.Buffer

	ctx := testCtx(t)
	cfg, err := config.LoadFile()
	require.NoError(t, err)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/admin/listclients", r.URL.Path)
		assert.Equal(t, cfg.Conf().Tracker.MountName, r.URL.Query().Get("mount"))
		user, passwd, ok := r.BasicAuth()
		assert.True(t, ok, "request should have auth header")
		assert.Equal(t, cfg.Conf().Tracker.MasterUsername, user)
		assert.Equal(t, cfg.Conf().Tracker.MasterPassword, passwd)
		io.Copy(w, &buf)
	}))
	defer srv.Close()

	c := cfg.Conf()
	c.Tracker.MasterServer = config.URL(srv.URL)
	cfg.StoreConf(c)

	p.Property("roundtrip", a.ForAll(func(in []radio.Listener) bool {
		buf.Reset()
		err := icecastListClientXMLTmpl.ExecuteTemplate(&buf, "listclients", in)
		require.NoError(t, err)

		out, err := GetIcecastListClients(ctx, cfg)
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
