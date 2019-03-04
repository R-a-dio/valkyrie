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

// FormatDuration formats a Duration similar to Duration.String except it adds a possible
// month and day value to the front if available. Such that instead of the form
// "72h3m0.5s" this returns "3d3m0.5s".
//
// The truncate argument indicates the smallest unit returned in the string
func FormatDuration(t time.Duration, truncate time.Duration) string {
	if t < truncate {
		return ""
	}

	var args []interface{}
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
	longDurationSmall = []string{"y", "m", "w", "d", "h", "m", "s"}
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

// IsAuthed checks if the source of the event is authenticated with nickserv
func IsAuthed(e Event) bool {
	nick := e.Source.Name
	// wait at maximum timeout seconds for a reply before giving up
	timeout := time.Second * 3
	// channel to tell us if we got authed or not, we only care about the first value
	// in the channel, the other one is a bogus result
	authCh := make(chan bool, 2)

	// prepare our handler for a whois reply, we either get the 307 reply we want to
	// indicate that our user is authenticated, or we get back nothing and our
	// ENDOFWHOIS handler will tell us that the whois end was reached without 307
	id, _ := e.Client.Handlers.AddTmp("307", timeout, func(c *girc.Client, e girc.Event) bool {
		if e.Params[1] != nick {
			return false
		}
		select {
		case authCh <- true:
		default:
			// this default cause should never happen, but it might be possible to
			// trigger it by having the same user call a command that checks
			// authentication in rapid succession, so we protect against that
		}
		return true
	})
	defer e.Client.Handlers.Remove(id)

	id, _ = e.Client.Handlers.AddTmp(girc.RPL_ENDOFWHOIS, timeout, func(c *girc.Client, e girc.Event) bool {
		if e.Params[1] != nick {
			// not the nick we're looking for
			return false
		}
		select {
		case authCh <- false:
		default:
			// this default cause should never happen, but it might be possible to
			// trigger it by having the same user call a command that checks
			// authentication in rapid succession, so we protect against that
		}
		return true
	})
	defer e.Client.Handlers.Remove(id)

	// send a whois and then wait for one of our handlers to be done
	e.Client.Cmd.Whois(nick)

	select {
	case ok := <-authCh:
		return ok
	case <-time.After(timeout):
		// girc gives us a done channel that is closed after the timeout, but we rather
		// have a single synchronization point in the authCh such that we eliminate a
		// race between girc internals and our temporary handlers.
		return false
	}
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

func HasAdminAccess(e Event) bool {
	// for security purposes we also require admins to always have channel access
	if !HasAccess(e.Client, e.Event) {
		return false
	}

	// we also require them to have authed with nickserv
	if !IsAuthed(e) {
		return false
	}

	/*
		user, err := e.Storage.User(e.Ctx).ByNick(e.Source.Name)
		if err != nil {
			return false
		}

		return user.HasPermission("dev")
	*/
	return false
}
