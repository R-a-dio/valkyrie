package rpc

import (
	reflect "reflect"
	"testing"
	"time"

	radio "github.com/R-a-dio/valkyrie"
	"github.com/leanovate/gopter"
	"github.com/leanovate/gopter/arbitrary"
	"github.com/leanovate/gopter/gen"
	"github.com/stretchr/testify/assert"
)

func TestRoundtrip(tt *testing.T) {
	a := arbitrary.DefaultArbitraries()
	// special generator for users since we don't actually pass all the fields
	// over the wire for reasons
	userGen := gen.Struct(reflect.TypeFor[radio.User](), map[string]gopter.Gen{
		"ID":              a.GenForType(reflect.TypeFor[radio.UserID]()).SuchThat(func(v radio.UserID) bool { return v > 0 }),
		"Username":        gen.AnyString(),
		"IP":              gen.AnyString(),
		"UpdatedAt":       a.GenForType(reflect.TypeFor[*time.Time]()),
		"DeletedAt":       a.GenForType(reflect.TypeFor[*time.Time]()),
		"CreatedAt":       a.GenForType(reflect.TypeFor[time.Time]()),
		"DJ":              a.GenForType(reflect.TypeFor[radio.DJ]()),
		"UserPermissions": a.GenForType(reflect.TypeFor[radio.UserPermissions]()),
	})
	a.RegisterGen(userGen)
	a.RegisterGen(gen.PtrOf(userGen))

	// trackGen to make sure the trackid>0 because the code checks for that as
	// indication of a track existing
	trackGen := a.GenForType(reflect.TypeFor[radio.DatabaseTrack]()).SuchThat(func(a radio.DatabaseTrack) bool {
		return a.TrackID > 0
	})
	a.RegisterGen(trackGen)
	a.RegisterGen(gen.PtrOf(trackGen))

	p := gopter.NewProperties(nil)

	p.Property("timestamp", a.ForAll(func(in time.Time) bool {
		out := t(tp(in))

		return in.Equal(out)
	}))
	p.Property("timestamp-pointer", a.ForAll(func(in *time.Time) bool {
		out := ptrt(ptrtp(in))
		if in == nil {
			return out == nil
		}

		return in.Equal(*out)
	}))
	p.Property("duration", a.ForAll(func(in time.Duration) bool {
		out := d(dp(in))

		return in == out
	}))
	p.Property("status", a.ForAll(func(in radio.Status) bool {
		out := fromProtoStatus(toProtoStatus(in))

		return assert.EqualExportedValues(tt, in, out)
	}))
	p.Property("song", a.ForAll(func(in radio.Song) bool {
		out := fromProtoSong(toProtoSong(in))

		return assert.EqualExportedValues(tt, in, out)
	}))
	p.Property("song-info", a.ForAll(func(in radio.SongInfo) bool {
		out := fromProtoSongInfo(toProtoSongInfo(in))

		return assert.EqualExportedValues(tt, in, out)
	}))
	p.Property("song-update", a.ForAll(func(in *radio.SongUpdate) bool {
		out := fromProtoSongUpdate(toProtoSongUpdate(in))

		return assert.EqualExportedValues(tt, in, out)
	}))
	p.Property("queue-entry", a.ForAll(func(in radio.QueueEntry) bool {
		out := fromProtoQueueEntry(toProtoQueueEntry(in))

		return assert.EqualExportedValues(tt, in, out)
	}))
	p.Property("queue-id", a.ForAll(func(in radio.QueueID) bool {
		out := fromProtoQueueID(toProtoQueueID(in))

		return assert.EqualExportedValues(tt, in, out)
	}))
	p.Property("user", a.ForAll(func(in *radio.User) bool {
		out := fromProtoUser(toProtoUser(in))

		return assert.EqualExportedValues(tt, in, out)
	}))
	p.Property("user-permissions", a.ForAll(func(in radio.UserPermissions) bool {
		out := fromProtoUserPermissions(toProtoUserPermissions(in))

		if len(in) == 0 {
			return assert.Len(tt, out, 0)
		} else {
			return assert.Equal(tt, in, out)
		}
	}))
	p.Property("dj", a.ForAll(func(in radio.DJ) bool {
		out := fromProtoDJ(toProtoDJ(in))

		return assert.EqualExportedValues(tt, in, out)
	}))
	p.Property("listener", a.ForAll(func(in radio.Listener) bool {
		out := fromProtoListener(toProtoListener(in))

		return assert.EqualExportedValues(tt, in, out)
	}))

	toAndFrom(tt, p, a, "proxy-metadata-event", toProtoProxyMetadataEvent, fromProtoProxyMetadataEvent)
	toAndFrom(tt, p, a, "proxy-source-event", toProtoProxySourceEvent, fromProtoProxySourceEvent)

	p.TestingRun(tt)
}

func toAndFrom[A any, B any](t *testing.T, p *gopter.Properties, a *arbitrary.Arbitraries, name string, to func(A) B, from func(B) A) {
	p.Property(name, a.ForAll(func(in A) bool {
		out := from(to(in))
		return assert.EqualExportedValues(t, in, out)
	}))
}
