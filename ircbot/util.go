package ircbot

import (
	"fmt"
	"regexp"
	"strings"
	"time"
)

// pluralf returns fmt.Sprintf(format, amount) but returns the string without its last
// character when amount == 1
func pluralf(format string, amount int64) string {
	s := fmt.Sprintf(format, amount)
	if amount == 1 {
		return s[:len(s)-1]
	}
	return s
}

// formatDayDuration formats a Duration similar to Duration.String except it adds a day
// value on the front if required. Such that instead of the form "72h3m0.5s" this returns
// "3d3m0.5s"
func formatDayDuration(t time.Duration) string {
	if t < day {
		return t.String()
	}

	wholeDays := t.Truncate(day)
	return fmt.Sprintf("%dd%s", wholeDays/day, t-wholeDays)
}

// formatPlaybackDuration formats a Duration in the form mm:ss where mm are minutes and
// ss are seconds.
func formatPlaybackDuration(t time.Duration) string {
	minutes := t.Truncate(time.Minute)
	seconds := t - minutes

	return fmt.Sprintf("%.2d:%.2d", minutes/time.Minute, seconds/time.Second)
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

func formatLongDuration(t time.Duration) string {
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

		res = append(res, pluralf(longDurationFmt[i], int64(c)))
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
