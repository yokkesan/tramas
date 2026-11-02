package profile

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"tramas/backend/internal/auth"
	"unicode/utf8"
)

var ErrNotFound = errors.New("profile not found")
var ErrInput = errors.New("invalid profile input")

type Image struct {
	ID          string `json:"id"`
	ContentType string `json:"content_type"`
	Size        int64  `json:"size"`
}
type Profile struct {
	ID             string  `json:"id"`
	Nickname       *string `json:"nickname"`
	Biography      string  `json:"biography"`
	Avatar         *Image  `json:"avatar"`
	Header         *Image  `json:"header"`
	AvatarStandard *string `json:"avatar_standard_id"`
	HeaderStandard *string `json:"header_standard_id"`
}
type StandardImage struct {
	ID   string `json:"id"`
	Kind string `json:"kind"`
	URL  string `json:"url"`
}
type Repository interface {
	Get(context.Context, string, bool) (Profile, error)
	Update(context.Context, string, *string, *string) (Profile, error)
	SetImage(context.Context, string, string, string, string, int64) (Image, error)
	SelectStandard(context.Context, string, string, *string) error
	Media(context.Context, string, string, bool) (string, string, error)
	Unused(context.Context, string) ([]StoredMedia, error)
	DeleteUnused(context.Context, string) error
}
type StoredMedia struct{ ID, Key string }
type Service struct {
	Repository Repository
	Storage    Storage
	Validator  *ImageValidator
	Standards  []StandardImage
}

func (s Service) Update(ctx context.Context, userID string, values map[string]string) (Profile, error) {
	var nickname, biography *string
	if n, ok := values["nickname"]; ok {
		normalized, valid := auth.NormalizeNickname(n)
		if !valid {
			return Profile{}, auth.ErrNickname
		}
		nickname = &normalized
	}
	if b, ok := values["biography"]; ok {
		if !utf8.ValidString(b) {
			return Profile{}, ErrInput
		}
		biography = &b
	}
	return s.Repository.Update(ctx, userID, nickname, biography)
}
func validKind(kind string) bool { return kind == "avatar" || kind == "header" }
func (s Service) Upload(ctx context.Context, userID, kind, claimedType string, r io.Reader) (Image, error) {
	if !validKind(kind) {
		return Image{}, ErrInput
	}
	if s.Storage == nil || s.Validator == nil {
		return Image{}, ErrStorage
	}
	data, contentType, err := s.Validator.ReadAndValidate(ctx, r)
	if err != nil {
		return Image{}, err
	}
	if claimedType != contentType {
		return Image{}, ErrImage
	}
	key, err := s.Storage.Put(ctx, data)
	if err != nil {
		return Image{}, ErrStorage
	}
	media, err := s.Repository.SetImage(ctx, userID, kind, key, contentType, int64(len(data)))
	if err != nil {
		if cleanupErr := s.Storage.Delete(ctx, key); cleanupErr != nil {
			slog.Error("unreferenced image cleanup failed", "category", "storage")
		}
		return Image{}, err
	}
	s.Cleanup(ctx, userID)
	return media, nil
}
func (s Service) SelectStandard(ctx context.Context, userID, kind, id string) error {
	if !validKind(kind) {
		return ErrInput
	}
	found := false
	for _, v := range s.Standards {
		if v.ID == id && v.Kind == kind {
			found = true
		}
	}
	if !found {
		return ErrInput
	}
	if err := s.Repository.SelectStandard(ctx, userID, kind, &id); err != nil {
		return err
	}
	s.Cleanup(ctx, userID)
	return nil
}
func (s Service) RemoveImage(ctx context.Context, userID, kind string) error {
	if !validKind(kind) {
		return ErrInput
	}
	if err := s.Repository.SelectStandard(ctx, userID, kind, nil); err != nil {
		return err
	}
	s.Cleanup(ctx, userID)
	return nil
}
func (s Service) Cleanup(ctx context.Context, userID string) {
	if s.Storage == nil {
		return
	}
	rows, err := s.Repository.Unused(ctx, userID)
	if err != nil {
		slog.Error("image cleanup query failed", "category", "storage")
		return
	}
	for _, row := range rows {
		if err = s.Storage.Delete(ctx, row.Key); err != nil {
			slog.Error("image deletion deferred", "category", "storage")
			continue
		}
		if err = s.Repository.DeleteUnused(ctx, row.ID); err != nil {
			slog.Error("image deletion record deferred", "category", "storage")
		}
	}
}
