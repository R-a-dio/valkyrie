package ircbot

import (
	"context"
	"time"

	"github.com/R-a-dio/valkyrie/database"
	"github.com/R-a-dio/valkyrie/rpc/manager"
	"github.com/R-a-dio/valkyrie/rpc/streamer"
	"github.com/lrstanley/girc"
)

func nowPlaying(echo RespondPublic, status *manager.StatusResponse, db database.Handler, track CurrentTrack) CommandFn {
	return func() error {
		message := "Now playing:{red} '%s' {clear}[%s/%s](%s), %s, %s, {green}LP:{clear} %s"

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

		favoriteCount, _ := track.FaveCount(db)
		playedCount, _ := track.PlayedCount(db)

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

func killStreamer(s streamer.Streamer, a Arguments) CommandFn {
	return func() error {
		// TODO: implement authorization
		req := &streamer.StopRequest{
			ForceStop: a.Bool("force"),
		}

		_, err := s.Stop(context.TODO(), req)
		return err
	}
}

func randomTrackRequest() CommandFn { return nil }
func luckyTrackRequest() CommandFn  { return nil }
func searchTrack() CommandFn        { return nil }

func requestTrack(echo Respond, s streamer.Streamer, e girc.Event, track ArgumentTrack) CommandFn {
	return func() error {
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
	}
}

func lastRequestInfo() CommandFn { return nil }
func trackInfo() CommandFn       { return nil }

func trackTags(echo Respond, db database.Handler, track ArgumentOrCurrentTrack) CommandFn {
	return func() error {
		message := "{clear}Title: {red}%s {clear}Album: {red}%s {clear}Faves: {red}%d {clear}Plays: {red}%d {clear}Tags: {red}%s"

		favoriteCount, _ := track.FaveCount(db)
		playedCount, _ := track.PlayedCount(db)

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
