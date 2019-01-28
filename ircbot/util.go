package ircbot

import (
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/lrstanley/girc"
)

// Fmt passes the message to girc.Fmt and then calls fmt.Sprintf with arguments given
func Fmt(message string, args ...interface{}) string {
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

// FormatDayDuration formats a Duration similar to Duration.String except it adds a day
// value on the front if required. Such that instead of the form "72h3m0.5s" this returns
// "3d3m0.5s"
func FormatDayDuration(t time.Duration) string {
	if t < day {
		return t.String()
	}

	wholeDays := t.Truncate(day)
	return fmt.Sprintf("%dd%s", wholeDays/day, t-wholeDays)
}

// FormatPlaybackDuration formats a Duration in the form mm:ss where mm are minutes and
// ss are seconds.
func FormatPlaybackDuration(t time.Duration) string {
	minutes := t.Truncate(time.Minute)
	seconds := t - minutes

	return fmt.Sprintf("%.2d:%.2d", minutes/time.Minute, seconds/time.Second)
}

// FormatPlaybackDurationHours is  similar to FormatPlaybackDuration but also includes
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
	year  = time.Duration(float64(day) * 365.25)
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
)

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

// FindNamedSubmatches runs re.FindStringSubmatch(s) and only returns the groups that
// are named in the regexp
func FindNamedSubmatches(re *regexp.Regexp, s string) map[string]string {
	groups := re.FindStringSubmatch(s)
	if len(groups) == 0 {
		return nil
	}

	m := make(map[string]string, 4)
	for i, name := range re.SubexpNames() {
		if name == "" {
			continue
		}

		m[name] = groups[i]
	}

	return m
}

// HasAccess checks if the user that send the PRIVMSG to us has access +h or higher in
// the channel it came from; HasAccess panics if the event is not PRIVMSG
func HasAccess(c *girc.Client, e girc.Event) bool {
	if e.Command != girc.PRIVMSG {
		panic("HasAccess called with non-PRIVMSG event")
	}

	user := c.LookupUser(e.Source.Name)
	if user == nil {
		return false
	}

	perms, ok := user.Perms.Lookup(e.Params[0])
	if !ok {
		return false
	}

	return perms.IsAdmin() || perms.HalfOp
}

// HasStreamAccess is similar to HasAccess but also includes special casing for streamers
// that don't have channel access, but do have the authorization to access the stream
func HasStreamAccess(c *girc.Client, e girc.Event) bool {
	return HasAccess(c, e)
}
