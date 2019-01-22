package ircbot

import (
	"context"
	"fmt"
	"log"
	"regexp"
	"strconv"
	"time"

	radio "github.com/R-a-dio/valkyrie"
	"github.com/R-a-dio/valkyrie/database"
	"github.com/lrstanley/girc"
)

var (
	// rePrefix is prefixed to all the below regex at runtime
	rePrefix          = "^[.!@]"
	reNowPlaying      = "n(ow)?p(laying)?$"
	reLastPlayed      = "l(ast)?p(layed)?$"
	reQueue           = "q(ueue)?$"
	reQueueLength     = "q(ueue)? l(ength)?"
	reDJ              = "dj( (?P<isGuest>guest:)?(?P<DJ>.+))?"
	reFave            = "(?P<isNegative>un)?f(ave|avorite)( ((?P<TrackID>[0-9]+)|(?P<relative>(last($| ))+)))?"
	reFaveList        = "f(ave|avorite)?l(ist)?( (?P<Nick>.+))?"
	reThread          = "thread( (?P<thread>.+))?"
	reTopic           = "topic( (?P<topic>.+))?"
	reKill            = "kill( (?P<force>force))?"
	reRandomRequest   = "ra(ndom)?( ((?P<isFave>f(ave)?)( (?P<Nick>.+))?|(?P<Query>.+)))?"
	reLuckyRequest    = "l(ucky)? (?P<Query>.+)"
	reSearch          = "s(earch)? ((?P<TrackID>[0-9]+)|(?P<Query>.+))"
	reRequest         = "r(equest)? (?P<TrackID>[0-9]+)"
	reLastRequestInfo = "lastr(equest)?( (?P<Nick>.+))?"
	reTrackInfo       = "i(nfo)?( (?P<TrackID>[0-9]+))?"
	reTrackTags       = "tags( (?P<TrackID>[0-9]+))?"
)

// UserError is an error that includes a message suitable for the user
//
// a UserError returned from a handler is send to the user that invoked it
type UserError interface {
	error
	Public() bool
	UserError() string
}

type userError struct {
	err    error
	msg    string
	public bool
}

func (u userError) Error() string {
	return u.err.Error()
}

func (u userError) UserError() string {
	return u.msg
}

func (u userError) Public() bool {
	return u.public
}

// NewUserError returns a new error with the given msg for the user
func NewUserError(err error, msg string) error {
	return userError{err, msg, false}
}

// NewPublicError returns a new error with the given msg for the public channel
func NewPublicError(err error, msg string) error {
	return userError{err, msg, true}
}

func checkUserError(c *girc.Client, e girc.Event, err error) bool {
	uerr, ok := err.(UserError)
	if !ok {
		return false
	}

	if uerr.Public() {
		c.Cmd.Message(e.Params[0], uerr.UserError())
	} else {
		c.Cmd.Notice(e.Source.Name, uerr.UserError())
	}

	return true
}

type HandlerFn func(Event) error

type RegexHandler struct {
	regex string
	fn    HandlerFn
}

func NewRegexHandlers(ctx context.Context, bot *Bot, handlers ...RegexHandler) RegexHandlers {
	h := RegexHandlers{
		ctx:      ctx,
		bot:      bot,
		cache:    make([]*regexp.Regexp, len(handlers)),
		handlers: handlers,
	}

	for i, handler := range handlers {
		h.cache[i] = regexp.MustCompile(rePrefix + handler.regex)
	}

	return h
}

// RegexHandlers is a collection of handlers that are triggered based on a regular
// expression.
//
// An IRC events last parameter is used to match against.
type RegexHandlers struct {
	ctx      context.Context
	bot      *Bot
	cache    []*regexp.Regexp
	handlers []RegexHandler
}

// Execute implements girc.Handler
func (rh RegexHandlers) Execute(c *girc.Client, e girc.Event) {
	s := e.Trailing

	for i, re := range rh.cache {
		match := FindNamedSubmatches(re, s)
		if match == nil {
			continue
		}

		ctx, cancel := context.WithTimeout(rh.ctx, time.Second*5)
		defer cancel()

		event := Event{
			internal: &internalEvent{
				ctx:    ctx,
				handle: database.Handle(ctx, rh.bot.DB),
			},
			Event:     e,
			Arguments: match,
			Bot:       rh.bot,
			Client:    c,
		}

		// execute our handler
		err := rh.handlers[i].fn(event)
		if err != nil {
			if !checkUserError(c, e, err) {
				log.Println("handler error:", err)
			}
			return
		}

		return
	}
}

var reHandlers = []RegexHandler{
	{reNowPlaying, NowPlaying},
	{reLastPlayed, LastPlayed},
	{reQueue, StreamerQueue},
	{reQueueLength, StreamerQueueLength},
	{reDJ, StreamerUserInfo},
	{reFave, FaveTrack},
	{reFaveList, FaveList},
	{reThread, ThreadURL},
	{reTopic, ChannelTopic},
	{reKill, KillStreamer},
	{reRandomRequest, RandomTrackRequest},
	{reLuckyRequest, LuckyTrackRequest},
	{reSearch, SearchTrack},
	{reRequest, RequestTrack},
	{reLastRequestInfo, LastRequestInfo},
	{reTrackInfo, TrackInfo},
	{reTrackTags, TrackTags},
}

func RegisterCommandHandlers(ctx context.Context, b *Bot) error {
	h := NewRegexHandlers(ctx, b, reHandlers...)
	// while RegexHandlers is a girc.Handler, girc does not expose a way to register said
	// interface as background handler; so we pass the method bound to AddBg instead
	b.c.Handlers.AddBg(girc.PRIVMSG, h.Execute)
	return nil
}

// Arguments is a map of key:value pairs from the named capturing groups used in
// the regular expression used for the command
type Arguments map[string]string

// Bool returns true if the key exists and is non-empty
func (a Arguments) Bool(key string) bool {
	return a[key] != ""
}

type internalEvent struct {
	ctx    context.Context
	handle database.Handler
}

// Event is a collection of parameters to handler functions, fields of Event are exposed
// to handlers by dependency injection and you should never depend on Event directly
type Event struct {
	internal *internalEvent

	girc.Event
	Arguments Arguments

	Bot    *Bot
	Client *girc.Client
}

// Echo sends either a PRIVMSG to a channel or a NOTICE to a user based on the prefix
// used when running the command
func (e Event) Echo(message string, args ...interface{}) {
	switch e.Trailing[0] {
	case '.', '!':
		e.EchoPrivate(message, args...)
	case '@':
		e.EchoPublic(message, args...)
	default:
		panic("non-prefixed regular expression used")
	}
}

// EchoPrivate always sends a message as a NOTICE to the user that invoked the event
func (e Event) EchoPrivate(message string, args ...interface{}) {
	e.Client.Cmd.Notice(e.Source.Name, Fmt(message, args...))
}

// EchoPublic always sends a message as a PRIVMSG to the channel that
// the event was invoked on
func (e Event) EchoPublic(message string, args ...interface{}) {
	e.Client.Cmd.Message(e.Params[0], Fmt(message, args...))
}

// ArgumentTrack returns the key given interpreted as a radio.TrackID and returns the
// song associated with it.
func (e Event) ArgumentTrack(key string) (*radio.Song, error) {
	stringID := e.Arguments[key]
	if stringID == "" {
		return nil, fmt.Errorf("key '%s' not found in arguments", key)
	}

	id, err := strconv.Atoi(stringID)
	if err != nil {
		return nil, err
	}

	track, err := database.GetTrack(e.Database(), radio.TrackID(id))
	if err == nil {
		return track, nil
	}

	if err == database.ErrTrackNotFound {
		return nil, NewUserError(err, "unknown track identifier")
	}

	return nil, err
}

// CurrentTrack returns the currently playing song on the main stream configured
func (e Event) CurrentTrack() (*radio.Song, error) {
	status, err := e.Bot.Manager.Status(e.Context())
	if err != nil {
		return nil, err
	}

	return database.GetSongFromMetadata(e.Database(), status.Song.Metadata)
}

// Database returns a database handle with a timeout context
func (e Event) Database() database.Handler {
	return e.internal.handle
}

// Context returns the context associated with this event
func (e Event) Context() context.Context {
	return e.internal.ctx
}
