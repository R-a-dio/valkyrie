package v1

import (
	"context"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"reflect"
	"sync"
	"testing"
	"time"

	radio "github.com/R-a-dio/valkyrie"
	"github.com/R-a-dio/valkyrie/mocks"
	"github.com/R-a-dio/valkyrie/templates"
	"github.com/leanovate/gopter"
	"github.com/leanovate/gopter/arbitrary"
	"github.com/leanovate/gopter/gen"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/alevinval/sse/pkg/eventsource"
)

func TestStream(t *testing.T) {
	ctx := context.Background()
	ctx = zerolog.New(zerolog.NewTestWriter(t)).WithContext(ctx)

	exec := &mocks.ExecutorMock{
		ExecuteAllFunc: func(input templates.TemplateSelectable) (map[radio.ThemeName][]byte, error) {
			jsonData, err := json.MarshalIndent(input, "", "  ")
			if err != nil {
				return nil, err
			}

			return map[radio.ThemeName][]byte{
				"json": jsonData,
			}, nil
		},
		ExecuteFunc: func(w io.Writer, r *http.Request, input templates.TemplateSelectable) error {
			jsonData, err := json.MarshalIndent(input, "", "  ")
			if err != nil {
				return err
			}

			_, err = w.Write(jsonData)
			return err
		},
	}

	var muExpected sync.Mutex
	var expected radio.Status

	stream := NewStream(ctx, exec)
	server := httptest.NewUnstartedServer(stream)
	server.Config.BaseContext = func(l net.Listener) context.Context { return ctx }
	server.Config.ConnContext = func(ctx context.Context, c net.Conn) context.Context {
		return templates.SetTheme(ctx, "json", true)
	}
	server.Start()

	t.Log(server.URL)
	es1, err := eventsource.New(server.URL)
	require.NoError(t, err)
	defer es1.Close()

	sync := make(chan struct{})
	go func() {
		a := arbitrary.DefaultArbitraries()
		timeGen := func(gp *gopter.GenParameters) *gopter.GenResult {
			v := time.Date(2000, 10, 9, 8, 7, 6, 5, time.UTC)
			res := gen.Time()(gp)
			res.Result = v
			return res
		}
		a.RegisterGen(timeGen)
		a.RegisterGen(gen.PtrOf(timeGen))

		genStatus := a.GenForType(reflect.TypeOf(radio.Status{}))
		param := gopter.DefaultGenParameters()

		for i := 0; i < 100; i++ {
			res := genStatus(param)
			<-sync
			sendStatus := res.Result.(radio.Status)
			muExpected.Lock()
			expected = sendStatus
			muExpected.Unlock()
			stream.SendNowPlaying(sendStatus)
		}
	}()

	ch1 := es1.MessageEvents()

	jsonEvent := <-ch1

	require.Equal(t, EventTime, jsonEvent.Name)

	for i := 0; i < 100; i++ {
		sync <- struct{}{}
		jsonEvent = <-ch1
		require.Equal(t, EventMetadata, jsonEvent.Name)

		var status radio.Status

		// check the json version
		err = json.Unmarshal([]byte(jsonEvent.Data), &status)
		assert.NoError(t, err)
		muExpected.Lock()
		assert.Equal(t, expected, status)
		muExpected.Unlock()
	}
}

func TestStreamSendInputs(t *testing.T) {
	var name string

	exec := &mocks.ExecutorMock{
		ExecuteAllFunc: func(input templates.TemplateSelectable) (map[radio.ThemeName][]byte, error) {
			name = input.TemplateName()
			return nil, nil
		},
	}

	ctx := context.Background()
	stream := NewStream(ctx, exec)
	defer stream.Shutdown()

	t.Run("SendStreamer", func(t *testing.T) {
		stream.SendStreamer(radio.User{})
		assert.Equal(t, "streamer", name)
	})

	t.Run("SendNowPlaying", func(t *testing.T) {
		stream.SendNowPlaying(radio.Status{})
		assert.Equal(t, "nowplaying", name)
	})

	t.Run("SendQueue", func(t *testing.T) {
		stream.SendQueue([]radio.QueueEntry{})
		assert.Equal(t, "queue", name)
	})

	t.Run("SendLastPlayed", func(t *testing.T) {
		stream.SendLastPlayed([]radio.Song{})
		assert.Equal(t, "lastplayed", name)
	})

	t.Run("SendThread", func(t *testing.T) {
		stream.SendThread(radio.Thread(""))
		assert.Equal(t, "thread", name)
	})
}

func TestStreamSlowSub(t *testing.T) {
	exec := &mocks.ExecutorMock{
		ExecuteAllFunc: func(input templates.TemplateSelectable) (map[radio.ThemeName][]byte, error) {
			return nil, nil
		},
	}

	ctx := templates.SetTheme(context.Background(), "default", true)
	stream := NewStream(ctx, exec)

	req := httptest.NewRequest("GET", "/", nil)
	req = req.WithContext(ctx)
	w := httptest.NewRecorder()

	go func() {
		stream.ServeHTTP(w, req)
	}()
}
