package auth

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"

	"golang.org/x/crypto/argon2"
)

var ErrBusy = errors.New("registration temporarily busy")

type Hasher interface {
	Hash(context.Context, string) (string, error)
}

type ArgonHasher struct {
	config Config
	slots  chan struct{}
}

func NewArgonHasher(c Config) (*ArgonHasher, error) {
	if err := c.Validate(); err != nil {
		return nil, err
	}
	return &ArgonHasher{config: c, slots: make(chan struct{}, c.HashConcurrency)}, nil
}

func (h *ArgonHasher) Hash(ctx context.Context, password string) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	select {
	case h.slots <- struct{}{}:
	default:
		return "", ErrBusy
	}
	defer func() { <-h.slots }()
	salt := make([]byte, 16)
	if _, err := rand.Read(salt); err != nil {
		return "", err
	}
	c := h.config
	key := argon2.IDKey([]byte(password), salt, c.ArgonIterations, c.ArgonMemory, c.ArgonParallelism, 32)
	defer clear(key)
	if err := ctx.Err(); err != nil {
		return "", err
	}
	b64 := base64.RawStdEncoding
	return fmt.Sprintf("$argon2id$v=19$m=%d,t=%d,p=%d$%s$%s", c.ArgonMemory, c.ArgonIterations, c.ArgonParallelism, b64.EncodeToString(salt), b64.EncodeToString(key)), nil
}
