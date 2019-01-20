package ircbot

import (
	"fmt"
	"log"
	"regexp"
	"strings"
	"time"

	"github.com/R-a-dio/valkyrie/database"
)

// nowPlaying is a tiny layer around nowPlayingImpl to give it a signature the RegexHandler
// expects
func NowPlaying(e Event) error {
	fn := nowPlayingMessage(e)

	message, args, err := fn()
	if err != nil {
		return err
	}

	e.EchoPublic(message, args...)
	return nil
}

// messageFn is a function that returns a string and arguments ready to be passed to
// any of the fmt.*f functions
type messageFn func() (msg string, args []interface{}, err error)

// nowPlayingMessage implements the internals for both the NowPlaying command and
// Bot.AnnounceSong for the API
//func nowPlayingMessage(status radio.Status, db database.Handler, track CurrentTrack) messageFn {
func nowPlayingMessage(e Event) messageFn {
	return func() (string, []interface{}, error) {
		message := "Now playing:{red} '%s' {clear}[%s/%s](%s), %s, %s, {green}LP:{clear} %s"

		status, err := e.Bot.Manager.Status()
		if err != nil {
			return "", nil, err
		}
		// status returns a bare song; so refresh it from the db
		track, err := database.GetSongFromMetadata(e.Database(), status.Song.Metadata)
		if err != nil {
			return "", nil, err
		}

		var lastPlayedDiff time.Duration
		if !track.LastPlayed.IsZero() {
			lastPlayedDiff = time.Since(track.LastPlayed)
		}

		var songPosition time.Duration
		var songLength time.Duration

		{
			start := status.StreamInfo.SongStart
			end := status.StreamInfo.SongEnd

			songPosition = time.Since(start)
			songLength = end.Sub(start)
		}

		db := e.Database()
		favoriteCount, _ := database.SongFaveCount(db, *track)
		playedCount, _ := database.SongPlayedCount(db, *track)

		args := []interface{}{
			status.Song.Metadata,
			FormatPlaybackDuration(songPosition), FormatPlaybackDuration(songLength),
			Pluralf("%d listeners", int64(status.StreamInfo.Listeners)),
			Pluralf("%d faves", favoriteCount),
			Pluralf("played %d times", playedCount),
			FormatLongDuration(lastPlayedDiff),
		}

		return message, args, nil
	}
}

func LastPlayed(e Event) error          { return nil }
func StreamerQueue(e Event) error       { return nil }
func StreamerQueueLength(e Event) error { return nil }

func StreamerUserInfo(e Event) error {
	return nil
}

func FaveTrack(e Event) error { return nil }

func FaveList(e Event) error {
	var nick = e.Source.Name
	if n := e.Arguments["Nick"]; n != "" {
		nick = n
	}

	e.Echo("Favorites are at: https://r-a-d.io/faves/%s", nick)
	return nil
}

func ThreadURL(e Event) error {
	thread := e.Arguments["thread"]

	if thread != "" && HasAccess(e.Client, e.Event) {
		err := e.Bot.Manager.UpdateThread(thread)
		if err != nil {
			return err
		}
	}

	resp, err := e.Bot.Manager.Status()
	if err != nil {
		return err
	}

	e.Echo("Thread: %s", resp.Thread)
	return nil
}

var reTopicBit = regexp.MustCompile("(.*?r/)(.*)(/dio.*?)(.*)")

func ChannelTopic(e Event) error {
	channel := e.Client.LookupChannel(e.Params[0])
	if channel == nil {
		log.Println("unknown channel in .topic")
		// unknown channel?
		return nil
	}

	newTopic := e.Arguments["topic"]
	if newTopic != "" && HasAccess(e.Client, e.Event) {
		// we want to set the topic and have access for it
		match := reTopicBit.FindAllStringSubmatch(channel.Topic, -1)
		// a match is a [][]string of all matches, we only have one match so get rid
		// of the outer slice
		parts := match[0]
		// regexp returns the full match as the first element, so we get rid of it
		parts = parts[1:]
		// now we replace the relevant bits between the forward slashes
		parts[1] = Fmt("%s{orange}", newTopic)
		// and now we can just merge them back together
		newTopic = strings.Join(parts, "")

		e.Client.Cmd.Topic(channel.Name, newTopic)
		return nil
	}

	// no access, or just want to know what the topic currently is
	e.EchoPublic("Topic: %s", channel.Topic)
	return nil
}

func KillStreamer(e Event) error {
	// TODO: this should use special caseing for streamers that don't have channel
	// access
	if !HasAccess(e.Client, e.Event) {
		return nil
	}

	// TODO: not everyone should be able to force kill
	return e.Bot.Streamer.Stop(e.Arguments.Bool("force"))
}

func RandomTrackRequest(e Event) error { return nil }
func LuckyTrackRequest(e Event) error  { return nil }
func SearchTrack(e Event) error        { return nil }

func RequestTrack(e Event) error {
	/*return func() error {
		req := &streamer.TrackRequest{
			Identifier: e.Source.Host,
			Track:      int64(track.TrackID),
		}

		resp, err := s.RequestTrack(context.TODO(), req)
		if err != nil {
			return err
		}

		echo(resp.Msg)
		return nil
	}*/
	return nil
}

func LastRequestInfo(e Event) error {
	message := "%s last requested at {red}%s {clear}, which is {red}%s {clear} ago."

	var host = e.Source.Host
	var withArgument bool
	if nick := e.Arguments["Nick"]; nick != "" {
		u := e.Client.LookupUser(nick)
		if u == nil {
			return NewUserError(nil, "Unknown nickname or is not in the channel")
		}

		host = u.Host
		withArgument = true
	}

	t, err := database.UserRequestTime(e.Database(), host)
	if err != nil {
		return err
	}

	if t.IsZero() {
		if withArgument {
			e.Echo("%s has never requested before", e.Arguments["Nick"])
		} else {
			e.Echo("You've never requested before")
		}
	}

	// calculate if enough time has passed since the last request
	canRequest := time.Since(t) >= time.Duration(e.Bot.Conf().UserRequestDelay)
	if canRequest {
		if withArgument {
			message += fmt.Sprintf(" {green}%s can request", e.Arguments["Nick"])
		} else {
			message += " {green}You can request"
		}
	}

	var name = "You"
	if withArgument {
		name = e.Arguments["Nick"]
	}

	e.Echo(message,
		name,
		t.Format("Jan 02, 15:04:05"),
		FormatDayDuration(time.Since(t)),
	)

	return nil
}

func TrackInfo(e Event) error {
	message := "ID: {red}%d {clear}" +
		"Title: {red}%s {clear}" +
		"Faves: {red}%d {clear}" +
		"Plays: {red}%d {clear}" +
		"RC: {red}%d {clear}" +
		"Priority: {red}%d {clear}" +
		"CD: {red}%s (%s) {clear}" +
		"Accepter: {red}%s {clear}" +
		"Tags: {red}%s {clear}"

	track, err := e.ArgumentTrack("TrackID")
	if err != nil {
		track, err = e.CurrentTrack()
		if err != nil {
			return err
		}
	}

	if track.DatabaseTrack == nil {
		return NewUserError(nil, "song is not in the database")
	}

	db := e.Database()
	favoriteCount, _ := database.SongFaveCount(db, *track)
	playedCount, _ := database.SongPlayedCount(db, *track)

	// calculate the time remaining until this can be requested again
	var cooldownIndicator = "!"
	{
		compareTime := track.LastPlayed
		if track.LastRequested.After(track.LastPlayed) {
			compareTime = track.LastRequested
		}

		leftover := track.RequestDelay - time.Since(compareTime)
		if leftover > 0 {
			cooldownIndicator = leftover.String()
		}
	}

	e.Echo(message,
		track.TrackID,
		track.Metadata,
		favoriteCount,
		playedCount,
		track.RequestCount,
		track.Priority,
		track.RequestDelay, cooldownIndicator,
		track.Acceptor,
		track.Tags,
	)

	return nil
}

func TrackTags(e Event) error {
	message := "Title: {red}%s {clear}" +
		"Album: {red}%s {clear}" +
		"Faves: {red}%d {clear}" +
		"Plays: {red}%d {clear}" +
		"Tags: {red}%s {clear}"

	track, err := e.ArgumentTrack("TrackID")
	if err != nil {
		track, err = e.CurrentTrack()
		if err != nil {
			return err
		}
	}

	var album string
	var tags = "no tags available"
	if track.DatabaseTrack != nil {
		album, tags = track.Album, track.Tags
	}

	db := e.Database()
	favoriteCount, _ := database.SongFaveCount(db, *track)
	playedCount, _ := database.SongPlayedCount(db, *track)

	e.Echo(message,
		track.Metadata,
		album,
		favoriteCount,
		playedCount,
		tags,
	)

	return nil
}
