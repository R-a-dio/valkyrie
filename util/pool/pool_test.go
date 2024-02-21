package pool_test

import (
	"testing"

	"github.com/R-a-dio/valkyrie/util/pool"
	"github.com/stretchr/testify/assert"
)

type testResetable struct {
	called bool
}

func (tr *testResetable) Reset() {
	tr.called = true
}

func TestResetPool(t *testing.T) {
	var called bool
	p := pool.NewResetPool(func() *testResetable {
		called = true
		return &testResetable{}
	})

	x := p.Get()
	assert.True(t, called)

	p.Put(x)
	assert.True(t, x.called)
}
