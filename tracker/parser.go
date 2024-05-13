package tracker

import (
	"encoding/xml"
	"fmt"
	"io"
	"math"
	"strconv"
	"time"

	radio "github.com/R-a-dio/valkyrie"
)

type Icestats struct {
	XMLName xml.Name `xml:"icestats"`
	Text    string   `xml:",chardata"`
	Source  struct {
		Text      string `xml:",chardata"`
		Mount     string `xml:"mount,attr"`
		Listeners string `xml:"Listeners"`
		Listener  []struct {
			Text      string `xml:",chardata"`
			IP        string `xml:"IP"`
			UserAgent string `xml:"UserAgent"`
			Connected string `xml:"Connected"`
			ID        string `xml:"ID"`
		} `xml:"listener"`
	} `xml:"source"`
}

func ParseListClientsXML(r io.Reader) ([]radio.Listener, error) {
	var icestats Icestats

	err := xml.NewDecoder(r).Decode(&icestats)
	if err != nil {
		return nil, err
	}

	listenerCount, err := strconv.ParseUint(icestats.Source.Listeners, 10, 64)
	if err != nil {
		return nil, err
	}

	if listenerCount != uint64(len(icestats.Source.Listener)) {
		return nil, fmt.Errorf("mismatched listener count and listener entries")
	}

	raw := icestats.Source.Listener
	listeners := make([]radio.Listener, len(raw))
	now := time.Now()
	for i := range listeners {
		id, err := radio.ParseListenerClientID(raw[i].ID)
		if err != nil {
			return nil, err
		}

		div, err := strconv.ParseInt(raw[i].Connected, 10, 64)
		if err != nil {
			return nil, err
		}
		if div > math.MaxInt64/int64(time.Second) {
			return nil, fmt.Errorf("connected duration out of range")
		}
		start := now.Add(-time.Duration(div) * time.Second)

		listeners[i] = radio.Listener{
			IP:        raw[i].IP,
			UserAgent: raw[i].UserAgent,
			ID:        id,
			Start:     start,
		}
	}

	return listeners, nil
}
