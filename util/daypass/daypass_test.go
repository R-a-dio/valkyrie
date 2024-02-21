package daypass_test

import (
	"context"
	"testing"
	"time"

	"github.com/R-a-dio/valkyrie/util/daypass"
	"github.com/stretchr/testify/assert"
)

func TestDaypassIs(t *testing.T) {
	d := daypass.New(context.Background())

	// should be equal right away
	assert.True(t, d.Is(d.Info().Value))
	// and should be valid for a while
	assert.True(t, d.Info().ValidUntil.After(time.Now()))
}
