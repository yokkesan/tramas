package auth

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"
	"sync"

	"golang.org/x/crypto/argon2"
)

var ErrBusy = errors.New("registration temporarily busy")

type Hasher interface {
	Hash(context.Context, string) (string, error)
}

// Verify accepts only bounded PHC parameters, including hashes from older settings.
// Hashing and verification share a single capacity limit.
func (h *ArgonHasher) Verify(ctx context.Context, password, encoded string) (bool, error) {
	if err := ctx.Err(); err != nil {
		return false, err
	}
	parts := strings.Split(encoded, "$")
	if len(parts) != 6 || parts[0] != "" || parts[1] != "argon2id" || parts[2] != "v=19" {
		return false, nil
	}
	var memory, iterations uint32
	var parallelism uint8
	if _, err := fmt.Sscanf(parts[3], "m=%d,t=%d,p=%d", &memory, &iterations, &parallelism); err != nil {
		return false, nil
	}
	if parts[3] != fmt.Sprintf("m=%d,t=%d,p=%d", memory, iterations, parallelism) || memory < 19*1024 || memory > 64*1024 || iterations < 2 || iterations > 5 || parallelism < 1 || parallelism > 4 {
		return false, nil
	}
	salt, err := base64.RawStdEncoding.Strict().DecodeString(parts[4])
	if err != nil || len(salt) != 16 {
		return false, nil
	}
	want, err := base64.RawStdEncoding.Strict().DecodeString(parts[5])
	if err != nil || len(want) != 32 {
		return false, nil
	}
	if !h.acquire(memory) {
		return false, ErrBusy
	}
	defer h.release(memory)
	key := argon2.IDKey([]byte(password), salt, iterations, memory, parallelism, 32)
	defer clear(key)
	if err := ctx.Err(); err != nil {
		return false, err
	}
	return subtle.ConstantTimeCompare(key, want) == 1, nil
}

type ArgonHasher struct {
	config      Config
	slots       chan struct{}
	memoryMu    sync.Mutex
	memoryInUse uint32
}

// Historical hashes can require more memory than the current hash settings.
// Enforce an aggregate 128 MiB budget for both creation and verification.
func (h *ArgonHasher) acquire(memory uint32) bool {
	h.memoryMu.Lock()
	defer h.memoryMu.Unlock()
	if h.memoryInUse+memory > 128*1024 {
		return false
	}
	select {
	case h.slots <- struct{}{}:
		h.memoryInUse += memory
		return true
	default:
		return false
	}
}
func (h *ArgonHasher) release(memory uint32) {
	h.memoryMu.Lock()
	defer h.memoryMu.Unlock()
	h.memoryInUse -= memory
	<-h.slots
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
	if !h.acquire(h.config.ArgonMemory) {
		return "", ErrBusy
	}
	defer h.release(h.config.ArgonMemory)
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
