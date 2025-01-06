package ircbot

import (
	"context"
	"testing"
	"time"

	"github.com/R-a-dio/valkyrie/config"
	"github.com/stretchr/testify/assert"
)

func TestGuestExpire(t *testing.T) {
	ctx := context.Background()
	cfg := config.TestConfig()

	gs := NewGuestSystem(ctx, cfg, nil)

	nick := "test-user"
	gs.AddGuest(nick)

	assert.True(t, gs.IsGuest(nick, AccessPurposeKill))
	assert.Len(t, gs.Authorized, 1)

	// do an expire with zero timeout, so basically everyone should expire
	gs.doExpire(0)
	assert.False(t, gs.IsGuest(nick, AccessPurposeKill))
	assert.Len(t, gs.Authorized, 0)

	// add a crafted guest user in that got authed an hour ago
	gs.Authorized["a-while-ago"] = &Guest{
		AuthTime: time.Now().Add(-time.Hour),
	}

	// expire everyone that went past the 30 minutes since
	gs.doExpire(time.Minute * 30)

	assert.Len(t, gs.Authorized, 0)
}

func TestGuestIsGuest(t *testing.T) {
	ctx := context.Background()
	cfg := config.TestConfig()

	gs := NewGuestSystem(ctx, cfg, nil)

	nick := "test"
	assert.False(t, gs.IsGuest(nick, AccessPurposeKill))
	gs.AddGuest(nick)
	assert.True(t, gs.IsGuest(nick, AccessPurposeKill))
	assert.False(t, gs.IsGuest("something else", AccessPurposeKill))
}
