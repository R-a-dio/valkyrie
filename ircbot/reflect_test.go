package ircbot

import (
	"reflect"
	"testing"
)

type notstring string

var testParseCheck = []testFuncCheck{
	{func(string) int { return 0 }, true, true},
	{func(string) testFunc { return testFunc{} }, true, true},
	{func(string) { return }, true, false},
	{func(int) (int, int) { return 0, 0 }, false, false},
	{func(notstring) (int, error, int) { return 0, nil, 0 }, true, false},
	{func(string) { return }, true, false},
	{func() { return }, false, false},
	{func(float32) { return }, false, false},
	{func(string, string) error { return nil }, false, false},
	{func(int, int, int, int, int) (error, string) { return nil, "" }, false, false},
}

var testParseFunc = []testFunc{
	{0, nil, nil},
	{"", nil, nil},
	{nil, nil, nil},
	{func(s string) string { return s }, "testing", nil},
	{func(string) (string, error) { return "", nil }, "", nil},
}

type testFuncCheck struct {
	fn       interface{}
	validIn  bool
	validOut bool
}

type testFunc struct {
	fn interface{}
	// expected should be nil to indicate fn not being valid
	expected interface{}
	parsefn  parseFunc
}

func TestParseIn(t *testing.T) {
	for _, fn := range testParseCheck {
		typ := reflect.TypeOf(fn.fn)
		err := checkParseIn(typ)
		if err == nil != fn.validIn {
			t.Error(typ, err)
		}
	}
}

func TestParseOut(t *testing.T) {
	for _, fn := range testParseCheck {
		typ := reflect.TypeOf(fn.fn)
		err := checkParseOut(typ)
		if err == nil != fn.validOut {
			t.Error(typ, err)
		}
	}
}

func TestParseFunc(t *testing.T) {
	for _, fn := range testParseCheck {
		_, _, err := makeParseFunc(fn.fn)
		if (err == nil) != (fn.validIn && fn.validOut) {
			t.Error(fn.fn, err)
		}
	}

	var valid []testFunc
	for _, fn := range testParseFunc {
		_, parsefn, err := makeParseFunc(fn.fn)
		if (err == nil) != (fn.expected != nil) {
			t.Error(fn.fn, err)
			continue
		}

		if err != nil {
			continue
		}

		fn.parsefn = parsefn
		valid = append(valid, fn)
	}

	for _, fn := range valid {
		v, err := fn.parsefn("testing")
		if err != nil && err != fn.expected {
			t.Error(fn, err)
		}

		if !reflect.DeepEqual(v.Interface(), fn.expected) {
			t.Errorf("unequal: %s != %s", v, fn.expected)
		}
	}
}
