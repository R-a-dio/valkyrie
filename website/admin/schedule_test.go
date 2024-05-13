package admin

import (
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"

	radio "github.com/R-a-dio/valkyrie"
	"github.com/R-a-dio/valkyrie/mocks"
	"github.com/leanovate/gopter"
	"github.com/leanovate/gopter/arbitrary"
	"github.com/leanovate/gopter/gen"
	"github.com/leanovate/gopter/prop"
	"github.com/stretchr/testify/assert"
)

func TestScheduleForm(t *testing.T) {
	a := arbitrary.DefaultArbitraries()
	p := gopter.NewProperties(nil)

	scheduleEntryGen := gen.StructPtr(reflect.TypeFor[radio.ScheduleEntry](), map[string]gopter.Gen{
		"Weekday": gen.OneConstOf(
			radio.Monday,
			radio.Tuesday,
			radio.Wednesday,
			radio.Thursday,
			radio.Friday,
			radio.Saturday,
			radio.Sunday,
		),
		"Text":         gen.AnyString(),
		"Notification": gen.Bool(),
		"Owner":        a.GenForType(reflect.TypeFor[*radio.User]()),
	})

	var getByDJIDRet *radio.User
	us := &mocks.UserStorageMock{
		GetByDJIDFunc: func(dJID radio.DJID) (*radio.User, error) {
			assert.Equal(t, getByDJIDRet.DJ.ID, dJID)
			return getByDJIDRet, nil
		},
		AllFunc: func() ([]radio.User, error) {
			if getByDJIDRet == nil {
				return []radio.User{}, nil
			}
			return []radio.User{
				*getByDJIDRet,
			}, nil
		},
	}

	p.Property("schedule form should roundtrip", prop.ForAll(
		func(entry *radio.ScheduleEntry, user radio.User) bool {
			in := ScheduleForm{
				Entry: entry,
			}
			if entry == nil {
				getByDJIDRet = nil
			} else {
				getByDJIDRet = entry.Owner
			}

			req := httptest.NewRequest(http.MethodPost, "/admin/schedule", nil)
			req.PostForm = in.ToValues()

			out, err := NewScheduleForm(us, user, req)
			if !assert.NoError(t, err) {
				return false
			}
			if !assert.NotNil(t, out) {
				return false
			}

			a, b := in.Entry, out.Entry
			return assert.Equal(t, a.Text, b.Text) &&
				assert.Equal(t, a.Weekday, b.Weekday) &&
				assert.Equal(t, a.Notification, b.Notification) &&
				assert.EqualExportedValues(t, a.Owner, b.Owner) &&
				assert.EqualExportedValues(t, user, b.UpdatedBy)
		}, scheduleEntryGen, a.GenForType(reflect.TypeFor[radio.User]()),
	))
	p.TestingRun(t)
}
