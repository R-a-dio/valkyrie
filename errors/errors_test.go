package errors

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestUnwrapJoin(t *testing.T) {
	var errs []error
	for i := range 10 {
		errs = append(errs, fmt.Errorf("error %d", i))
	}

	// the normal join
	err := Join(errs...)
	require.EqualValues(t, errs, UnwrapJoin(err))

	// wrap the error
	err = E("TestUnwrapJoin", err)
	require.EqualValues(t, errs, UnwrapJoin(err))
}
