package database

import (
	"database/sql"
	"math/rand"
	"reflect"
	"testing"
	"testing/quick"
	"time"

	radio "github.com/R-a-dio/valkyrie"
	"github.com/go-sql-driver/mysql"
)

func generateDatabaseTrack(v []reflect.Value, r *rand.Rand) {
	for i := range v {
		dt := &databaseTrack{}

		vv := reflect.ValueOf(dt).Elem()
		for i := 0; i < vv.NumField(); i++ {
			vt := vv.Field(i).Type()

			var value reflect.Value
			var ok bool

			// special case creation of some types, mostly ones that have unexported
			// fields somewhere in a struct
			switch vt.Name() {
			case "NullTime":
				value = reflect.ValueOf(mysql.NullTime{
					Valid: true,
					Time:  time.Unix(r.Int63(), r.Int63()),
				})
			default:
				value, ok = quick.Value(vt, r)
				if !ok {
					panic("invalid value to field")
				}
			}

			vv.Field(i).Set(value)
		}

		v[i] = vv
	}
}

var databaseTrackToSongMapping = map[string]string{}

func indirectDatabaseNull(v reflect.Value) reflect.Value {
	switch i := v.Interface().(type) {
	case sql.NullFloat64:
		return reflect.ValueOf(i.Float64)
	case sql.NullInt64:
		return reflect.ValueOf(i.Int64)
	case sql.NullString:
		return reflect.ValueOf(i.String)
	case mysql.NullTime:
		return reflect.ValueOf(i.Time)
	}

	return v
}

func indirectRadioTypes(v reflect.Value) reflect.Value {
	switch i := v.Interface().(type) {
	case radio.SongID:
		return reflect.ValueOf(int64(i))
	case radio.TrackID:
		return reflect.ValueOf(int64(i))
	}
	return v
}
func TestDatabaseTrackToSong(t *testing.T) {
	err := quick.Check(func(dt databaseTrack) bool {
		song := dt.ToSong()

		vv := reflect.ValueOf(dt)
		vt := vv.Type()

		sv := reflect.ValueOf(song)

		for i := 0; i < vv.NumField(); i++ {
			var name = vt.Field(i).Name
			if mapped, ok := databaseTrackToSongMapping[name]; ok {
				name = mapped
			}

			originalValue := vv.FieldByName(name)
			targetValue := sv.FieldByName(name)
			// some of our types are Null* types from the database package, we want to
			// extract the actual value of these temporary types to compare to
			originalValue = indirectDatabaseNull(originalValue)
			// we have specialized types for some of the integer types, so convert those
			// back to normal types
			targetValue = indirectRadioTypes(targetValue)

			switch name {
			case "Length":
				continue
			}

			if reflect.DeepEqual(originalValue.Interface(), targetValue.Interface()) {
				continue
			}

			t.Errorf("unequal target field %s: (%s) %v != (%s) %v", name,
				originalValue.Type(), originalValue,
				targetValue.Type(), targetValue)
			return false
		}

		return true
	}, &quick.Config{
		Values: generateDatabaseTrack,
	})

	if err != nil {
		t.Error(err)
	}
}
