package ircbot

import (
	"context"

	radio "github.com/R-a-dio/valkyrie"
	"github.com/lrstanley/girc"
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

	u, err := e.Bot.Guest.Auth(e.Ctx, nick)
	if err != nil {
		e.EchoPrivate("something went wrong")
		return err
	}

	e.Client.Cmd.Message(nick, Fmt("you are now authorized to stream, your username is {yellow}%s{clear}", u.Username))
	e.EchoPublic("%s is/are authorized to guest DJ. Stick around for a comfy fire.", nick)
	return nil
}

func GuestCreate(e Event) error {
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

	u, pwd, err := e.Bot.Guest.Create(e.Ctx, nick)
	if err != nil {
		e.EchoPrivate("something went wrong")
		return err
	}

	e.Client.Cmd.Message(nick, Fmt("you are now a guest streamer, your username is %s", u.Username))
	if pwd != "" {
		e.Client.Cmd.Message(nick, Fmt("and your password is {yellow}%s{clear} please store this somewhere", pwd))
	}
	e.EchoPublic("%s is/are now a guest DJ. Welcome them to the group.", nick)
	return nil
}

func RegisterGuestHandlers(ctx context.Context, client *girc.Client, gs radio.GuestService) {
	client.Handlers.AddBg(girc.QUIT, func(client *girc.Client, event girc.Event) {
		gs.Deauth(ctx, event.Source.Name)
	})
	client.Handlers.AddBg(girc.PART, func(client *girc.Client, event girc.Event) {
		gs.Deauth(ctx, event.Source.Name)
	})
}
