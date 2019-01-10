package engine

import (
	"os"

	"github.com/R-a-dio/valkyrie/config"
)

// Errors is a slice of multiple config-file errors
type Errors []error

func (e Errors) Error() string {
	s := "config: error opening file:"
	if len(e) == 1 {
		return s + e[0].Error()
	}

	for _, err := range e {
		s += "\n" + err.Error()
	}

	return s
}

// ConfigComponent loads the configuration files given, in order they are given and
// using the file that succeeds to open first
func ConfigComponent(paths ...string) StartFn {
	return func(e *Engine) (DeferFn, error) {
		var f *os.File
		var err error
		var errs Errors

		for _, path := range paths {
			f, err = os.Open(path)
			if err == nil {
				break
			}

			errs = append(errs, err)
		}

		if f == nil {
			return nil, errs
		}

		defer f.Close()

		e.Config, err = config.Load(f)
		if err != nil {
			return nil, err
		}

		return nil, nil
	}
}
