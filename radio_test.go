package radio

import (
	"reflect"
	"testing"
	"time"
	"unicode"

	"github.com/leanovate/gopter"
	"github.com/leanovate/gopter/arbitrary"
	"github.com/leanovate/gopter/gen"
	"github.com/leanovate/gopter/prop"
	"github.com/stretchr/testify/assert"
)

func TestSongEqualTo(t *testing.T) {
	tp := gopter.DefaultTestParameters()
	tp.MinSuccessfulTests = 500
	a := arbitrary.DefaultArbitraries()
	p := gopter.NewProperties(tp)

	// we only want songs with actual string data to compare against
	a.RegisterGen(gen.UnicodeString(unicode.Katakana).SuchThat(func(s string) bool { return len(s) > 5 }))

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
