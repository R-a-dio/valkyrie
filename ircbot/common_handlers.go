package ircbot

import (
	"github.com/lrstanley/girc"
)

// RegisterCommonHandlers registers non-command handlers that are required for a
// functional client
func RegisterCommonHandlers(b *Bot, c *girc.Client) error {
	ch := CommonHandlers{b}
	c.Handlers.Add(girc.CONNECTED, ch.AuthenticateWithNickServ)
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
func (h CommonHandlers) AuthenticateWithNickServ(c *girc.Client, _ girc.Event) {
	conf := h.Conf()
	if conf.IRC.NickPassword != "" {
		c.Cmd.Messagef("nickserv", "id %s", conf.IRC.NickPassword)
	}
}

// JoinDefaultChannels joins all the channels listed in the configuration file
func (h CommonHandlers) JoinDefaultChannels(c *girc.Client, _ girc.Event) {
	conf := h.Conf()
	for _, channel := range conf.IRC.Channels {
		c.Cmd.Join(channel)
	}
}
