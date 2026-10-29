package users

import (
	"context"
	"errors"
	"time"
)

var ErrEmailExists = errors.New("email already registered")

// User deliberately contains neither credentials nor administrator privileges.
type User struct {
	ID        string    `json:"id"`
	Email     string    `json:"email"`
	CreatedAt time.Time `json:"created_at"`
}

type Repository interface {
	Create(context.Context, string, string) (User, error)
}
