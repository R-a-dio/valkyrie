package ircbot

import (
	"context"
	"fmt"
	"time"

	"github.com/R-a-dio/valkyrie/database"
	"github.com/R-a-dio/valkyrie/rpc/manager"
	"github.com/lrstanley/girc"
)

var (
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

func RegisterCommandHandlers(b *Bot, c *girc.Client) error {
	h := CommandHandlers{b}
	c.Handlers.Add(girc.PRIVMSG, h.NowPlaying)
	return nil
}

type CommandHandlers struct {
	*Bot
}

func (h CommandHandlers) NowPlaying(c *girc.Client, e girc.Event) {
	// TODO: move out of handler
	if e.Trailing != "test!np" {
		return
	}

	// TODO above

	message := "Now playing:{red} '%s' {clear}[%s/%s](%d listeners), %s, %s, {red}LP:{clear} %s"

	status, err := h.manager.Status(context.TODO(), new(manager.StatusRequest))
	if err != nil {
		fmt.Println("status:", err)
		return
	}

	db := database.Handle(context.TODO(), h.DB)
	track, err := database.GetSongFromMetadata(db, status.Song.Metadata)
	if err != nil {
		fmt.Println("track:", err)
		return
	}

	var lastPlayedDiff time.Duration
	if !track.LastPlayed.IsZero() {
		lastPlayedDiff = time.Since(track.LastPlayed)
	}

	var songPosition time.Duration
	var songLength time.Duration

	{
		start := time.Unix(int64(status.Song.StartTime), 0)
		end := time.Unix(int64(status.Song.EndTime), 0)

		songPosition = time.Since(start)
		songLength = end.Sub(start)
	}

	var favoriteCount int64
	var playedCount int64

	message = girc.Fmt(message)
	message = fmt.Sprintf(message,
		status.Song.Metadata,
		formatPlaybackDuration(songPosition), formatPlaybackDuration(songLength),
		status.ListenerInfo.Listeners,
		pluralf("%d faves", favoriteCount),
		pluralf("played %d times", playedCount),
		formatLongDuration(lastPlayedDiff),
	)

	c.Cmd.Message(e.Params[0], message)
	return
}
