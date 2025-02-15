package ircbot

import (
	"context"
	"math/rand/v2"
	"regexp"
	"strings"
	"time"

	radio "github.com/R-a-dio/valkyrie"
	"github.com/R-a-dio/valkyrie/errors"
	"github.com/rs/zerolog"
)

func NowPlaying(e Event) error {
	const op errors.Op = "irc/NowPlaying"

	// there is very similar looking code located in api.go under AnnounceSong, so if
	// you introduce a change here you might want to see if that change is also required
	// in the announcement code
	message := "Now playing:{red} '%s' {clear}[%s/%s](%s), %s, %s, {green}LP:{clear} %s"

	status := e.Bot.StatusValue.Latest()

	if e.Bot.UserValue.Latest() == nil {
		e.EchoPublic("Stream is currently down.")
		return nil
	}

	ss := e.Storage.Song(e.Ctx)
	// status returns a bare song; so refresh it from the db
	song, err := ss.FromMetadata(status.Song.Metadata)
	if err != nil {
		return errors.E(op, err)
	}

	var lastPlayedDiff time.Duration
	if !song.LastPlayed.IsZero() {
		lastPlayedDiff = time.Since(song.LastPlayed)
	}

	songPosition := time.Since(status.SongInfo.Start)
	songLength := status.Song.Length

	favoriteCount, err := ss.FavoriteCount(*song)
	if err != nil {
		return errors.E(op, err)
	}
	playedCount, err := ss.PlayedCount(*song)
	if err != nil {
		return errors.E(op, err)
	}

	e.EchoPublic(message,
		status.Song.Metadata,
		FormatPlaybackDuration(songPosition), FormatPlaybackDuration(songLength),
		Pluralf("%d listeners", e.Bot.ListenersValue.Latest()),
		Pluralf("%d faves", favoriteCount),
		Pluralf("played %d times", playedCount),
		FormatLongDuration(lastPlayedDiff),
	)

	return nil
}

func LastPlayed(e Event) error {
	const op errors.Op = "irc/LastPlayed"

	songs, err := e.Storage.Song(e.Ctx).LastPlayed(radio.LPKeyLast, 5)
	if err != nil {
		return errors.E(op, err)
	}

	message := "{green}Last Played:{clear}"
	messageJoin := "{green}|{clear}"
	onlyFmt := make([]string, len(songs))
	onlyMetadata := make([]any, len(songs))
	for i, song := range songs {
		onlyFmt[i] = " %s "
		onlyMetadata[i] = song.Metadata
	}

	message = message + strings.Join(onlyFmt, messageJoin)
	e.EchoPublic(message, onlyMetadata...)
	return nil
}

func StreamerQueue(e Event) error {
	const op errors.Op = "irc/StreamerQueue"

	// Get queue
	songQueue, err := e.Bot.Queue.Entries(e.Ctx)
	if err != nil {
		return errors.E(op, err)
	}

	// If the queue is empty then we're done
	if len(songQueue) == 0 {
		e.EchoPublic("No queue at the moment")
		return nil
	}

	// Calculate playback time for the queue
	var totalQueueTime time.Duration
	for _, song := range songQueue {
		totalQueueTime += song.Length
	}

	// limit the length that we're printing
	songQueue = songQueue.Limit(5)

	// Define the message strings
	message := "{green}Queue (/r/ time: %s):{clear}"
	messageJoin := "{red}|{clear}"

	// Grab metadata and set color green if requestable
	onlyFmt := make([]string, len(songQueue))
	onlyMetadata := make([]any, len(songQueue))
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
		[]any{FormatPlaybackDurationHours(totalQueueTime)},
		onlyMetadata...,
	)

	// Echo out the message
	e.EchoPublic(message, args...)

	// All done!
	return nil
}

func StreamerQueueLength(e Event) error {
	const op errors.Op = "irc/StreamerQueueLength"

	// Define echo message
	message := "There are %d requests (%s), %d randoms (%s), total of %d songs (%s)"

	// Get queue from streamer
	songQueue, err := e.Bot.Queue.Entries(e.Ctx)
	if err != nil {
		return errors.E(op, err)
	}

	// If the queue is empty then we're done
	if len(songQueue) == 0 {
		e.EchoPublic("No queue at the moment")
		return nil
	}

	// Calculate the total queue time, request time, and request count
	var totalQueueTime time.Duration
	var totalReqTime time.Duration
	reqCount := 0
	for _, song := range songQueue {
		if song.IsUserRequest {
			totalReqTime += song.Length
			reqCount++
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

func StreamerUserInfo(e Event) error {
	const op errors.Op = "irc/StreamerUserInfo"

	name := e.Arguments["DJ"]
	if name == "" || !e.HasStreamAccess(radio.GuestNone) {
		// simple path if people are just asking for the current dj
		status := e.Bot.StatusValue.Latest()
		e.EchoPublic("Current DJ: {green}%s", status.StreamerName)
		return nil
	}

	us := e.Storage.User(e.Ctx)

	// guest handling
	if e.Arguments.Bool("isGuest") {
		user, err := us.Get("guest")
		if err != nil {
			return errors.E(op, err)
		}
		user.DJ.Name = name
		_, err = us.Update(*user)
		if err != nil {
			return errors.E(op, err)
		}
		err = e.Bot.Manager.UpdateFromStorage(e.Ctx)
		if err != nil {
			return errors.E(op, err)
		}
		return nil
	}

	// non-guest handling, though we currently only do a lookup to see
	// if the user given is a robot
	user, err := us.LookupName(name)
	if err != nil {
		if errors.Is(errors.UserUnknown, err) {
			e.EchoPrivate("Unknown DJ: if you're trying to set a DJ that isn't hanyuu don't bother")
			return nil
		}
		return errors.E(op, err)
	}

	// user given isn't a robot, so all thats left to do is print the
	// current dj instead
	if !radio.IsRobot(*user) {
		e.EchoPublic("Current DJ: {green}%s", e.Bot.StatusValue.Latest().StreamerName)
		return nil
	}

	// otherwise we should only care if the user is the one we are aware of
	// but for now we only have one robot ever so just assume thats the value
	// TODO: make this work with multiple streamers
	err = e.Bot.Streamer.Start(e.Ctx)
	if err != nil {
		return err
	}

	e.EchoPrivate("Hanyuu-sama has been awakened, drop stream before 1 minute has passed please")
	return nil
}

func FaveTrack(e Event) error {
	const op errors.Op = "irc/FaveTrack"

	var song radio.Song

	ss := e.Storage.Song(e.Ctx)
	if e.Arguments.Bool("relative") {
		// for when `last` is given as argument

		// count the amount of `last`'s used to determine how far back we should go
		index := strings.Count(e.Arguments["relative"], "last") - 1

		key := radio.LPKeyLast
		if index > 0 {
			// if our index is higher than 0 we need to lookup the key for that
			_, next, err := ss.LastPlayedPagination(radio.LPKeyLast, 1, 50)
			if err != nil {
				return errors.E(op, err)
			}
			// make sure our index exists in the list
			if index >= len(next) {
				return errors.E(op, "index too far")
			}

			key = next[index]
		}

		songs, err := ss.LastPlayed(key, 1)
		if err != nil {
			return errors.E(op, err)
		}
		song = songs[0]
	} else if e.Arguments.Bool("TrackID") {
		// for when a track number is given as argument
		s, err := e.ArgumentTrack("TrackID")
		if err != nil {
			if errors.Is(errors.SongUnknown, err) {
				// song doesn't exist with that trackid
				e.EchoPrivate("I don't know of a song with that ID...")
				return nil
			}
			return errors.E(op, err)
		}
		song = *s
	} else {
		// for when there is no argument given
		s, err := e.CurrentTrack()
		if err != nil {
			return errors.E(op, err)
		}
		song = *s
	}

	// now check to see if we want to favorite or unfavorite something
	var dbFunc = ss.AddFavorite
	if e.Arguments.Bool("isNegative") {
		dbFunc = ss.RemoveFavorite
	}

	changed, err := dbFunc(song, e.Source.Name)
	if err != nil {
		return errors.E(op, err)
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
	const op errors.Op = "irc/ThreadURL"

	thread := e.Arguments["thread"]
	thread = strings.TrimSpace(thread)

	if thread != "" && e.HasStreamAccess(radio.GuestThread) {
		err := e.Bot.Manager.UpdateThread(e.Ctx, thread)
		if err != nil {
			return errors.E(op, err)
		}
	} else {
		thread = e.Bot.StatusValue.Latest().Thread
		e.EchoPublic("Thread: %s", thread)
	}

	return nil
}

var reTopicBit = regexp.MustCompile("(.*?r/)(.*)(/dio.*?)(.*)")

func ChannelTopic(e Event) error {
	const op errors.Op = "irc/ChannelTopic"

	channel := e.Client.LookupChannel(e.Params[0])
	if channel == nil {
		zerolog.Ctx(e.Ctx).Warn().Str("command", "topic").Msg("nil channel")
		return nil
	}

	newTopic := e.Arguments["topic"]
	if newTopic != "" && e.HasAccess() {
		// we want to set the topic and have access for it
		match := reTopicBit.FindStringSubmatch(channel.Topic)
		if match == nil || len(match) < 2 {
			return errors.E(op, errors.BrokenTopic, errors.Info(channel.Topic))
		}
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
	const op errors.Op = "irc/KillStreamer"

	if !e.HasStreamAccess(radio.GuestKill) {
		return nil
	}

	force := e.Arguments.Bool("force")
	if force {
		// check if the user has the authorization to use force
		ok, err := e.HasDeveloperAccess()
		if err != nil {
			return errors.E(op, err)
		}
		force = ok
	}

	var quickErr = make(chan error, 1)
	go func() {
		// we call this async with a fairly long timeout, most songs on the streamer
		// should be shorter than the timeout given here. Don't use the event context
		// since it has a very short timeout
		ctx, cancel := context.WithTimeout(context.Background(), time.Minute*15)
		defer cancel()

		err := e.Bot.Streamer.Stop(ctx, force)
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
			return errors.E(op, err)
		}
	case <-time.After(time.Second):
	case <-e.Ctx.Done():
	}

	status := e.Bot.StatusValue.Latest()

	until := time.Until(status.SongInfo.End)
	if force {
		e.EchoPublic("Disconnecting right now")
	} else if until <= 0 {
		e.EchoPublic("Disconnecting after the current song")
	} else {
		e.EchoPublic("Disconnecting in about %s",
			FormatLongDuration(until),
		)
	}

	return nil
}

func RandomTrackRequest(e Event) error {
	const op errors.Op = "irc/RandomTrackRequest"

	var songs []radio.Song
	var err error
	var limit = 100

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

		songs, err = e.Storage.Track(e.Ctx).RandomFavoriteOf(nickname, limit)
		if err != nil {
			return errors.E(op, err)
		}
	} else if query != "" {
		// query random, select of top 100 results
		var res *radio.SearchResult
		res, err = e.Bot.Searcher.Search(e.Ctx, query, 100, 0)
		if err != nil {
			return errors.E(op, err)
		}
		songs = res.Songs
	} else {
		// purely random, just select from all tracks
		songs, err = e.Storage.Track(e.Ctx).Random(limit)
		if err != nil {
			return errors.E(op, err)
		}
	}

	if len(songs) == 0 {
		e.Echo("no songs were found")
		return nil
	}

	// select songs randomly of what we have
	for len(songs) > 0 {
		n := rand.IntN(len(songs))
		song := songs[n]
		// swap our last element with the one we selected to remove it from the list
		songs[n] = songs[len(songs)-1]
		songs = songs[:len(songs)-1]

		// can the song be requested
		if !song.Requestable() {
			continue
		}

		// try requesting the song
		err = e.Bot.Streamer.RequestSong(e.Ctx, song, e.Source.Host)
		if err == nil {
			// finished and requested a song successfully
			return nil
		}

		// the only error that isn't fatal here is a SongCooldown one
		if !errors.Is(errors.SongCooldown, err) {
			// UserCooldown and StreamerNoRequests are handled by the parent handler
			return errors.E(op, err)
		}
	}

	e.Echo("none of the songs found could be requested")
	return nil
}

func LuckyTrackRequest(e Event) error {
	const op errors.Op = "irc/LuckyTrackRequest"

	query := e.Arguments["Query"]
	if query == "" {
		return nil
	}

	res, err := e.Bot.Searcher.Search(e.Ctx, query, 100, 0)
	if err != nil {
		return errors.E(op, err)
	}

	for _, song := range res.Songs {
		if !song.Requestable() {
			continue
		}

		err = e.Bot.Streamer.RequestSong(e.Ctx, song, e.Source.Host)
		if err == nil {
			// finished and requested a song successfully
			return nil
		}

		// if user cooldown the user isn't allowed to request yet
		if errors.Is(errors.UserCooldown, err) {
			return errors.E(op, err)
		}

		// if not a song cooldown error we received some other error so exit early
		if !errors.Is(errors.SongCooldown, err) {
			return errors.E(op, err)
		}
	}

	e.Echo("None of the songs found could be requested.")
	return nil
}

func SearchTrack(e Event) error {
	const op errors.Op = "irc/SearchTrack"

	var songs []radio.Song
	var err error

	if e.Arguments.Bool("TrackID") {
		song, err := e.ArgumentTrack("TrackID")
		if err != nil {
			return errors.E(op, err)
		}
		songs = []radio.Song{*song}
	} else {
		var res *radio.SearchResult
		query := e.Arguments["Query"]
		res, err = e.Bot.Searcher.Search(e.Ctx, query, 5, 0)
		if err != nil {
			return errors.E(op, err)
		}
		songs = res.Songs
	}

	var (
		// setup formatting strings
		requestableColor   = "{green}"
		unrequestableColor = "{red}"
		format             = "%s {green}(%d) {clear}(LP:{brown}%s{clear})"
		// setup message and argument list for later
		message = make([]string, 0, 10)
		args    = make([]any, 0, 15)
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
	const op errors.Op = "irc/RequestTrack"

	song, err := e.ArgumentTrack("TrackID")
	if err != nil {
		return errors.E(op, err)
	}

	err = e.Bot.Streamer.RequestSong(e.Ctx, *song, e.Source.Host)
	if err != nil {
		return errors.E(op, err)
	}

	return nil
}

// MessageFromError returns a friendlier, coloured error message for errors
func MessageFromError(err error) string {
	switch {
	case errors.Is(errors.UserCooldown, err):
		return userCooldownMessageFromError(err)
	case errors.Is(errors.SongCooldown, err):
		return songCooldownMessageFromError(err)
	case errors.Is(errors.StreamerNoRequests, err):
		return "{brown}The streamer is currently not taking requests."
	case errors.Is(errors.SearchNoResults, err):
		return "Your search returned no results"
	case errors.Is(errors.UserUnknown, err):
		return "No such user exists"
	case errors.Is(errors.SongUnknown, err):
		return "No such song exists"
	}
	return ""
}

func userCooldownMessageFromError(err error) string {
	d, ok := errors.SelectDelay(err)
	if !ok {
		return "{green}No cooldown found."
	}

	switch d := time.Duration(d); {
	case d < time.Minute*10:
		return "{green}Only less than ten minutes before you can request again!"
	case d < time.Minute*30:
		return "{blue}You need to wait at most another half hour until you can request!"
	case d < time.Minute*61:
		return "{brown}You still have quite a lot of time before you can request again..."
	default:
		return "{red}No."
	}
}

func songCooldownMessageFromError(err error) string {
	d, ok := errors.SelectDelay(err)
	if !ok {
		return "{green}No cooldown found."
	}

	switch d := time.Duration(d); {
	case d < time.Minute*5:
		return "{green}Only five more minutes before I'll let you request that!"
	case d < time.Minute*15:
		return "{green}Just another 15 minutes to go for that song!"
	case d < time.Minute*40:
		return "{blue}Only less than 40 minutes to go for that song!"
	case d < time.Hour:
		return "{blue}You need to wait at most an hour for that song!"
	case d < time.Hour*4:
		return "{blue}That song can be requested in a few hours!"
	case d < time.Hour*24:
		return "{brown}You'll have to wait at most a day for that song..."
	case d < time.Hour*24*3:
		return "{brown}That song can only be requested in a few days' time..."
	case d < time.Hour*24*7:
		return "{brown}You might want to go do something else while you wait for that song."
	default:
		return "{red}No."
	}
}

func LastRequestInfo(e Event) error {
	const op errors.Op = "irc/LastRequestInfo"

	message := "%s last requested at {red}%s {clear}, which is {red}%s{clear} ago."

	var host = e.Source.Host
	var withArgument bool
	if nick := e.Arguments["Nick"]; nick != "" {
		u := e.Client.LookupUser(nick)
		if u == nil {
			e.EchoPrivate("I don't know who that is")
			return nil
		}

		host = u.Host
		withArgument = true
	}

	t, err := e.Storage.Request(e.Ctx).LastRequest(host)
	if err != nil {
		return errors.E(op, err)
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

	args := []any{
		name,
		t.Format("Jan 02, 15:04:05"),
		FormatDuration(time.Since(t).Truncate(time.Second), time.Second),
	}

	// calculate if enough time has passed since the last request
	_, canRequest := radio.CalculateCooldown(e.Bot.cfgUserRequestDelay(), t)
	if canRequest {
		message += " {green}%s can request!"
		args = append(args, name)
	}

	e.Echo(message, args...)
	return nil
}

func TrackInfo(e Event) error {
	const op errors.Op = "irc/TrackInfo"

	if !e.HasAccess() {
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

	song, err := e.ArgumentTrack("TrackID")
	if err != nil {
		song, err = e.CurrentTrack()
		if err != nil {
			return errors.E(op, err)
		}
	}

	if !song.HasTrack() {
		e.EchoPrivate("Song is not in the database")
		return nil
	}

	ss := e.Storage.Song(e.Ctx)
	favoriteCount, err := ss.FavoriteCount(*song)
	if err != nil {
		return errors.E(op, err)
	}
	playedCount, err := ss.PlayedCount(*song)
	if err != nil {
		return errors.E(op, err)
	}

	// calculate the time remaining until this can be requested again
	var cooldownIndicator = "!"
	{
		compareTime := song.LastPlayed
		if song.LastRequested.After(song.LastPlayed) {
			compareTime = song.LastRequested
		}

		leftover := song.RequestDelay() - time.Since(compareTime)
		if leftover > 0 {
			cooldownIndicator = FormatDuration(leftover, time.Second)
		}
	}

	e.Echo(message,
		song.TrackID,
		song.Metadata,
		favoriteCount,
		playedCount,
		song.RequestCount,
		song.Priority,
		FormatDuration(song.RequestDelay(), time.Second), cooldownIndicator,
		song.Acceptor,
		song.Tags,
	)

	return nil
}

func TrackTags(e Event) error {
	const op errors.Op = "irc/TrackTags"

	message := "Title: {red}%s {clear}" +
		"Album: {red}%s {clear}" +
		"Faves: {red}%d {clear}" +
		"Plays: {red}%d {clear}" +
		"Tags: {red}%s {clear}"

	song, err := e.ArgumentTrack("TrackID")
	if err != nil {
		song, err = e.CurrentTrack()
		if err != nil {
			return errors.E(op, err)
		}
	}

	var album string
	var tags = "no tags available"
	if song.HasTrack() {
		album, tags = song.Album, song.Tags
	}

	ss := e.Storage.Song(e.Ctx)
	favoriteCount, err := ss.FavoriteCount(*song)
	if err != nil {
		return errors.E(op, err)
	}
	playedCount, err := ss.PlayedCount(*song)
	if err != nil {
		return errors.E(op, err)
	}

	e.Echo(message,
		song.Metadata,
		album,
		favoriteCount,
		playedCount,
		tags,
	)

	return nil
}
