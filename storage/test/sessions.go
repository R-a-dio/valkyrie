package storagetest

import (
	"encoding/base64"
	"encoding/json"
	"reflect"
	"time"

	radio "github.com/R-a-dio/valkyrie"
	"github.com/R-a-dio/valkyrie/errors"
	"github.com/leanovate/gopter"
	"github.com/leanovate/gopter/gen"
	"github.com/leanovate/gopter/prop"
	"github.com/stretchr/testify/assert"
)

var ourStartTime = time.Date(2009, 8, 20, 0, 0, 0, 0, time.UTC)

// TestGenSessionStorage runs through all methods of the SessionStorage interface with
// a randomly generated session.
func (suite *Suite) TestGenSessionStorage() {
	time.Local = time.UTC
	ss := suite.Storage.Sessions(suite.ctx)

	parameters := gopter.DefaultTestParameters()
	parameters.MaxSize = 1028 * 16
	properties := gopter.NewProperties(parameters)
	properties.Property("Save > Get > Delete > Get Unknown", prop.ForAll(
		func(in radio.Session) bool {
			t := suite.T()

			err := ss.Save(in)
			if err != nil {
				t.Log("failed to save:", err)
				return false
			}
			got, err := ss.Get(in.Token)
			if err != nil {
				t.Log("failed to get 1:", err)
				return false
			}

			gotChecked := assert.Equal(t, got.Token, in.Token) &&
				assert.WithinDuration(t, got.Expiry, in.Expiry, time.Second) &&
				assert.Equal(t, got.Data, in.Data)

			err = ss.Delete(got.Token)
			if err != nil {
				t.Log("failed to delete:", err)
				return false
			}

			_, err = ss.Get(got.Token)
			if err != nil && !errors.Is(errors.SessionUnknown, err) {
				t.Log("failed to get 2:", err)
				return false
			}

			return gotChecked
		},
		genSession(),
	))
	suite.Run("gopter: Save > Get > Delete > Get Unknown", func() {
		properties.TestingRun(suite.T(), gopter.ConsoleReporter(true))
	})
}

func genSessionJson() gopter.Gen {
	return func(par *gopter.GenParameters) *gopter.GenResult {
		res := gen.SliceOf(gen.UInt8())(par)
		res.Result, _ = json.Marshal(res.Result.([]byte))
		return res
	}
}

func genSessionToken() gopter.Gen {
	return gopter.Gen(func(par *gopter.GenParameters) *gopter.GenResult {
		ours := *par
		ours.MaxSize = 32
		res := gen.SliceOf(gen.UInt8())(&ours)
		s := base64.RawURLEncoding.EncodeToString(res.Result.([]byte))
		res.Result = radio.SessionToken(s)
		res.ResultType = reflect.TypeOf(res.Result)
		return res
	}).WithShrinker(nil)
}

func genSession() gopter.Gen {
	gennie := gen.Struct(reflect.TypeOf(radio.Session{}), map[string]gopter.Gen{
		"Token":  genSessionToken(),
		"Expiry": gen.TimeRange(ourStartTime, time.Hour*24*3500),
		"Data":   genSessionJson(),
	})
	// the above gen sometimes returns a time before our start time that we don't support so
	// set it to the `ourStartTime` when that is the case
	return func(gp *gopter.GenParameters) *gopter.GenResult {
		res := gennie(gp)
		s := res.Result.(radio.Session)
		if s.Expiry.Before(ourStartTime) {
			s.Expiry = ourStartTime
		}
		res.Result = s
		return res
	}
}
