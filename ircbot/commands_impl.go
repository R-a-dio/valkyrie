package ircbot

import (
	"context"
	"log"
	"math/rand"
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
	songLength := status.Song.Length

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

	message := "{green}Last Played:{clear}"
	messageJoin := "{green}|{clear}"
	onlyFmt := make([]string, len(songs))
	onlyMetadata := make([]interface{}, len(songs))
	for i, song := range songs {
		onlyFmt[i] = " %s "
		onlyMetadata[i] = song.Metadata
	}

	message = message + strings.Join(onlyFmt, messageJoin)
	e.EchoPublic(message, onlyMetadata...)
	return nil
}

func StreamerQueue(e Event) error {
	// Get queue from streamer
	songQueue, err := e.Bot.Streamer.Queue(e.Context())
	if err != nil {
		return err
	}

	// If the queue is empty then we're done
	if len(songQueue) == 0 {
		return NewPublicError(nil, "No queue at the moment")
	}

	// Calculate playback time for the queue
	var totalQueueTime time.Duration
	for _, song := range songQueue {
		totalQueueTime += song.Length
	}

	// Define the message strings
	message := "{green}Queue (/r/ time: %s):{clear}"
	messageJoin := "{red}|{clear}"

	// Grab metadata and set color green if requestable
	onlyFmt := make([]string, len(songQueue))
	onlyMetadata := make([]interface{}, len(songQueue))
	for i, song := range songQueue {
		if song.IsUserRequest {
			onlyFmt[i] = "{green} %s {clear}"
		} else {
			onlyFmt[i] = " %s "
		}
		onlyMetadata[i] = song.Metadata
	}

	// Add information to message
	message = message + strings.Join(onlyFmt, messageJoin)

	// Create the args
	args := append(
		[]interface{}{FormatPlaybackDurationHours(totalQueueTime)},
		onlyMetadata...,
	)

	// Echo out the message
	e.EchoPublic(message, args...)

	// All done!
	return nil
}

func StreamerQueueLength(e Event) error {
	// Define echo message
	message := "There are %d requests (%s), %d randoms (%s), total of %d songs (%s)"

	// Get queue from streamer
	songQueue, err := e.Bot.Streamer.Queue(e.Context())
	if err != nil {
		return err
	}

	// If the queue is empty then we're done
	if len(songQueue) == 0 {
		return NewPublicError(nil, "No queue at the moment")
	}

	// Calculate the total queue time, request time, and request count
	var totalQueueTime time.Duration
	var totalReqTime time.Duration
	reqCount := 0
	for _, song := range songQueue {
		if song.IsUserRequest {
			totalReqTime += song.Length
			reqCount += 1
		}
		totalQueueTime += song.Length
	}

	// Calculate the total count
	totalCount := len(songQueue)

	// Calculate random count and time
	totalRandTime := totalQueueTime - totalReqTime
	randCount := totalCount - reqCount

	// Echo the message
	e.EchoPublic(message,
		reqCount,
		FormatPlaybackDurationHours(totalReqTime),
		randCount,
		FormatPlaybackDurationHours(totalRandTime),
		totalCount,
		FormatPlaybackDurationHours(totalQueueTime),
	)

	// All done!
	return nil
}

var reOtherTopicBit = regexp.MustCompile(`(.*?r/.*/dio.*?)(\|.*?\|)(.*)`)

func StreamerUserInfo(e Event) error {
	name := e.Arguments["DJ"]
	if name == "" || !HasAccess(e.Client, e.Event) {
		// simple path with no argument or no access
		status, err := e.Bot.Manager.Status(e.Context())
		if err != nil {
			return err
		}
		e.EchoPublic("Current DJ: {green}%s", status.StreamerName)
		return nil
	}

	channel := e.Client.LookupChannel(e.Params[0])
	if channel == nil {
		return nil
	}

	var err error
	var user *radio.User
	var topicStatus = "UP"

	// skip the name lookup if the name is None, since it means we are down and out
	if name != "None" {
		user, err = database.LookupNickname(e.Database(), name)
		if err != nil {
			return err
		}
	} else {
		topicStatus = "DOWN"
		user = &radio.User{}
	}

	err = e.Bot.Manager.UpdateUser(e.Context(), name, *user)
	if err != nil {
		return err
	}

	// parse the topic so we can change it
	match := reOtherTopicBit.FindStringSubmatch(channel.Topic)
	if len(match) < 4 {
		return NewPublicError(nil, "Topic format is broken")
	}
	// we get a []string back with all our groups, the first is the full match
	// which we don't need
	match = match[1:]
	// now the group we're interested in is the second one, so replace that with
	// our new status print
	match[1] = Fmt(
		"|{orange} Stream:{red} %s {orange}DJ:{red} %s {cyan} https://r-a-d.io {clear}|",
		topicStatus, name,
	)

	newTopic := strings.Join(match, "")
	e.Client.Cmd.Topic(channel.Name, newTopic)
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

	if thread != "" && HasStreamAccess(e.Client, e.Event) {
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
		match := reTopicBit.FindStringSubmatch(channel.Topic)
		// regexp returns the full match as the first element, so we get rid of it
		match = match[1:]
		// now we replace the relevant bits between the forward slashes
		match[1] = Fmt("%s{orange}", newTopic)
		// and now we can just merge them back together
		newTopic = strings.Join(match, "")

		e.Client.Cmd.Topic(channel.Name, newTopic)
		return nil
	}

	// no access, or just want to know what the topic currently is
	e.EchoPublic("Topic: %s", channel.Topic)
	return nil
}

func KillStreamer(e Event) error {
	if !HasStreamAccess(e.Client, e.Event) {
		return nil
	}

	var quickErr = make(chan error, 1)
	go func() {
		// we call this async with a fairly long timeout, most songs on the streamer
		// should be shorter than the timeout given here. Don't use the event context
		// since it has a very short timeout
		ctx, cancel := context.WithTimeout(context.Background(), time.Minute*15)
		defer cancel()
		// TODO: not everyone should be able to force kill
		err := e.Bot.Streamer.Stop(ctx, e.Arguments.Bool("force"))
		if err != nil {
			quickErr <- err
			return
		}
		quickErr <- nil
		e.EchoPublic("I've stopped streaming now %s!", e.Source.Name)
	}()

	select {
	case err := <-quickErr:
		if err != nil {
			return err
		}
	case <-time.After(time.Second):
	case <-e.Context().Done():
	}

	status, err := e.Bot.Manager.Status(e.Context())
	if err != nil {
		e.EchoPublic("Disconnecting after the current song")
	} else {
		e.EchoPublic("Disconnecting in about %s",
			FormatLongDuration(time.Until(status.SongInfo.End)),
		)
	}

	return nil
}

func RandomTrackRequest(e Event) error {
	var songs []radio.Song
	var err error

	// we select random song from a specific set of songs, either:
	// - favorites of the caller
	// - favorites of the nickname given
	// - from the search result of the query
	// - purely random from all songs
	isFave := e.Arguments.Bool("isFave")
	query := e.Arguments["Query"]

	// see where we need to get our songs from
	if isFave {
		// favorite list random
		nickname := e.Source.Name
		// check if we have a nickname argument to use the list of instead
		if nick := e.Arguments["Nick"]; nick != "" {
			nickname = nick
		}

		songs, err = database.GetSongFavoritesOf(e.Database(), nickname)
		if err != nil {
			return err
		}
	} else if query != "" {
		// query random, select of top 100 results
		songs, err = e.Bot.Searcher.Search(e.Context(), query, 100, 0)
		if err != nil {
			return err
		}
		if len(songs) == 0 {
			return NewUserError(nil, "Your search returned no results")
		}
	} else {
		// purely random, just select from all tracks
		songs, err = database.AllTracks(e.Database())
		if err != nil {
			return err
		}
	}

	// select songs randomly of what we have
	for len(songs) > 0 {
		n := rand.Intn(len(songs))
		song := songs[n]
		// swap our last element with the one we selected to remove it from the list
		songs[n] = songs[len(songs)-1]
		songs = songs[:len(songs)-1]

		// can the song be requested
		if !song.Requestable() {
			continue
		}

		// try requesting the song
		err = e.Bot.Streamer.RequestSong(e.Context(), song, e.Source.Host)
		if err == nil {
			// finished and requested a song successfully
			return nil
		}

		// if not a cooldown error we can stop early too
		if !radio.IsCooldownError(err) {
			return err
		}

		// if user cooldown the user isn't allowed to request yet
		if radio.IsUserCooldownError(err) {
			return generateFriendlyCooldownError(err)
		}
	}

	return NewUserError(nil, "None of the songs found could be requested")
}

func LuckyTrackRequest(e Event) error {
	query := e.Arguments["Query"]
	if query == "" {
		return nil
	}

	res, err := e.Bot.Searcher.Search(e.Context(), query, 100, 0)
	if err != nil {
		return err
	}
	if len(res) == 0 {
		return NewUserError(nil, "Your search returned no results")
	}

	for _, song := range res {
		if !song.Requestable() {
			continue
		}

		err = e.Bot.Streamer.RequestSong(e.Context(), song, e.Source.Host)
		if err == nil {
			return nil
		}

		// if it's not a cooldown error it's some other error we can't handle so return
		if !radio.IsCooldownError(err) {
			return err
		}

		// if user cooldown the user isn't allowed to request yet
		if radio.IsUserCooldownError(err) {
			return generateFriendlyCooldownError(err)
		}
	}

	return NewUserError(nil, "None of the songs found could be requested.")
}

func SearchTrack(e Event) error {
	var songs []radio.Song
	var err error

	if e.Arguments.Bool("TrackID") {
		song, err := e.ArgumentTrack("TrackID")
		if err != nil {
			return err
		}
		songs = []radio.Song{*song}
	} else {
		query := e.Arguments["Query"]
		songs, err = e.Bot.Searcher.Search(e.Context(), query, 5, 0)
		if err != nil {
			return err
		}
	}

	if len(songs) == 0 {
		return NewUserError(nil, "Your search returned no results")
	}

	var (
		// setup formatting strings
		requestableColor   = "{green}"
		unrequestableColor = "{red}"
		format             = "%s {green}(%d) {clear}(LP:{brown}%s{clear})"
		// setup message and argument list for later
		message = make([]string, 0, 10)
		args    = make([]interface{}, 0, 15)
	)
	// loop over our songs and append to our args and message
	for _, song := range songs {
		// check if song is requestable to change the color
		if song.Requestable() {
			message = append(message, requestableColor+format)
		} else {
			message = append(message, unrequestableColor+format)
		}

		var lastPlayed = "Never"
		if !song.LastPlayed.IsZero() {
			// we have three different format limits here due to restricted space,
			// either month/day, day/hour or hour/minutes
			diff := time.Since(song.LastPlayed)
			if diff < time.Hour*24 {
				lastPlayed = FormatDuration(diff, time.Minute)
			} else if diff < time.Hour*24*30 {
				lastPlayed = FormatDuration(diff, time.Hour)
			} else {
				lastPlayed = FormatDuration(diff, time.Hour*24)
			}
		}

		args = append(args, song.Metadata, song.TrackID, lastPlayed)
	}

	e.Echo(strings.Join(message, " | "), args...)
	return nil
}

func RequestTrack(e Event) error {
	song, err := e.ArgumentTrack("TrackID")
	if err != nil {
		return err
	}

	err = e.Bot.Streamer.RequestSong(e.Context(), *song, e.Source.Host)
	if err != nil {
		return generateFriendlyCooldownError(err)
	}

	return nil
}

// generate a friendlier and coloured error message for cooldown related errors,
// returns the original error if it's not of type radio.SongRequestError
func generateFriendlyCooldownError(err error) error {
	srerr, ok := err.(radio.SongRequestError)
	if !ok {
		return err
	}

	var message string
	// first check if a user cooldown was triggered
	switch d := srerr.UserDelay; {
	case d == 0:
		break
	case d < time.Minute*10:
		message = "{green}Only less than ten minutes before you can request again!"
	case d < time.Minute*30:
		message = "{blue}You need to wait at most another half hour until you can request!"
	case d < time.Minute*61:
		message = "{brown}You still have quite a lot of time before you can request again..."
	}
	if message != "" {
		srerr.UserMessage = Fmt(message)
		return err
	}
	switch d := srerr.SongDelay; {
	case d == 0:
		break
	case d < time.Minute*5:
		message = "{green}Only five more minutes before I'll let you request that!"
	case d < time.Minute*15:
		message = "{green}Just another 15 minutes to go for that song!"
	case d < time.Minute*40:
		message = "{blue}Only less than 40 minutes to go for that song!"
	case d < time.Hour:
		message = "{blue}You need to wait at most an hour for that song!"
	case d < time.Hour*4:
		message = "{blue}That song can be requested in a few hours!"
	case d < time.Hour*24:
		message = "{brown}You'll have to wait at most a day for that song..."
	case d < time.Hour*24*3:
		message = "{brown}That song can only be requested in a few days' time..."
	case d < time.Hour*24*7:
		message = "{brown}You might want to go do something else while you wait for that song."
	default:
		message = "{red}No."
	}

	if message != "" {
		srerr.UserMessage = Fmt(message)
	}
	return srerr
}

func LastRequestInfo(e Event) error {
	message := "%s last requested at {red}%s {clear}, which is {red}%s{clear} ago."

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

	var name = "You"
	if withArgument {
		name = e.Arguments["Nick"]
	}

	args := []interface{}{
		name,
		t.Format("Jan 02, 15:04:05"),
		FormatDuration(time.Since(t).Truncate(time.Second), time.Second),
	}

	// calculate if enough time has passed since the last request
	canRequest := time.Since(t) >= time.Duration(e.Bot.Conf().UserRequestDelay)
	if canRequest {
		message += " {green}%s can request!"
		args = append(args, name)
	}

	e.Echo(message, args...)
	return nil
}

func TrackInfo(e Event) error {
	if !HasAccess(e.Client, e.Event) {
		return nil
	}

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
