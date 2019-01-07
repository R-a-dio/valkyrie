// +build wireinject

package ircbot

import (
	"context"
	"errors"
	"fmt"
	"strconv"

	"github.com/R-a-dio/valkyrie/database"
	"github.com/R-a-dio/valkyrie/rpc/manager"
	"github.com/lrstanley/girc"
	"github.com/google/wire"
	"github.com/jmoiron/sqlx"
)

var Providers = wire.NewSet(
	WithBot,
	WithDB,
	WithClient,
	WithIRCEvent,

	WithDatabase,
	//WithDatabaseTx, // TODO: find a way to handle Commit/Rollback
	WithManagerStatus,

	WithArguments,

	WithCurrentTrack,
	WithArgumentTrack,
	WithArgumentOrCurrentTrack,

	WithRespond,
	WithRespondPrivate,
	WithRespondPublic,
)

type CommandFn func() error

type Arguments map[string]string

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

func WithManagerStatus(bot *Bot) (*manager.StatusResponse, error) {
	return bot.Manager.Status(context.TODO(), new(manager.StatusRequest))
}

func WithDatabase(db *sqlx.DB) database.Handler {
	return database.Handle(context.TODO(), db)
}

func WithDatabaseTx(db *sqlx.DB) (database.HandlerTx, error) {
	return database.HandleTx(context.TODO(), db)
}

// ArgumentTrack is a track specified by user arguments
type ArgumentTrack struct {
	database.Track
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
	
	track, err := database.GetTrack(h, database.TrackID(tid))
	return ArgumentTrack{track}, err
}

// CurrentTrack is a track that is currently being played on stream
type CurrentTrack struct { 
	database.Track
}

// WithCurrentTrack returns the currently playing track
func WithCurrentTrack(h database.Handler, s *manager.StatusResponse) (CurrentTrack, error) {
	track, err := database.GetSongFromMetadata(h, s.Song.Metadata)
	return CurrentTrack{track}, err 
}

type ArgumentOrCurrentTrack struct {
	database.Track 
}

// WithArgumentOrCurrentTrack combines WithArgumentTrack and WithCurrentTrack returning
// ArgumentTrack first if available
func WithArgumentOrCurrentTrack(h database.Handler, a Arguments, s *manager.StatusResponse) (ArgumentOrCurrentTrack, error) {
	trackA, err := WithArgumentTrack(h, a)
	if err == nil {
		return ArgumentOrCurrentTrack{trackA.Track}, err
	} else if err == database.ErrTrackNotFound {
		return ArgumentOrCurrentTrack{}, NewUserError(err, "track identifier does not exist")
	}

	trackC, err := WithCurrentTrack(h, s)
	return ArgumentOrCurrentTrack{trackC.Track}, err
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
		message = girc.Fmt(message)
		message = fmt.Sprintf(message, args...)

		c.Cmd.Notice(e.Source.Name, message)
	}
}

func WithRespondPublic(c *girc.Client, e girc.Event) RespondPublic {
	return func(message string, args ...interface{}) {
		message = girc.Fmt(message)
		message = fmt.Sprintf(message, args...)

		c.Cmd.Message(e.Params[0], message)
	}
}

// ======================================
// below are only wire Build instructions
// ======================================

func NowPlaying(Event) (CommandFn, error) {
	wire.Build(Providers, nowPlaying)
	return nil, nil
}

func LastPlayed(Event) (CommandFn, error) {
	//wire.Build(Providers, lastPlayed)
	return nil, nil
}

func StreamerQueue(Event) (CommandFn, error) {
	//wire.Build(Providers, streamerQueue)
	return nil, nil
}

func StreamerQueueLength(Event) (CommandFn, error) {
	//wire.Build(Providers, streamerQueueLength)
	return nil, nil
}

func StreamerUserInfo(Event) (CommandFn, error) {
	//wire.Build(Providers, streamerUserInfo)
	return nil, nil
}

func FaveTrack(Event) (CommandFn, error) {
	//wire.Build(Providers, faveTrack)
	return nil, nil
}

func FaveList(Event) (CommandFn, error) {
	//wire.Build(Providers, faveList)
	return nil, nil
}

func ThreadURL(Event) (CommandFn, error) {
	//wire.Build(Providers, threadURL)
	return nil, nil
}

func ChannelTopic(Event) (CommandFn, error) {
	//wire.Build(Providers, channelTopic)
	return nil, nil
}

func KillStreamer(Event) (CommandFn, error) {
	//wire.Build(Providers, killStreamer)
	return nil, nil
}

func RandomTrackRequest(Event) (CommandFn, error) {
	//wire.Build(Providers, randomTrackRequest)
	return nil, nil
}

func LuckyTrackRequest(Event) (CommandFn, error) {
	//wire.Build(Providers, luckyTrackRequest)
	return nil, nil
}

func SearchTrack(Event) (CommandFn, error) {
	//wire.Build(Providers, searchTrack)
	return nil, nil
}

func RequestTrack(Event) (CommandFn, error) {
	//wire.Build(Providers, requestTrack)
	return nil, nil
}

func LastRequestInfo(Event) (CommandFn, error) {
	//wire.Build(Providers, lastRequestInfo)
	return nil, nil
}

func TrackInfo(Event) (CommandFn, error) {
	//wire.Build(Providers, trackInfo)
	return nil, nil
}

func TrackTags(Event) (CommandFn, error) {
	wire.Build(Providers, trackTags)
	return nil, nil
}


