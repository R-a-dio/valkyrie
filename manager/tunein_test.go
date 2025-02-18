package manager

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	radio "github.com/R-a-dio/valkyrie"
	"github.com/R-a-dio/valkyrie/config"
	"github.com/R-a-dio/valkyrie/mocks"
	"github.com/R-a-dio/valkyrie/util/eventstream"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTuneinUpdater(t *testing.T) {
	ctx := context.Background()
	ctx = zerolog.New(zerolog.NewTestWriter(t)).WithContext(ctx)

	// sleep a little bit at the very end since the goroutine launched inside of TuneinUpdater
	// might still be running for a little bit after we cancel the context
	defer time.Sleep(time.Second)

	es := eventstream.NewEventStream((*radio.SongUpdate)(nil))
	defer es.Shutdown()
	m := &mocks.ManagerServiceMock{
		CurrentSongFunc: func(ctx context.Context) (eventstream.Stream[*radio.SongUpdate], error) {
			return es.SubStream(ctx), nil
		},
	}

	cfg := config.TestConfig()
	c := cfg.Conf()
	c.Tunein.Key = "a key"
	c.Tunein.StationID = "s321"
	c.Tunein.PartnerID = "112333"

	done := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Log("received request")
		assert.Equal(t, c.Tunein.PartnerID, r.FormValue(TUNEIN_PARTNER_ID))
		assert.Equal(t, c.Tunein.StationID, r.FormValue(TUNEIN_STATION_ID))
		assert.Equal(t, c.Tunein.Key, r.FormValue(TUNEIN_PARTNER_KEY))
		assert.Equal(t, "fancy artist", r.FormValue("artist"))
		assert.Equal(t, "a song title", r.FormValue("title"))

		w.WriteHeader(http.StatusOK)

		close(done)
	}))
	defer server.Close()

	c.Tunein.Endpoint = server.URL
	cfg.StoreConf(c)

	t.Log(c.Tunein)
	tu, err := NewTuneinUpdater(ctx, cfg, m, server.Client())
	require.NoError(t, err)

	defer tu.Close()

	es.Send(&radio.SongUpdate{
		Song: radio.NewSong("fancy artist - a song title"),
		Info: radio.SongInfo{
			Start: time.Now(),
			End:   time.Now(),
		},
	})

	<-done
	t.Log("done")
}
