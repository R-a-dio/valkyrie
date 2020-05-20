package admin

import (
	"math/rand"
	"reflect"
	"testing"
	"testing/quick"
	"time"

	radio "github.com/R-a-dio/valkyrie"
)

type testSession struct {
	Deadline time.Time
	Values   map[string]interface{}
}

func (ts testSession) Generate(rand *rand.Rand, size int) reflect.Value {
	s := testSession{
		Values: make(map[string]interface{}),
	}
	ss := radio.Session{}

	token, ok := quick.Value(reflect.TypeOf(radio.SessionToken("")), rand)
	if !ok {
		panic("??")
	}
	ss.Token = token.Interface().(radio.SessionToken)

	u1, ok := quick.Value(reflect.TypeOf(int64(0)), rand)
	if !ok {
		panic("failed to create int64 for time")
	}
	u2, ok := quick.Value(reflect.TypeOf(int64(0)), rand)
	if !ok {
		panic("failed to create int64 for time")
	}

	ss.Expiry = time.Unix(
		u1.Interface().(int64),
		u2.Interface().(int64),
	)

	s.Values[sessionKey] = &ss
	s.Deadline = ss.Expiry

	return reflect.ValueOf(s)
}

func TestPtrCodecQuick(t *testing.T) {
	codec := ptrCodec{}

	f := func(s testSession) bool {
		x, err := codec.Encode(s.Deadline, s.Values)
		if err != nil {
			return false
		}

		_, values, err := codec.Decode(x)
		if err != nil {
			return false
		}

		aaa := values[sessionKey].(*radio.Session)
		bbb := s.Values[sessionKey].(*radio.Session)

		if aaa != bbb {
			t.Errorf("%#v", bbb)
			t.Errorf("%#v", aaa)
			return false
		}

		return true
	}

	if err := quick.Check(f, nil); err != nil {
		t.Error(err)
	}
}

func TestPtrCodec(t *testing.T) {
	username := "valkyrie"
	var up radio.UserPermissions
	err := up.Scan("1,2,3,4,5")
	if err != nil {
		t.Fatal("failed to scan simple userpermissions:", err)
	}

	session := &radio.Session{
		Token:    "Random Token",
		Expiry:   time.Now(),
		Username: &username,
		SessionData: radio.SessionData{
			UserPermissions: up,
		},
	}
	codec := ptrCodec{}

	values := map[string]interface{}{
		sessionKey: session,
	}

	b, err := codec.Encode(session.Expiry, values)
	if err != nil {
		t.Fatal("failed encoding:", err)
	}

	expiry, values, err := codec.Decode(b)
	if err != nil {
		t.Fatal("failed decoding:", err)
	}

	if expiry != session.Expiry {
		t.Errorf("%#v != %#v", expiry, session.Expiry)
	}

	dSession := values[sessionKey].(*radio.Session)
	if dSession != session {
		t.Errorf("%v != %v", dSession, session)
	}
}
