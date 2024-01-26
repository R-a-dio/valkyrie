package sse

import (
	"bytes"
	"testing"
	"time"
)

type eventCase struct {
	desc   string
	e      Event
	expect string
}

var eventEncodeCases = []eventCase{
	{
		"simple message",
		Event{Name: "queue", Data: []byte("queue information")},
		"event: queue\ndata: queue information\n\n",
	}, {
		"simple newlines",
		Event{Name: "thread", Data: []byte("some data\nwith\nnewlines")},
		"event: thread\ndata: some data\ndata: with\ndata: newlines\n\n",
	}, {
		"double newline",
		Event{Name: "metadata", Data: []byte("some data\n\nwith newlines")},
		"event: metadata\ndata: some data\ndata\ndata: with newlines\n\n",
	}, {
		"encode id",
		Event{ID: []byte("50")},
		"id: 50\n\n",
	}, {
		"encode retry",
		Event{Retry: time.Second * 10},
		"retry: 10000\n\n",
	}, {
		"encode everything",
		Event{ID: []byte("100"), Name: "metadata", Retry: time.Second * 50, Data: []byte("some data\nand a newline")},
		"id: 100\nevent: metadata\nretry: 50000\ndata: some data\ndata: and a newline\n\n",
	},
}

func TestEventEncode(t *testing.T) {
	for _, c := range eventEncodeCases {
		t.Run(c.desc, func(t *testing.T) {
			result := c.e.Encode()
			expect := []byte(c.expect)

			if !bytes.Equal(result, expect) {
				t.Errorf("%#v != %#v\n", string(result), string(expect))
			}
		})
	}
}

func BenchmarkEventEncodeSimple(b *testing.B) {
	e := Event{Name: "metadata", Data: []byte("some data\nand a newline")}
	for i := 0; i < b.N; i++ {
		e.Encode()
	}
}

func BenchmarkEventEncodeEverything(b *testing.B) {
	e := Event{ID: []byte("100"), Name: "metadata", Retry: time.Second * 50, Data: []byte("some data\nand a newline")}
	for i := 0; i < b.N; i++ {
		e.Encode()
	}
}
