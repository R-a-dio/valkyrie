package eventstream

import (
	"context"
	"io"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEventStreamInitialValue(t *testing.T) {
	initial := 500
	stream := NewEventStream(initial)

	s1 := stream.Sub()
	assert.Equal(t, initial, <-s1)
	stream.Leave(s1)

	second := 1000
	stream.Send(second)
	s2 := stream.Sub()
	assert.Equal(t, second, <-s2)
	stream.Leave(s2)
}

type testCase[V any] struct {
	name     string
	clients  int
	messages int
	next     func(last V) V
}

var intCases = []testCase[int]{
	{"only initial", 1, 0, func(last int) int { return last + 1 }},
	{"one sub, 100 messages", 1, 100, func(last int) int { return last + 1 }},
	{"one sub, 100000 messages", 1, 100000, func(last int) int { return last - 1 }},
	{"100 subs, 10000 messages", 100, 10000, func(last int) int { return last + 4 }},
}

func TestEventServer(t *testing.T) {
	for _, cas := range intCases {
		t.Run(cas.name, func(t *testing.T) {
			initial := 0
			es := NewEventStream[int](initial)
			wg := new(sync.WaitGroup)
			// setup clients
			client := func(ch chan int) {
				// rename some variables for easier reasoning
				should_next := initial
				have_received := 0
				should_receive := cas.messages + 1 // because of the initial value

				// start receiving
				for m := range ch {
					have_received++
					if m != should_next {
						t.Error("m != next", m, should_next)
					}

					should_next = cas.next(m)
				}

				if should_receive != have_received {
					t.Error("should_receive != have_received", should_receive, have_received)
				}

				wg.Done()
			}

			for i := 0; i < cas.clients; i++ {
				wg.Add(1)
				go client(es.Sub())
			}

			// send the messages
			var val int
			for i := 0; i < cas.messages; i++ {
				val = cas.next(val)
				es.Send(val)
			}

			es.Shutdown()
			wg.Wait()
		})
	}
}

func TestEventServerStream(t *testing.T) {
	for _, cas := range intCases {
		t.Run(cas.name, func(t *testing.T) {
			initial := 0
			es := NewEventStream[int](initial)
			wg := new(sync.WaitGroup)
			// setup clients
			client := func(stream Stream[int]) {
				// rename some variables for easier reasoning
				should_next := initial
				have_received := 0
				should_receive := cas.messages + 1 // because of the initial value

				// start receiving
				for {
					m, err := stream.Next()
					if err != nil {
						break
					}

					have_received++
					if m != should_next {
						t.Error("m != next", m, should_next)
					}

					should_next = cas.next(m)

				}

				if should_receive != have_received {
					t.Error("should_receive != have_received", should_receive, have_received)
				}

				wg.Done()
			}

			for i := 0; i < cas.clients; i++ {
				wg.Add(1)
				go client(es.SubStream(context.Background()))
			}

			// send the messages
			var val int
			for i := 0; i < cas.messages; i++ {
				val = cas.next(val)
				es.Send(val)
			}

			es.Shutdown()
			wg.Wait()
		})
	}
}

func TestEventServerStreamContextCancel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())

	es := NewEventStream("hello world")

	s := es.SubStream(ctx)

	initial, err := s.Next()
	require.NoError(t, err)
	require.Equal(t, "hello world", initial)

	// now cancel the context we passed to SubStream
	cancel()

	next, err := s.Next()           // this should not block because the context got canceled
	require.ErrorIs(t, err, io.EOF) // should be EOF
	require.Zero(t, next)
}

func TestEventServerLeave(t *testing.T) {
	es := NewEventStream[string]("hello world")

	ch := es.Sub()
	if es.length() != 1 {
		t.Fatal("failed to subscribe")
	}
	es.Leave(ch)
	es.Send("secondary")
	if es.length() != 0 {
		t.Fatal("failed to leave")
	}
}

func TestEventServerLeaveStream(t *testing.T) {
	es := NewEventStream[string]("hello world")

	s := es.SubStream(context.Background())
	if es.length() != 1 {
		t.Fatal("failed to subscribe")
	}
	_ = s.Close()

	es.Send("secondary")
	if es.length() != 0 {
		t.Fatal("failed to leave")
	}
}

func TestEventServerShutdown(t *testing.T) {
	es := NewEventStream[string]("primary")
	es.Shutdown()

	ch := es.Sub()
	if es.length() != 0 {
		t.Fatal("added a subscriber after shutdown")
	}

	es.Leave(ch)

	// sending after Shutdown should not block
	es.Send("after shutdown")
	es.CompareAndSend("compare after shutdown", func(new, old string) bool {
		return new == old
	})
}

func TestEventServerCloseSubs(t *testing.T) {
	es := NewEventStream[string]("test")
	beforeCh := es.Sub()

	es.CloseSubs()

	afterCh := es.Sub()
	if es.length() != 0 {
		t.Fatal("added a subscriber after close")
	}

	select {
	case <-beforeCh:
	case <-time.After(time.Second):
		t.Fatal("channel returned by Sub should be closed after CloseSubs")
	}

	select {
	case <-afterCh:
	case <-time.After(time.Second):
		t.Fatal("channel returned by Sub after Close should be closed")
	}

	// we called close, but not shutdown so Send's should still be being processed
	es.Send("welcome")

	assert.Eventually(t, func() bool {
		return "welcome" == es.Latest()
	}, time.Second, time.Millisecond*50)
}

func TestEventServerShutdownMulti(t *testing.T) {
	es := NewEventStream("test")

	for range 50 {
		// call Shutdown multiple times to make sure it can't break somehow
		es.Shutdown()
	}
}

func TestEventServerCloseSubsMulti(t *testing.T) {
	es := NewEventStream("test")

	for range 50 {
		// call CloseSubs multiple times to hit the extra case where we
		// take the double CLOSE request path
		es.CloseSubs()
	}
}

func TestEventServerCloseSubsMultiSub(t *testing.T) {
	es := NewEventStream("test")

	es.CloseSubs()

	for range 50 {
		// call Sub multiple times to hit the extra case where we
		// take the SUBSCRIBE request path
		es.Sub()
	}
}

func TestEventServerSlowSub(t *testing.T) {
	es := NewEventStream(0)
	ch := es.Sub()

	time.Sleep(TIMEOUT * 2) // also wait a bit for the initial ticker to tick
	for i := range 50 {
		es.Send(i)
	}

	t.Log(<-ch)
}

func TestEventServerCompareAndSend(t *testing.T) {
	v := int(50)

	es := NewEventStream(v)
	ch := es.Sub()

	// initial value
	assert.Equal(t, v, <-ch)

	// send a normal update
	v = int(100)
	es.Send(v)
	assert.Equal(t, v, <-ch)

	// send a compare update
	es.CompareAndSend(int(500), func(new, old int) bool {
		assert.Equal(t, int(500), new)
		return new == old
	})
	// the above shouldn't update, neither should this below
	es.CompareAndSend(v, nil)

	// but this one below should
	prev, v := v, int(1000)
	es.CompareAndSend(v, func(new, old int) bool {
		return old == prev
	})
	assert.Equal(t, v, <-ch)

}

func benchmarkEventStream(subcount int, b *testing.B) {
	es := NewEventStream("hello world")

	var channels []chan string
	for i := 0; i < subcount; i++ {
		ch := es.Sub()
		channels = append(channels, ch)
		go func(ch chan string) {
			for range ch {
			}
		}(ch)
	}

	thing := "some garbage"
	for n := 0; n < b.N; n++ {
		es.Send(thing)
	}

	es.Shutdown()
}

func BenchmarkEventStream1(b *testing.B)     { benchmarkEventStream(1, b) }
func BenchmarkEventStream5(b *testing.B)     { benchmarkEventStream(5, b) }
func BenchmarkEventStream10(b *testing.B)    { benchmarkEventStream(10, b) }
func BenchmarkEventStream50(b *testing.B)    { benchmarkEventStream(50, b) }
func BenchmarkEventStream100(b *testing.B)   { benchmarkEventStream(100, b) }
func BenchmarkEventStream1000(b *testing.B)  { benchmarkEventStream(1000, b) }
func BenchmarkEventStream10000(b *testing.B) { benchmarkEventStream(10000, b) }
