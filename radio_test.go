package radio

import (
	"fmt"
	"reflect"
	"testing"
	"time"
	"unicode"

	"github.com/leanovate/gopter"
	"github.com/leanovate/gopter/arbitrary"
	"github.com/leanovate/gopter/gen"
	"github.com/leanovate/gopter/prop"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/exp/constraints"
)

func TestSongHydrate(t *testing.T) {
	t.Run("song with just metadata", func(t *testing.T) {
		a := Song{
			Metadata: "we test if this gets hydrated",
		}
		a.Hydrate()

		assert.False(t, a.Hash.IsZero(), "Hash should not be zero after hydrate")
		assert.False(t, a.HashLink.IsZero(), "HashLink should not be zero after hydrate")
	})

	t.Run("song with a HashLink already set", func(t *testing.T) {
		a := Song{
			Metadata: "we test if this gets hydrated",
		}
		a.Hydrate()

		b := Song{
			Metadata: "some other metadata",
			HashLink: a.Hash, // set the HashLink to the other song
		}
		// hydrate should now not touch HashLink but still update Hash
		b.Hydrate()
		assert.False(t, b.Hash.IsZero(), "Hash should not be zero after hydrate")
		assert.Equal(t, a.Hash, b.HashLink)
	})

	t.Run("song with no metadata, but does have DatabaseTrack", func(t *testing.T) {
		c := Song{
			DatabaseTrack: &DatabaseTrack{
				Artist: "Hello",
				Title:  "World",
			},
		}
		c.Hydrate()
		assert.NotEmpty(t, c.Metadata)
		assert.False(t, c.Hash.IsZero())
		assert.False(t, c.HashLink.IsZero())
	})

	t.Run("song with no metadata and no DatabaseTrack", func(t *testing.T) {
		a := Song{}
		a.Hydrate()

		assert.Empty(t, a.Metadata, "hydrate should not update Metadata if there is nothing")
		assert.True(t, a.Hash.IsZero(), "hydrate should not update Hash if there is nothing")
		assert.True(t, a.HashLink.IsZero(), "hydrate shoudl not update HashLink if there is nothing")
	})
}

func TestParseTrackID(t *testing.T) {
	testParseAndString(t, ParseTrackID)
}

func TestParsePostPendingID(t *testing.T) {
	testParseAndString(t, ParsePostPendingID)
}

func TestParseSongID(t *testing.T) {
	testParseAndString(t, ParseSongID)
}

func TestParseSubmissionID(t *testing.T) {
	testParseAndString(t, ParseSubmissionID)
}

func TestParseNewsPostID(t *testing.T) {
	testParseAndString(t, ParseNewsPostID)
}

func TestParseNewsCommentID(t *testing.T) {
	testParseAndString(t, ParseNewsCommentID)
}

func TestParseDJID(t *testing.T) {
	testParseAndString(t, ParseDJID)
}

func TestParseUserID(t *testing.T) {
	testParseAndString(t, ParseUserID)
}

func TestParseSongHash(t *testing.T) {
	a := arbitrary.DefaultArbitraries()

	p := gopter.NewProperties(nil)
	// roundtrips should always succeed
	p.Property("roundtrip", a.ForAll(func(in SongHash) bool {
		out, err := ParseSongHash(in.String())
		if err != nil {
			return false
		}
		return in == out
	}))
	p.TestingRun(t)
}

type stringAndComparable interface {
	fmt.Stringer
	constraints.Integer
	comparable
}

func testParseAndString[T stringAndComparable](t *testing.T, parseFn func(string) (T, error)) {
	a := arbitrary.DefaultArbitraries()

	p := gopter.NewProperties(nil)
	// roundtrips should always succeed
	p.Property("roundtrip", a.ForAll(func(in T) bool {
		out, err := parseFn(in.String())
		if err != nil {
			return false
		}
		return in == out
	}))
	// alpha-only should always fail
	p.Property("alpha-only", prop.ForAll(func(in string) bool {
		out, err := parseFn(in)
		return out == 0 && err != nil
	}, gen.AlphaString()))
	p.TestingRun(t)
}

func TestMetadata(t *testing.T) {
	cases := []struct {
		name     string
		artist   string
		title    string
		expected string
	}{
		{
			name:     "simple",
			artist:   "hello",
			title:    "world",
			expected: "hello - world",
		},
		{
			name:     "missing artist",
			artist:   "",
			title:    "hello world",
			expected: "hello world",
		},
		{
			name:     "whitespace at start of artist",
			artist:   "	hello",
			title:    "world",
			expected: "hello - world",
		},
		{
			name:     "whitespace at end of artist",
			artist:   "hello	 ",
			title:    "world",
			expected: "hello - world",
		},
		{
			name:     "whitespace at start of title",
			artist:   "hello",
			title:    "	  world",
			expected: "hello - world",
		},
		{
			name:     "whitespace at end of title",
			artist:   "hello",
			title:    "world	 ",
			expected: "hello - world",
		},
		{
			name:     "whitespace only artist",
			artist:   "	  	",
			title:    "hello world",
			expected: "hello world",
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			res := Metadata(c.artist, c.title)

			assert.Equal(t, c.expected, res)
		})
	}
}

func TestSongEqualTo(t *testing.T) {
	tp := gopter.DefaultTestParameters()
	tp.MinSuccessfulTests = 500
	a := arbitrary.DefaultArbitraries()
	p := gopter.NewProperties(tp)
	ab := arbitrary.DefaultArbitraries()

	// we only want songs with actual string data to compare against
	a.RegisterGen(gen.UnicodeString(unicode.Katakana).SuchThat(func(s string) bool { return len(s) > 5 }))
	a.RegisterGen(ab.GenForType(reflect.TypeOf(&User{})))
	a.RegisterGen(ab.GenForType(reflect.TypeOf(&DatabaseTrack{})))

	p.Property("song a != b",
		a.ForAll(func(a, b Song) bool {
			return !a.EqualTo(b)
		}),
	)
	p.Property("song b != a",
		a.ForAll(func(a, b Song) bool {
			return !b.EqualTo(a)
		}),
	)
	p.Property("song a == a",
		a.ForAll(func(a, b Song) bool {
			return a.EqualTo(a)
		}),
	)
	p.Property("song b == b",
		a.ForAll(func(a, b Song) bool {
			return b.EqualTo(b)
		}),
	)

	p.TestingRun(t)
}

func TestSongRequestable(t *testing.T) {
	tp := gopter.DefaultTestParameters()
	tp.MinSuccessfulTests = 500
	a := arbitrary.DefaultArbitraries()
	p := gopter.NewProperties(tp)
	aNoTimes := arbitrary.DefaultArbitraries()
	aNoTimes.RegisterGen(gen.TimeRange(time.Time{}, time.Hour))

	songType := reflect.TypeOf(Song{})

	p.Property("if Requestable then UntilRequestable == 0", prop.ForAll(
		func(s Song) bool {
			return s.UntilRequestable() == 0
		},
		aNoTimes.GenForType(songType).SuchThat(func(s Song) bool { return s.Requestable() }),
	))

	p.Property("if not Requestable then UntilRequestable > 0", prop.ForAll(
		func(s Song) bool {
			return s.UntilRequestable() > 0
		}, a.GenForType(songType).SuchThat(func(s Song) bool { return !s.Requestable() }),
	))

	p.TestingRun(t)
}

func TestStatusIsZero(t *testing.T) {
	var status Status
	assert.True(t, status.IsZero())
	status.Song.DatabaseTrack = &DatabaseTrack{}
	assert.True(t, status.IsZero())
}

func TestCalculateCooldown(t *testing.T) {
	now := time.Now()

	tests := []struct {
		expect time.Duration
		delay  time.Duration
		last   time.Time
		ok     bool
	}{
		{time.Hour, time.Hour, now, false},
		{0, time.Hour, now.Add(-time.Hour * 2), true},
		{time.Hour, time.Hour, now.Add(time.Hour * 2), false},
		{0, time.Hour, time.Time{}, true},
	}

	for _, test := range tests {
		d, ok := CalculateCooldown(test.delay, test.last)
		if ok != test.ok || test.expect-d > time.Minute {
			t.Errorf("failed %s on %s, returned: %s expected %s", test.last, test.delay, d, test.expect)
		}
	}
}

func TestCalculateRequestDelay(t *testing.T) {
	tp := gopter.DefaultTestParameters()
	tp.MinSuccessfulTests = 10000
	a := arbitrary.DefaultArbitraries()
	p := gopter.NewProperties(tp)

	p.Property("+1 should be higher or equal", a.ForAll(
		func(i int) bool {
			return CalculateRequestDelay(i) <= CalculateRequestDelay(i+1)
		}))
	p.Property("+1 should be higher or equal", prop.ForAll(
		func(i int) bool {
			return CalculateRequestDelay(i) <= CalculateRequestDelay(i+1)
		},
		gen.IntRange(0, 50),
	))
	p.TestingRun(t)
}

func TestUserPermissionsHas(t *testing.T) {
	var up UserPermissions
	// nil map should not panic and return false instead
	assert.False(t, up.Has(PermActive))
	up = make(UserPermissions)

	// dev permission, but not active so this should be false too
	up[PermDev] = struct{}{}
	assert.False(t, up.Has(PermDev))

	// now we're actually active and both Active and Dev previously
	// added should return true
	up[PermActive] = struct{}{}
	assert.True(t, up.Has(PermActive))
	assert.True(t, up.Has(PermDev))
	// Dev permission also gives you a blanket permission on all other permissions
	// so the above combination should give us true for any permission we throw at it
	for _, perm := range AllUserPermissions() {
		assert.True(t, up.Has(perm))
	}
}

func TestUserPermissionsScan(t *testing.T) {
	assert.Error(t, (*UserPermissions).Scan(nil, nil))

	tests := []struct {
		name      string
		input     any
		expectErr bool
		expected  UserPermissions
	}{
		{
			name:      "nil",
			input:     nil,
			expectErr: false,
			expected:  UserPermissions{},
		},
		{
			name:      "something",
			input:     struct{ literallyAnyType any }{},
			expectErr: true,
			expected:  UserPermissions{},
		},
		{
			name:      "string",
			input:     "active,dev,admin",
			expectErr: false,
			expected: UserPermissions{
				PermActive: struct{}{},
				PermDev:    struct{}{},
				PermAdmin:  struct{}{},
			},
		},
		{
			name:      "[]byte",
			input:     []byte("news,database_edit,pending_view"),
			expectErr: false,
			expected: UserPermissions{
				PermPendingView:  struct{}{},
				PermNews:         struct{}{},
				PermDatabaseEdit: struct{}{},
			},
		},
		{
			name:      "string with spaces",
			input:     "active,  dev,  admin",
			expectErr: false,
			expected: UserPermissions{
				PermActive: struct{}{},
				PermDev:    struct{}{},
				PermAdmin:  struct{}{},
			},
		},
		{
			name:      "[]byte with spaces",
			input:     []byte("news,  database_edit,  pending_view"),
			expectErr: false,
			expected: UserPermissions{
				PermPendingView:  struct{}{},
				PermNews:         struct{}{},
				PermDatabaseEdit: struct{}{},
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var up UserPermissions
			err := up.Scan(test.input)
			if test.expectErr {
				assert.EqualError(t, err, "invalid argument passed to Scan")
			} else {
				assert.NoError(t, err)
			}
			assert.Equal(t, test.expected, up)
		})
	}
}

func TestUserIsValid(t *testing.T) {
	tests := []struct {
		name     string
		user     *User
		expected bool
	}{
		{
			name:     "nil",
			user:     nil,
			expected: false,
		},
		{
			name:     "zero value",
			user:     new(User),
			expected: false,
		},
		{
			name: "actual value",
			user: &User{
				ID:       500,
				Username: "test",
			},
			expected: true,
		},
		{
			name: "value with 0 ID",
			user: &User{
				ID:       0,
				Username: "test",
			},
			expected: true,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := test.user.IsValid()
			assert.Equal(t, test.expected, got)
		})
	}
}

func TestScheduleDayString(t *testing.T) {
	assert.Equal(t, "Monday", Monday.String())
	assert.Equal(t, "Tuesday", Tuesday.String())
	assert.Equal(t, "Wednesday", Wednesday.String())
	assert.Equal(t, "Thursday", Thursday.String())
	assert.Equal(t, "Friday", Friday.String())
	assert.Equal(t, "Saturday", Saturday.String())
	assert.Equal(t, "Sunday", Sunday.String())
	assert.Equal(t, "Unknown", ScheduleDay(100).String())
}

func TestParseScheduleDay(t *testing.T) {
	assert.Equal(t, Monday, ParseScheduleDay(Monday.String()))
	assert.Equal(t, Tuesday, ParseScheduleDay(Tuesday.String()))
	assert.Equal(t, Wednesday, ParseScheduleDay(Wednesday.String()))
	assert.Equal(t, Thursday, ParseScheduleDay(Thursday.String()))
	assert.Equal(t, Friday, ParseScheduleDay(Friday.String()))
	assert.Equal(t, Saturday, ParseScheduleDay(Saturday.String()))
	assert.Equal(t, Sunday, ParseScheduleDay(Sunday.String()))
	assert.Equal(t, UnknownDay, ParseScheduleDay("garbage"))
}

func TestPasswordFunctions(t *testing.T) {
	// change the cost to the minimum so tests run faster, we're not testing
	// bcrypt itself here, just that our wrappers around whatever crypto lib
	// we're using match up together
	var oldCost = bcryptCost
	bcryptCost = 0
	defer func() {
		bcryptCost = oldCost
	}()

	dtp := gopter.DefaultTestParameters()
	dtp.MaxSize = 72            // max size of bcrypt passwords
	dtp.MinSuccessfulTests = 50 // lower this so it runs a bit faster
	p := gopter.NewProperties(dtp)
	p.Property("should compare", prop.ForAll(func(in []byte) bool {
		passwd := string(in)

		hash, err := GenerateHashFromPassword(passwd)
		if err != nil {
			t.Log(len(passwd))
			t.Log("generate failed:", err)
			return false
		}
		if hash == "" {
			t.Log("generate returned empty hash")
			return false
		}

		user := User{
			Password: hash,
		}
		err = user.ComparePassword(passwd)
		if err != nil {
			t.Log("compare failed:", err)
			return false
		}
		return true
	}, gen.SliceOf(gen.UInt8())))
	p.TestingRun(t)

}

func TestSongHashValuer(t *testing.T) {
	a := arbitrary.DefaultArbitraries()
	p := gopter.NewProperties(nil)
	p.Property("roundtrip", a.ForAll(func(in SongHash) bool {
		value, err := in.Value()
		if err != nil {
			t.Log("value failed:", err)
			return false
		}

		var out SongHash
		err = out.Scan(value)
		if err != nil {
			t.Log("scan failed:", err)
			return false
		}
		return out == in
	}))
	p.TestingRun(t)
}

func TestSongHashScan(t *testing.T) {
	var hash SongHash
	other := NewSongHash("testing song hash")

	require.Error(t, hash.Scan(0), "unsupported type should error")
	require.NoError(t, hash.Scan(nil), "nil should not error")

	value, err := other.Value()
	require.NoError(t, err, "value should not error")
	require.NotNil(t, value, "value returned should not be nil")

	sv, ok := value.(string)
	require.True(t, ok, "value should be a string")

	var fromValue SongHash
	require.NoError(t, fromValue.Scan(sv), "scan of string should not error")
	require.Equal(t, other, fromValue, "should be equal after value>scan sequence")

	var fromByte SongHash
	require.NoError(t, fromByte.Scan([]byte(sv)), "scan of []byte should not error")
	require.Equal(t, other, fromByte, "should be equal after value>scan sequence")
}

func TestSongHashJSON(t *testing.T) {
	a := arbitrary.DefaultArbitraries()
	p := gopter.NewProperties(nil)
	p.Property("roundtrip", a.ForAll(func(in SongHash) bool {
		value, err := in.MarshalJSON()
		if err != nil {
			t.Log("marshal failed:", err)
			return false
		}

		var out SongHash
		err = out.UnmarshalJSON(value)
		if err != nil {
			t.Log("unmarshal failed:", err)
			return false
		}
		return out == in
	}))
	p.TestingRun(t)
}
