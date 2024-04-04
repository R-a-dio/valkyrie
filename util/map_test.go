package util

import (
	"net"
	"testing"
)

func TestTypedValueNil(t *testing.T) {
	var v = new(TypedValue[net.Conn])

	v.Store(nil)
}
