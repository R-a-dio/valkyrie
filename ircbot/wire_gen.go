// Code generated by Wire. DO NOT EDIT.

//go:generate wire
//+build !wireinject

package ircbot

import (
	"context"
	"errors"
	"github.com/R-a-dio/valkyrie"
	"github.com/R-a-dio/valkyrie/database"
	"github.com/google/wire"
	"github.com/jmoiron/sqlx"
	"github.com/lrstanley/girc"
	"strconv"
)

// Injectors from providers.go:

func NowPlaying(event Event) (CommandFn, error) {
	client := WithClient(event)
	gircEvent := WithIRCEvent(event)
	respondPublic := WithRespondPublic(client, gircEvent)
	commandFn := nowPlaying(event, respondPublic)
	return commandFn, nil
}

func NowPlayingMessage(event Event) (messageFn, error) {
	bot := WithBot(event)
	status, err := WithManagerStatus(bot)
	if err != nil {
		return nil, err
	}
	db := WithDB(bot)
	handler := WithDatabase(db)
	currentTrack, err := WithCurrentTrack(handler, status)
	if err != nil {
		return nil, err
	}
	ircbotMessageFn := nowPlayingMessage(status, handler, currentTrack)
	return ircbotMessageFn, nil
}

func ThreadURL(event Event) (CommandFn, error) {
	client := WithClient(event)
	gircEvent := WithIRCEvent(event)
	respondPublic := WithRespondPublic(client, gircEvent)
	arguments := WithArguments(event)
	bot := WithBot(event)
	managerService := WithManager(bot)
	access := WithAccess(client, gircEvent)
	commandFn := threadURL(respondPublic, arguments, managerService, access)
	return commandFn, nil
}

func ChannelTopic(event Event) (CommandFn, error) {
	client := WithClient(event)
	gircEvent := WithIRCEvent(event)
	respondPublic := WithRespondPublic(client, gircEvent)
	arguments := WithArguments(event)
	commandFn := channelTopic(respondPublic, arguments, client, gircEvent)
	return commandFn, nil
}

func KillStreamer(event Event) (CommandFn, error) {
	bot := WithBot(event)
	streamerService := WithStreamer(bot)
	arguments := WithArguments(event)
	client := WithClient(event)
	gircEvent := WithIRCEvent(event)
	access := WithAccess(client, gircEvent)
	commandFn := killStreamer(streamerService, arguments, access)
	return commandFn, nil
}

func RequestTrack(event Event) (CommandFn, error) {
	bot := WithBot(event)
	streamerService := WithStreamer(bot)
	commandFn := requestTrack(streamerService)
	return commandFn, nil
}

func TrackTags(event Event) (CommandFn, error) {
	client := WithClient(event)
	gircEvent := WithIRCEvent(event)
	respond := WithRespond(client, gircEvent)
	bot := WithBot(event)
	db := WithDB(bot)
	handler := WithDatabase(db)
	arguments := WithArguments(event)
	status, err := WithManagerStatus(bot)
	if err != nil {
		return nil, err
	}
	argumentOrCurrentTrack, err := WithArgumentOrCurrentTrack(handler, arguments, status)
	if err != nil {
		return nil, err
	}
	commandFn := trackTags(respond, handler, argumentOrCurrentTrack)
	return commandFn, nil
}

// providers.go:

var Providers = wire.NewSet(

	WithBot,
	WithDB,
	WithClient,
	WithIRCEvent,

	WithDatabase,

	WithStreamer,
	WithManager,
	WithManagerStatus,

	WithArguments,
	WithCurrentTrack,
	WithArgumentTrack,
	WithArgumentOrCurrentTrack,

	WithRespond,
	WithRespondPrivate,
	WithRespondPublic,

	WithAccess,
)

type CommandFn func() error

// Arguments is a map of key:value pairs from the named capturing groups used in
// the regular expression used for the command
type Arguments map[string]string

// Bool returns true if the key exists and is non-empty
func (a Arguments) Bool(key string) bool {
	return a[key] != ""
}

func WithBot(e Event) *Bot {
	return e.bot
}

func WithDB(bot *Bot) *sqlx.DB {
	return bot.DB
}

func WithIRCEvent(e Event) girc.Event {
	return e.e
}

func WithClient(e Event) *girc.Client {
	return e.c
}

func WithArguments(e Event) Arguments {
	return e.a
}

func WithStreamer(bot *Bot) radio.StreamerService {
	return bot.streamer
}

func WithManager(bot *Bot) radio.ManagerService {
	return bot.manager
}

func WithManagerStatus(bot *Bot) (radio.Status, error) {
	return bot.manager.Status()
}

func WithDatabase(db *sqlx.DB) database.Handler {
	return database.Handle(context.TODO(), db)
}

func WithDatabaseTx(db *sqlx.DB) (database.HandlerTx, error) {
	return database.HandleTx(context.TODO(), db)
}

// ArgumentTrack is a track specified by user arguments
type ArgumentTrack struct {
	radio.Song
}

// WithArgumentTrack returns the track specified by the argument 'TrackID'
func WithArgumentTrack(h database.Handler, a Arguments) (ArgumentTrack, error) {
	id := a["TrackID"]
	if id == "" {
		return ArgumentTrack{}, errors.New("no TrackID found in arguments")
	}

	tid, err := strconv.Atoi(id)
	if err != nil {
		panic("non-numeric TrackID found from arguments: " + id)
	}

	track, err := database.GetTrack(h, radio.TrackID(tid))
	return ArgumentTrack{*track}, err
}

// CurrentTrack is a track that is currently being played on stream
type CurrentTrack struct {
	radio.Song
}

// WithCurrentTrack returns the currently playing track
func WithCurrentTrack(h database.Handler, s radio.Status) (CurrentTrack, error) {
	track, err := database.GetSongFromMetadata(h, s.Song.Metadata)
	return CurrentTrack{*track}, err
}

type ArgumentOrCurrentTrack struct {
	radio.Song
}

// WithArgumentOrCurrentTrack combines WithArgumentTrack and WithCurrentTrack returning
// ArgumentTrack first if available
func WithArgumentOrCurrentTrack(h database.Handler, a Arguments, s radio.Status) (ArgumentOrCurrentTrack, error) {
	trackA, err := WithArgumentTrack(h, a)
	if err == nil {
		return ArgumentOrCurrentTrack{trackA.Song}, err
	} else if err == database.ErrTrackNotFound {
		return ArgumentOrCurrentTrack{}, NewUserError(err, "track identifier does not exist")
	}

	trackC, err := WithCurrentTrack(h, s)
	return ArgumentOrCurrentTrack{trackC.Song}, err
}

type Respond func(message string, args ...interface{})

type RespondPrivate func(message string, args ...interface{})

type RespondPublic func(message string, args ...interface{})

func WithRespond(c *girc.Client, e girc.Event) Respond {
	switch e.Trailing[0] {
	case '.', '!':
		return Respond(WithRespondPrivate(c, e))
	case '@':
		return Respond(WithRespondPublic(c, e))
	default:
		panic("non-prefixed regular expression used")
	}
}

func WithRespondPrivate(c *girc.Client, e girc.Event) RespondPrivate {
	return func(message string, args ...interface{}) {
		c.Cmd.Notice(e.Source.Name, Fmt(message, args...))
	}
}

func WithRespondPublic(c *girc.Client, e girc.Event) RespondPublic {
	return func(message string, args ...interface{}) {
		c.Cmd.Message(e.Params[0], Fmt(message, args...))
	}
}

type Access bool

func WithAccess(c *girc.Client, e girc.Event) Access {
	return Access(HasAccess(c, e))
}

func LastPlayed(Event) (CommandFn, error) {

	return nil, nil
}

func StreamerQueue(Event) (CommandFn, error) {

	return nil, nil
}

func StreamerQueueLength(Event) (CommandFn, error) {

	return nil, nil
}

func StreamerUserInfo(Event) (CommandFn, error) {

	return nil, nil
}

func FaveTrack(Event) (CommandFn, error) {

	return nil, nil
}

func FaveList(Event) (CommandFn, error) {

	return nil, nil
}

func RandomTrackRequest(Event) (CommandFn, error) {

	return nil, nil
}

func LuckyTrackRequest(Event) (CommandFn, error) {

	return nil, nil
}

func SearchTrack(Event) (CommandFn, error) {

	return nil, nil
}

func LastRequestInfo(Event) (CommandFn, error) {

	return nil, nil
}

func TrackInfo(Event) (CommandFn, error) {

	return nil, nil
}
