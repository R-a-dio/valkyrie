package ircbot

import (
	"fmt"
	"log"
	"regexp"
	"strings"
	"time"

	radio "github.com/R-a-dio/valkyrie"
	"github.com/R-a-dio/valkyrie/database"
)

func NowPlaying(e Event) error {
	// there is very similar looking code located in api.go under AnnounceSong, so if
	// you introduce a change here you might want to see if that change is also required
	// in the announcement code
	message := "Now playing:{red} '%s' {clear}[%s/%s](%s), %s, %s, {green}LP:{clear} %s"

	status, err := e.Bot.Manager.Status(e.Context())
	if err != nil {
		return err
	}
	// status returns a bare song; so refresh it from the db
	track, err := database.GetSongFromMetadata(e.Database(), status.Song.Metadata)
	if err != nil {
		return err
	}

	var lastPlayedDiff time.Duration
	if !track.LastPlayed.IsZero() {
		lastPlayedDiff = time.Since(track.LastPlayed)
	}

	songPosition := time.Since(status.SongInfo.Start)
	songLength := status.SongInfo.End.Sub(status.SongInfo.Start)

	db := e.Database()
	favoriteCount, _ := database.SongFaveCount(db, *track)
	playedCount, _ := database.SongPlayedCount(db, *track)

	e.EchoPublic(message,
		status.Song.Metadata,
		FormatPlaybackDuration(songPosition), FormatPlaybackDuration(songLength),
		Pluralf("%d listeners", int64(status.Listeners)),
		Pluralf("%d faves", favoriteCount),
		Pluralf("played %d times", playedCount),
		FormatLongDuration(lastPlayedDiff),
	)

	return nil
}

func LastPlayed(e Event) error {
	songs, err := database.GetLastPlayed(e.Database(), 0, 5)
	if err != nil {
		return err
	}

	message := "{green}Last Played:{clear} %s"
	messageJoin := " {green}|{clear} "
	onlyMetadata := make([]string, len(songs))
	for i, song := range songs {
		onlyMetadata[i] = song.Metadata
	}

	message = fmt.Sprintf(message, strings.Join(onlyMetadata, messageJoin))
	e.EchoPublic(message)
	return nil
}

func StreamerQueue(e Event) error       { return nil }
func StreamerQueueLength(e Event) error { return nil }

func StreamerUserInfo(e Event) error {
	return nil
}

func FaveTrack(e Event) error {
	var song radio.Song

	if e.Arguments.Bool("relative") {
		// for when `last` is given as argument

		// count the amount of `last`'s used to determine how far back we should go
		index := strings.Count(e.Arguments["relative"], "last") - 1
		songs, err := database.GetLastPlayed(e.Database(), index, 1)
		if err != nil {
			return err
		}
		song = songs[0]
	} else if e.Arguments.Bool("TrackID") {
		// for when a track number is given as argument
		s, err := e.ArgumentTrack("TrackID")
		if err != nil {
			return err
		}
		song = *s
	} else {
		// for when there is no argument given
		s, err := e.CurrentTrack()
		if err != nil {
			return err
		}
		song = *s
	}

	// now check to see if we want to favorite or unfavorite something
	var dbFunc = database.FaveSong
	if e.Arguments.Bool("isNegative") {
		dbFunc = database.UnfaveSong
	}

	changed, err := dbFunc(e.Database(), e.Source.Name, song)
	if err != nil {
		return err
	}

	// now we need the correct message based on the success of the query
	// `changed` will be true if the database was changed
	var message string
	if e.Arguments.Bool("isNegative") {
		if changed {
			message = "{green}'%s'{clear} is removed from your favorites."
		} else {
			message = "You don't have {green}'%s'{clear} in your favorites."
		}
	} else {
		if changed {
			message = "Added {green}'%s'{clear} to your favorites."
		} else {
			message = "You already have {green}'%s'{clear} favorited."
		}
	}

	e.EchoPrivate(message, song.Metadata)
	return nil
}

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
		err := e.Bot.Manager.UpdateThread(e.Context(), thread)
		if err != nil {
			return err
		}
	}

	resp, err := e.Bot.Manager.Status(e.Context())
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
	err := e.Bot.Streamer.Stop(e.Context(), e.Arguments.Bool("force"))
	if err != nil {
		return NewUserError(err, "Something went wrong ;_;, trying again will only make it worse, hauu~")
	}

	status, err := e.Bot.Manager.Status(e.Context())
	if err != nil {
		e.EchoPublic("Disconnecting after the current song")
	} else {
		e.EchoPublic("Disconnecting in about %s",
			time.Until(status.SongInfo.End))
	}

	return nil
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
			return NewUserError(nil, "I don't know who that is")
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
		return nil
	}

	// calculate if enough time has passed since the last request
	canRequest := time.Since(t) >= time.Duration(e.Bot.Conf().UserRequestDelay)
	if canRequest {
		if withArgument {
			message += fmt.Sprintf(" {green}%s can request", e.Arguments["Nick"])
		} else {
			message += " {green}You can request!"
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
