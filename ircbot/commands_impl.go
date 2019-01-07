package ircbot

import (
	"time"

	"github.com/R-a-dio/valkyrie/rpc/manager"
)

func nowPlaying(echo RespondPublic, status *manager.StatusResponse, track CurrentTrack) CommandFn {
	return func() error {
		message := "Now playing:{red} '%s' {clear}[%s/%s](%s), %s, %s, {red}LP:{clear} %s"

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

		echo(message,
			status.Song.Metadata,
			FormatPlaybackDuration(songPosition), FormatPlaybackDuration(songLength),
			Pluralf("%d listeners", status.ListenerInfo.Listeners),
			Pluralf("%d faves", favoriteCount),
			Pluralf("played %d times", playedCount),
			FormatLongDuration(lastPlayedDiff),
		)

		return nil
	}
}

func lastPlayed() CommandFn          { return nil }
func streamerQueue() CommandFn       { return nil }
func streamerQueueLength() CommandFn { return nil }
func streamerUserInfo() CommandFn    { return nil }
func faveTrack() CommandFn           { return nil }
func faveList() CommandFn            { return nil }
func threadURL() CommandFn           { return nil }
func channelTopic() CommandFn        { return nil }
func killStreamer() CommandFn        { return nil }
func randomTrackRequest() CommandFn  { return nil }
func luckyTrackRequest() CommandFn   { return nil }
func searchTrack() CommandFn         { return nil }
func requestTrack() CommandFn        { return nil }
func lastRequestInfo() CommandFn     { return nil }
func trackInfo() CommandFn           { return nil }

func trackTags(echo Respond, track ArgumentOrCurrentTrack) CommandFn {
	return func() error {
		message := "{clear}Title: {red}%s {clear}Album: {red}%s {clear}Faves: {red}%d {clear}Plays: {red}%d {clear}Tags: {red}%s"

		var favoriteCount int64
		var playedCount int64

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
