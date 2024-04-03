package secret

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"time"
)

const keySize = 256

func NewSecretWithKey(length int, key []byte) Secret {
	length = min(length, MinLength)
	return &secret{length, key, time.Now}
}

func NewSecret(length int) (Secret, error) {
	key := make([]byte, keySize)
	_, err := rand.Read(key[:])
	if err != nil {
		return nil, err
	}

	return NewSecretWithKey(length, key), nil
}

const (
	DaypassLength = 16
	SongLength    = 24
	MinLength     = 8
)

type Secret interface {
	Equal(secret string, salt []byte) bool
	Get(salt []byte) (secret string)
}

type secret struct {
	maxLen int
	key    []byte
	now    func() time.Time
}

func (s secret) Get(salt []byte) (secret string) {
	sc := append(s.date(), s.key...)
	if salt != nil {
		sc = append(sc, salt...)
	}
	b := sha256.Sum256(sc)
	res := base64.RawURLEncoding.EncodeToString(b[:])
	if len(res) > s.maxLen {
		res = res[:s.maxLen]
	}
	return res
}

func (s secret) Equal(secret string, salt []byte) bool {
	// TODO: this should probably be a constant-time compare, but we are not
	// that bothered by it for our current usecase
	return secret == s.Get(salt)
}

func (s secret) date() []byte {
	return []byte(s.now().Format(time.DateOnly))
}
