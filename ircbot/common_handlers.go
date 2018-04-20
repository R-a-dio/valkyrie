package ircbot

import "github.com/lrstanley/girc"

// CommonHandlers groups all handlers that should always be registered to
// function as basic irc bot
type CommonHandlers struct {
	*State
}

// RegisterCommonHandlers registers all common handlers with the client
// associated with the state given
func RegisterCommonHandlers(s *State) {
	h := CommonHandlers{s}
	h.client.Handlers.Add(girc.CONNECTED, h.AuthenticateNickServ)
	h.client.Handlers.Add(girc.CONNECTED, h.JoinDefaultChannels)
}

// AuthenticateNickServ tries to authenticate with nickserv with the password
// configured
func (h CommonHandlers) AuthenticateNickServ(c *girc.Client, _ girc.Event) {
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
