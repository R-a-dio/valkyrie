package ircbot

import (
	"log"
	"regexp"
	"strings"
	"time"

	radio "github.com/R-a-dio/valkyrie"
	"github.com/R-a-dio/valkyrie/database"
	"github.com/lrstanley/girc"
)

// nowPlaying is a tiny layer around nowPlayingImpl to give it a signature the RegexHandler
// expects
func nowPlaying(e Event, echo RespondPublic) CommandFn {
	return func() error {
		fn, err := NowPlayingMessage(e)
		if err != nil {
			return err
		}

		message, args, err := fn()
		if err != nil {
			return err
		}

		echo(message, args...)
		return nil
	}
}

// messageFn is a function that returns a string and arguments ready to be passed to
// any of the fmt.*f functions
type messageFn func() (msg string, args []interface{}, err error)

// nowPlayingMessage implements the internals for both the NowPlaying command and
// Bot.AnnounceSong for the API
func nowPlayingMessage(status radio.Status, db database.Handler, track CurrentTrack) messageFn {
	return func() (string, []interface{}, error) {
		message := "Now playing:{red} '%s' {clear}[%s/%s](%s), %s, %s, {green}LP:{clear} %s"

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

		favoriteCount, _ := database.SongFaveCount(db, track.Song)
		playedCount, _ := database.SongPlayedCount(db, track.Song)

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

func lastPlayed() CommandFn          { return nil }
func streamerQueue() CommandFn       { return nil }
func streamerQueueLength() CommandFn { return nil }
func streamerUserInfo() CommandFn    { return nil }
func faveTrack() CommandFn           { return nil }
func faveList() CommandFn            { return nil }

func threadURL(echo RespondPublic, args Arguments, m radio.ManagerService, op Access) CommandFn {
	return func() error {
		thread := args["thread"]

		if thread != "" && op {
			err := m.UpdateThread(thread)
			if err != nil {
				log.Println(err)
				return nil
			}
		}

		resp, err := m.Status()
		if err != nil {
			log.Println(err)
			return nil
		}

		echo("Thread: %s", resp.Thread)
		return nil
	}
}

var reTopicBit = regexp.MustCompile("(.*?r/)(.*)(/dio.*?)(.*)")

func channelTopic(echo RespondPublic, args Arguments, c *girc.Client, e girc.Event) CommandFn {
	return func() error {
		channel := c.LookupChannel(e.Params[0])
		if channel == nil {
			log.Println("unknown channel in .topic")
			// unknown channel?
			return nil
		}

		newTopic := args["topic"]
		if newTopic != "" && HasAccess(c, e) {
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

			c.Cmd.Topic(channel.Name, newTopic)
			return nil
		}

		// no access, or just want to know what the topic currently is
		echo("Topic: %s", channel.Topic)
		return nil
	}
}

func killStreamer(s radio.StreamerService, a Arguments, op Access) CommandFn {
	return func() error {
		// TODO: this should use special caseing for streamers that don't have channel
		// access
		if !op {
			return nil
		}

		// TODO: not everyone should be able to force kill
		return s.Stop(a.Bool("force"))
	}
}

func randomTrackRequest() CommandFn { return nil }
func luckyTrackRequest() CommandFn  { return nil }
func searchTrack() CommandFn        { return nil }

func requestTrack(s radio.StreamerService) CommandFn {
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

func lastRequestInfo() CommandFn { return nil }
func trackInfo() CommandFn       { return nil }

func trackTags(echo Respond, db database.Handler, track ArgumentOrCurrentTrack) CommandFn {
	return func() error {
		message := "{clear}Title: {red}%s {clear}Album: {red}%s {clear}Faves: {red}%d {clear}Plays: {red}%d {clear}Tags: {red}%s"

		favoriteCount, _ := database.SongFaveCount(db, track.Song)
		playedCount, _ := database.SongPlayedCount(db, track.Song)

		echo(message,
			track.Metadata,
			track.Album,
			favoriteCount,
			playedCount,
			track.Tags,
		)

		return nil
	}
}
