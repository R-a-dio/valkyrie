package ircbot

import (
	"fmt"
	"strings"
	"time"

	"github.com/lrstanley/girc"
)

// Fmt passes the message to girc.Fmt and then calls fmt.Sprintf with arguments given
func Fmt(message string, args ...any) string {
	message = girc.Fmt(message)
	return fmt.Sprintf(message, args...)
}

// Pluralf returns fmt.Sprintf(format, amount) but returns the string without its last
// character when amount == 1
func Pluralf(format string, amount int64) string {
	s := fmt.Sprintf(format, amount)
	if amount == 1 {
		return s[:len(s)-1]
	}
	return s
}

// FormatDuration formats a Duration similar to Duration.String except it adds a possible
// month and day value to the front if available. Such that instead of the form
// "72h3m0.5s" this returns "3d3m0.5s".
//
// The truncate argument indicates the smallest unit returned in the string
func FormatDuration(t time.Duration, truncate time.Duration) string {
	if t < truncate {
		return ""
	}

	var args []any
	var msg []string
	for i, d := range longDurations {
		if t < truncate {
			break
		}
		if t < d {
			continue
		}

		c := t.Truncate(d)
		t -= c
		c /= d
		if c > 0 {
			msg = append(msg, "%d%s")
			args = append(args, c, longDurationSmall[i])
		}
	}

	return fmt.Sprintf(strings.Join(msg, ""), args...)
}

// FormatPlaybackDuration formats a Duration in the form mm:ss where mm are minutes and
// ss are seconds.
func FormatPlaybackDuration(t time.Duration) string {
	minutes := t.Truncate(time.Minute)
	seconds := t - minutes

	return fmt.Sprintf("%.2d:%.2d", minutes/time.Minute, seconds/time.Second)
}

// FormatPlaybackDurationHours is similar to FormatPlaybackDuration but also includes
// the hour part, making it "hh:mm:ss"
func FormatPlaybackDurationHours(t time.Duration) string {
	hours := t.Truncate(time.Hour)
	t -= hours
	minutes := t.Truncate(time.Minute)
	seconds := t - minutes

	return fmt.Sprintf("%.2d:%.2d:%.2d",
		hours/time.Hour, minutes/time.Minute, seconds/time.Second)
}

var (
	year  = time.Duration(float64(day) * 365.2422)
	month = year / 12
	week  = day * 7
	day   = time.Hour * 24

	longDurations   = []time.Duration{year, month, week, day, time.Hour, time.Minute, time.Second}
	longDurationFmt = []string{
		"%d years",
		"%d months",
		"%d weeks",
		"%d days",
		"%d hours",
		"%d minutes",
		"%d seconds",
	}
	longDurationSmall = []string{"y", "m", "w", "d", "h", "m", "s"}
)

// FormatLongDuration formats a Duration in a long form with each unit spelled out
// completely and spaced properly. For example a duration of 1d5h22s would be formatted
// as `1 day, 5 hours, 22 seconds`
func FormatLongDuration(t time.Duration) string {
	if t <= 0 {
		return "Never before"
	}

	var res = make([]string, 0, 10)

	for i, d := range longDurations {
		if t < d {
			continue
		}

		c := t.Truncate(d)
		t -= c
		c /= d

		res = append(res, Pluralf(longDurationFmt[i], int64(c)))
	}

	return strings.Join(res, " ")
}
