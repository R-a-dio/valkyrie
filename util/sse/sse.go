package sse

import (
	"bytes"
	"slices"
	"strconv"
	"time"

	"github.com/R-a-dio/valkyrie/util/pool"
)

type Event struct {
	ID    []byte
	Name  string
	Retry time.Duration
	Data  []byte
}

var bufferPool = pool.NewResetPool(func() *bytes.Buffer { return new(bytes.Buffer) })

func (e Event) Encode() []byte {
	b := bufferPool.Get()
	defer bufferPool.Put(b)

	if e.ID != nil {
		b.WriteString("id: ")
		b.Write(e.ID)
		b.WriteString("\n")
	}
	if e.Name != "" {
		b.WriteString("event: ")
		b.WriteString(e.Name)
		b.WriteString("\n")
	}
	if e.Retry != 0 {
		b.WriteString("retry: ")
		b.WriteString(strconv.Itoa(int(e.Retry.Milliseconds())))
		b.WriteString("\n")
	}
	if e.Data != nil {
		for _, line := range bytes.Split(e.Data, []byte("\n")) {
			if len(line) == 0 {
				b.WriteString("data\n")
			} else {
				b.WriteString("data: ")
				b.Write(line)
				b.WriteString("\n")
			}
		}
	}
	b.WriteString("\n")

	return slices.Clone(b.Bytes())
}
