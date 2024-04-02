package secret

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"time"
)

const keySize = 256

func NewSecretWithKey(length int, key []byte) Secret {
	return secret{length, key}
}

func NewSecret(length int) (Secret, error) {
	key := make([]byte, keySize)
	_, err := rand.Read(key[:])
	if err != nil {
		return nil, err
	}

	return NewSecretWithKey(length, key), nil
}

const DaypassLength = 16

type Secret interface {
	Equal(secret string, salt []byte) bool
	Get(salt []byte) (secret string)
}

type secret struct {
	maxLen int
	key    []byte
}

func (s secret) Get(salt []byte) (secret string) {
	sc := append(date(), s.key...)
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
	return secret == s.Get(salt)
}

var date = dateNow

func dateNow() []byte {
	return []byte(time.Now().Format(time.DateOnly))
}
