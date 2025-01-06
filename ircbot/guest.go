package ircbot

import (
	"context"
	"sync"
	"time"

	"github.com/R-a-dio/valkyrie/config"
	"github.com/lrstanley/girc"
)

const (
	GUEST_EXPIRE_LOOP_TICK = time.Minute * 5
)

func GuestAuth(e Event) error {
	// TODO: allow people on a whitelist as well access to .auth
	if !e.HasAccess() {
		return nil
	}

	nick := e.Arguments["Nick"]
	if nick == "" {
		e.EchoPrivate("you need to supply a nickname")
		return nil
	}

	user := e.Client.LookupUser(nick)
	if user == nil {
		e.EchoPrivate("nickname does not exist")
		return nil
	}

	channel := e.Bot.Conf().IRC.MainChannel
	if !user.InChannel(channel) {
		e.EchoPrivate("{yellow}%s {clear}needs to be in the {yellow}%s {clear}channel", nick, channel)
		return nil
	}

	e.Bot.Guest.AddGuest(nick)
	e.EchoPublic("%s is/are authorized to guest DJ. Stick around for a comfy fire.", nick)
	return nil
}

func NewGuestSystem(ctx context.Context, cfg config.Config, client *girc.Client) *GuestSystem {
	gs := &GuestSystem{
		irc:        client,
		Authorized: map[GuestNick]*Guest{},
	}

	gs.registerHandlers()
	go gs.loopExpire(ctx, time.Hour*24)
	return gs
}

type GuestSystem struct {
	irc        *girc.Client
	mu         sync.Mutex
	Authorized map[GuestNick]*Guest
}

// AddGuest adds the nick given as a guest user
func (gs *GuestSystem) AddGuest(nick GuestNick) {
	gs.mu.Lock()
	defer gs.mu.Unlock()

	gs.Authorized[nick] = &Guest{
		Nick:     nick,
		AuthTime: time.Now(),
	}
	return
}

func (gs *GuestSystem) RemoveGuest(nick GuestNick) {
	gs.mu.Lock()
	defer gs.mu.Unlock()

	delete(gs.Authorized, nick)
}

func (gs *GuestSystem) IsGuest(nick GuestNick, purpose AccessPurpose) bool {
	gs.mu.Lock()
	defer gs.mu.Unlock()

	guest, ok := gs.Authorized[nick]
	switch purpose {
	case AccessPurposeKill:
		guest.KillAttempts++
	case AccessPurposeThread:
		guest.ThreadSets++
	}
	return ok
}

func (gs *GuestSystem) loopExpire(ctx context.Context, timeout time.Duration) {
	ticker := time.NewTicker(GUEST_EXPIRE_LOOP_TICK)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			gs.doExpire(timeout)
		case <-ctx.Done():
			return
		}
	}
}

func (gs *GuestSystem) registerHandlers() {
	gs.irc.Handlers.AddBg(girc.QUIT, func(client *girc.Client, event girc.Event) {
		gs.RemoveGuest(event.Source.Name)
	})
	gs.irc.Handlers.AddBg(girc.PART, func(client *girc.Client, event girc.Event) {
		gs.RemoveGuest(event.Source.Name)
	})
}

func (gs *GuestSystem) doExpire(timeout time.Duration) {
	gs.mu.Lock()
	defer gs.mu.Unlock()

	for nick, guest := range gs.Authorized {
		if time.Since(guest.AuthTime) < timeout {
			continue
		}

		delete(gs.Authorized, nick)
	}
}

type GuestNick = string

type Guest struct {
	Nick GuestNick
	// AuthTime is the time this guest got authorized
	AuthTime time.Time
	// ThreadSets is the amount of times this guest has used their .thread privilege
	ThreadSets int
	// KillAttempts is the amount of times this guest has used their .kill privilege
	KillAttempts int
}
