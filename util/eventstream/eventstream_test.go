package eventstream

import (
	"context"
	"sync"
	"testing"
)

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

func TestEventServerLeave(t *testing.T) {
	es := NewEventStream[string]("hello world")

	ch := es.Sub()
	if len(es.subs) != 1 {
		t.Fatal("failed to subscribe")
	}
	es.Leave(ch)
	es.Send("secondary")
	if len(es.subs) != 0 {
		t.Fatal("failed to leave:", es.subs)
	}
}

func TestEventServerLeaveStream(t *testing.T) {
	es := NewEventStream[string]("hello world")

	s := es.SubStream(context.Background())
	if len(es.subs) != 1 {
		t.Fatal("failed to subscribe")
	}
	_ = s.Close()

	es.Send("secondary")
	if len(es.subs) != 0 {
		t.Fatal("failed to leave:", es.subs)
	}
}

func TestEventServerShutdown(t *testing.T) {
	es := NewEventStream[string]("primary")
	es.Shutdown()

	ch := es.Sub()
	if len(es.subs) != 0 {
		t.Fatal("added a subscriber after shutdown")
	}

	es.Leave(ch)
}

var testVar string

func benchmarkEventStream(subcount int, b *testing.B) {
	es := NewEventStream("hello world")

	var channels []chan string
	for i := 0; i < subcount; i++ {
		ch := es.Sub()
		channels = append(channels, ch)
		go func(ch chan string) {
			for m := range ch {
				testVar = m
			}
		}(ch)
	}

	thing := "some garbage"
	ch := es.Sub()
	go func() {
		for n := 0; n < b.N; n++ {
			es.Send(thing)
		}
	}()

	var counter = 0
	for range ch {
		counter++
		if counter >= b.N {
			break
		}
	}
	for _, ch := range channels {
		es.Leave(ch)
	}
}

func BenchmarkEventStream1(b *testing.B)     { benchmarkEventStream(1, b) }
func BenchmarkEventStream5(b *testing.B)     { benchmarkEventStream(5, b) }
func BenchmarkEventStream10(b *testing.B)    { benchmarkEventStream(10, b) }
func BenchmarkEventStream50(b *testing.B)    { benchmarkEventStream(50, b) }
func BenchmarkEventStream100(b *testing.B)   { benchmarkEventStream(100, b) }
func BenchmarkEventStream1000(b *testing.B)  { benchmarkEventStream(1000, b) }
func BenchmarkEventStream10000(b *testing.B) { benchmarkEventStream(10000, b) }
