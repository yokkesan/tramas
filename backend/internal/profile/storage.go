package profile

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"io"
	"os"
	"regexp"
)

var ErrStorage = errors.New("image storage unavailable")

type Storage interface {
	Put(context.Context, []byte) (string, error)
	Open(context.Context, string) (io.ReadCloser, error)
	Delete(context.Context, string) error
}
type FileStorage struct{ root *os.Root }

var storageKey = regexp.MustCompile(`^[a-f0-9]{64}$`)

func NewFileStorage(path string) (*FileStorage, error) {
	root, err := os.OpenRoot(path)
	if err != nil {
		return nil, err
	}
	return &FileStorage{root}, nil
}
func (s *FileStorage) Close() error { return s.root.Close() }
func (s *FileStorage) Put(ctx context.Context, data []byte) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	key := hex.EncodeToString(b)
	f, err := s.root.OpenFile(key, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return "", err
	}
	_, err = f.Write(data)
	if closeErr := f.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		_ = s.root.Remove(key)
		return "", err
	}
	return key, nil
}
func (s *FileStorage) Open(ctx context.Context, key string) (io.ReadCloser, error) {
	if !storageKey.MatchString(key) {
		return nil, ErrStorage
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return s.root.Open(key)
}
func (s *FileStorage) Delete(ctx context.Context, key string) error {
	if !storageKey.MatchString(key) {
		return ErrStorage
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	err := s.root.Remove(key)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	return err
}
