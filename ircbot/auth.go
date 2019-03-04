package ircbot

import (
	"time"

	radio "github.com/R-a-dio/valkyrie"
	"github.com/R-a-dio/valkyrie/errors"
	"github.com/lrstanley/girc"
)

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

func HasDeveloperAccess(e Event) (bool, error) {
	const op errors.Op = "irc/HasDeveloperAccess"

	// for security purposes we also require devs to always have channel access
	if !HasAccess(e.Client, e.Event) {
		return false, nil
	}

	// we also require them to have authed with nickserv
	if !IsAuthed(e) {
		return false, nil
	}

	us := e.Storage.User(e.Ctx)
	user, err := us.ByNick(e.Source.Name)
	if err != nil {
		return false, errors.E(op, err)
	}

	ok, err := us.HasPermission(*user, radio.PermDev)
	if err != nil {
		return false, errors.E(op, err)
	}
	return ok, nil
}
