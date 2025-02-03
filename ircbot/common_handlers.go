package ircbot

import (
	"github.com/lrstanley/girc"
)

// RegisterCommonHandlers registers non-command handlers that are required for a
// functional client
func RegisterCommonHandlers(b *Bot, c *girc.Client) error {
	ch := CommonHandlers{b}
	c.Handlers.Add(girc.CONNECTED, ch.AuthenticateWithNickServ)
	c.Handlers.Add(girc.NICK, ch.AuthenticateWithNickServ)
	c.Handlers.Add(girc.CONNECTED, ch.JoinDefaultChannels)
	return nil
}

// CommonHandlers groups all handlers that should always be registered to
// function as basic irc bot
type CommonHandlers struct {
	*Bot
}

// AuthenticateWithNickServ tries to authenticate with nickserv with the password
// configured
func (h CommonHandlers) AuthenticateWithNickServ(c *girc.Client, e girc.Event) {
	if e.Command == girc.NICK {
		// ignore the event if it's not our nick being changed
		if e.Params[0] != c.GetNick() {
			return
		}
	}

	if h.cfgNickPassword() != "" && c.GetNick() == h.cfgNick() {
		c.Cmd.Messagef("nickserv", "id %s", h.cfgNickPassword())
	}
}

// JoinDefaultChannels joins all the channels listed in the configuration file
func (h CommonHandlers) JoinDefaultChannels(c *girc.Client, _ girc.Event) {
	for _, channel := range h.cfgChannels() {
		c.Cmd.Join(channel)
	}
}
