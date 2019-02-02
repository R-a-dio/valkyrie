package ircbot

import (
	"testing"
	"time"
)

type testDurationFormat struct {
	// the input duration to test
	dur time.Duration
	// the expected output of each function
	dayDuration           string
	playbackDuration      string
	playbackDurationHours string
	longDuration          string
}

func dur(s string) time.Duration {
	d, err := time.ParseDuration(s)
	if err != nil {
		panic("invalid duration used in test setup: " + err.Error())
	}
	return d
}

var formatTests = []testDurationFormat{
	{dur("1h30m15s"), "1h30m15s", "90:15", "01:30:15", "1 hour 30 minutes 15 seconds"},
	{dur("90h"), "3d18h0m0s", "5400:00", "90:00:00", "3 days 18 hours"},
	{dur("60m"), "1h0m0s", "60:00", "01:00:00", "1 hour"},
	{dur("125m"), "2h5m0s", "125:00", "02:05:00", "2 hours 5 minutes"},
}

func TestFormatDayDuration(t *testing.T) {
	for _, d := range formatTests {
		expected := d.dayDuration
		out := FormatDuration(d.dur, time.Second)
		if out != expected {
			t.Errorf("(%s) as %s expected %s instead", d.dur, out, expected)
		}
	}
}

func TestFormatPlaybackDuration(t *testing.T) {
	for _, d := range formatTests {
		expected := d.playbackDuration
		out := FormatPlaybackDuration(d.dur)
		if out != expected {
			t.Errorf("(%s) as %s expected %s instead", d.dur, out, expected)
		}
	}
}

func TestFormatPlaybackDurationHours(t *testing.T) {
	for _, d := range formatTests {
		expected := d.playbackDurationHours
		out := FormatPlaybackDurationHours(d.dur)
		if out != expected {
			t.Errorf("(%s) as %s expected %s instead", d.dur, out, expected)
		}
	}
}

func TestFormatLongDuration(t *testing.T) {
	for _, d := range formatTests {
		expected := d.longDuration
		out := FormatLongDuration(d.dur)
		if out != expected {
			t.Errorf("(%s) as %s expected %s instead", d.dur, out, expected)
		}
	}
}
