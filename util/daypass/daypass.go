package daypass

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"sync"
	"time"

	"github.com/rs/zerolog"
)

func New(ctx context.Context) *Daypass {
	return &Daypass{
		logger: zerolog.Ctx(ctx),
	}
}

type Daypass struct {
	logger *zerolog.Logger
	// mu protects update and daypass
	mu      sync.Mutex
	update  time.Time
	daypass string
}

type DaypassInfo struct {
	// ValidUntil is the time this daypass will expire
	ValidUntil time.Time
	// Value is the current daypass
	Value string
}

// Info returns info about the daypass
func (di *Daypass) Info() DaypassInfo {
	var info DaypassInfo
	info.Value = di.get()
	di.mu.Lock()
	info.ValidUntil = di.update.Add(time.Hour * 24)
	di.mu.Unlock()
	return info
}

// Is checks if the daypass given is equal to the current daypass
func (di *Daypass) Is(daypass string) bool {
	return di.get() == daypass
}

// get returns the current daypass and optionally generates a new one
// if it has expired
func (di *Daypass) get() string {
	di.mu.Lock()
	defer di.mu.Unlock()

	if time.Since(di.update) >= time.Hour*24 {
		di.update = time.Now()
		di.daypass = di.generate()
	}

	return di.daypass
}

// generate a new random daypass, this is a random sequence of
// bytes, sha256'd and base64 encoded before trimming it down to 16 characters
func (di *Daypass) generate() string {
	var b [32]byte
	if _, err := rand.Read(b[:]); err != nil {
		di.logger.WithLevel(zerolog.PanicLevel).Err(err).Msg("daypass failure")
		// keep using the old daypass if we error
		return di.daypass
	}

	b = sha256.Sum256(b[:])
	new := base64.RawURLEncoding.EncodeToString(b[:])[:16]
	di.logger.Info().Str("value", new).Msg("daypass update")
	return new
}
