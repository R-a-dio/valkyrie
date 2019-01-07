package ircbot

import (
	"log"
	"regexp"

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
	reFave            = "(?P<isNegative>un)?f(ave|avorite)( ((?P<TrackID>[0-9]+)|(?P<relative>last)))?"
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

type HandlerFn func(Event) (CommandFn, error)

type RegexHandler struct {
	regex string
	fn    HandlerFn
}

func NewRegexHandlers(bot *Bot, handlers ...RegexHandler) RegexHandlers {
	h := RegexHandlers{
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

		event := Event{
			bot: rh.bot,
			c:   c,
			e:   e,
			a:   match,
		}

		// create our handler
		fn, err := rh.handlers[i].fn(event)
		if err != nil {
			if !checkUserError(c, e, err) {
				log.Println("provider error:", err)
			}
			return
		}

		// execute our handler
		err = fn()
		if err != nil {
			if !checkUserError(c, e, err) {
				log.Println("handler error:", err)
			}
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

func RegisterCommandHandlers(b *Bot, c *girc.Client) error {
	h := NewRegexHandlers(b, reHandlers...)
	// while RegexHandlers is a girc.Handler, girc does not expose a way to register said
	// interface as background handler; so we pass the method bound to AddBg instead
	c.Handlers.AddBg(girc.PRIVMSG, h.Execute)
	return nil
}

// Event is a collection of parameters to handler functions, fields of Event are exposed
// to handlers by dependency injection and you should never depend on Event directly
type Event struct {
	bot *Bot
	c   *girc.Client
	e   girc.Event
	a   Arguments
}
