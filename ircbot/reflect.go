package ircbot

import (
	"errors"
	"fmt"
	"reflect"
)

var errorType = reflect.TypeOf((*error)(nil)).Elem()

type parseFunc func(string) (reflect.Value, error)

func makeParseFunc(fn interface{}) (reflect.Type, parseFunc, error) {
	v := reflect.ValueOf(fn)
	if v.Kind() != reflect.Func {
		return nil, nil, errors.New("parse: argument not a function")
	}

	vt := v.Type()

	err := checkParseIn(vt)
	if err != nil {
		return nil, nil, err
	}

	err = checkParseOut(vt)
	if err != nil {
		return nil, nil, err
	}

	cfn := func(in string) (reflect.Value, error) {
		inv := reflect.ValueOf(in)

		res := v.Call([]reflect.Value{inv})

		var err error
		// if we're expecting an error, unwrap it from the reflect.Value
		if vt.NumOut() == 2 {
			// ignore the ok return because it will be false when error is nil
			err, _ = res[1].Interface().(error)
		}

		return res[0], err
	}

	return vt.Out(0), cfn, nil
}

func checkParseIn(typ reflect.Type) error {
	if typ.NumIn() != 1 {
		return fmt.Errorf("parse: need exactly one argument, have %d", typ.NumIn())
	}

	shouldString := typ.In(0)
	if shouldString.Kind() != reflect.String {
		return fmt.Errorf("parse: argument is not a string, is %s", shouldString)
	}

	return nil
}

func checkParseOut(typ reflect.Type) error {
	if typ.NumOut() == 0 {
		return errors.New("parse: no return values")
	} else if typ.NumOut() > 2 {
		return fmt.Errorf("parse: too many return values, have %d", typ.NumOut())
	}

	// we assume it's an error if the first return value is an error
	var shouldNotError = typ.Out(0)
	if shouldNotError == errorType {
		return fmt.Errorf("parse: first return value is error")
	}

	var shouldError = errorType
	if typ.NumOut() == 2 {
		shouldError = typ.Out(1)
	}

	if shouldError != errorType {
		return fmt.Errorf(
			"parse: second return value is not an error, is %s",
			shouldError,
		)
	}

	return nil
}
