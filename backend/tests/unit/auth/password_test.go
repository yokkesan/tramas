package auth_test

import (
	"context"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"runtime"
	"strings"
	"sync"
	"testing"
	"tramas/backend/internal/auth"

	"golang.org/x/crypto/argon2"
)

func TestArgon2HashAndSalt(t *testing.T) {
	c := auth.DefaultConfig()
	h, err := auth.NewArgonHasher(c)
	if err != nil {
		t.Fatal(err)
	}
	password := "a unique long passphrase"
	first, err := h.Hash(context.Background(), password)
	if err != nil {
		t.Fatal(err)
	}
	second, err := h.Hash(context.Background(), password)
	if err != nil {
		t.Fatal(err)
	}
	if first == second || strings.Contains(first, password) {
		t.Fatal("hash not salted or plaintext leaked")
	}
	parts := strings.Split(first, "$")
	if len(parts) != 6 || parts[1] != "argon2id" || parts[2] != "v=19" {
		t.Fatal("invalid PHC format")
	}
	salt, err := base64.RawStdEncoding.DecodeString(parts[4])
	if err != nil || len(salt) != 16 {
		t.Fatal("invalid salt")
	}
	key, err := base64.RawStdEncoding.DecodeString(parts[5])
	if err != nil || len(key) != 32 {
		t.Fatal("invalid key")
	}
	want := argon2.IDKey([]byte(password), salt, c.ArgonIterations, c.ArgonMemory, c.ArgonParallelism, 32)
	if subtle.ConstantTimeCompare(want, key) != 1 {
		t.Fatal("incorrect hash")
	}
	wrong := argon2.IDKey([]byte(password+"wrong"), salt, c.ArgonIterations, c.ArgonMemory, c.ArgonParallelism, 32)
	if subtle.ConstantTimeCompare(wrong, key) != 0 {
		t.Fatal("wrong password matched")
	}
}

func TestHashConcurrencyAndCancellation(t *testing.T) {
	c := auth.DefaultConfig()
	c.HashConcurrency = 1
	c.ArgonMemory = 64 * 1024
	c.ArgonIterations = 5
	h, err := auth.NewArgonHasher(c)
	if err != nil {
		t.Fatal(err)
	}

	previous := runtime.GOMAXPROCS(4)
	defer runtime.GOMAXPROCS(previous)
	const workers = 4
	start := make(chan struct{})
	ready := sync.WaitGroup{}
	ready.Add(workers)
	results := make(chan error, workers)
	for range workers {
		go func() {
			ready.Done()
			<-start
			_, err := h.Hash(context.Background(), "a unique long passphrase")
			results <- err
		}()
	}
	ready.Wait()
	close(start)
	success, busy := 0, 0
	for range workers {
		err := <-results
		switch {
		case err == nil:
			success++
		case errors.Is(err, auth.ErrBusy):
			busy++
		default:
			t.Fatalf("unexpected hash error: %v", err)
		}
	}
	if success == 0 || busy == 0 {
		t.Fatal("concurrent hashing did not enforce capacity")
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := h.Hash(ctx, "a unique long passphrase"); !errors.Is(err, context.Canceled) {
		t.Fatal("cancellation ignored")
	}
	if _, err := h.Hash(context.Background(), "a unique long passphrase"); err != nil {
		t.Fatal("hash capacity was not released after completion/cancellation")
	}
}
