package auth

import (
	"context"
	"time"

	"tramas/backend/internal/users"
)

type ValidationError struct{ Fields map[string]string }

func (e *ValidationError) Error() string { return "invalid registration input" }

type RegistrationService struct {
	Repository users.Repository
	Hasher     Hasher
	Policy     Policy
}

func (s RegistrationService) Register(ctx context.Context, email, password string) (users.User, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	email, valid := NormalizeEmail(email)
	fields := make(map[string]string)
	if !valid {
		fields["email"] = "invalid"
	}
	if !s.Policy.ValidatePassword(password) {
		fields["password"] = "invalid"
	}
	if len(fields) > 0 {
		return users.User{}, &ValidationError{Fields: fields}
	}
	hash, err := s.Hasher.Hash(ctx, password)
	if err != nil {
		return users.User{}, err
	}
	return s.Repository.Create(ctx, email, hash)
}
