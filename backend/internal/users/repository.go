package users

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

type PostgresRepository struct{ Pool *pgxpool.Pool }

func (r PostgresRepository) Create(ctx context.Context, email, hash string) (User, error) {
	var u User
	// One statement is atomic. The database constraint arbitrates concurrent requests.
	err := r.Pool.QueryRow(ctx, `INSERT INTO users (email, password_hash) VALUES ($1, $2)
		RETURNING id::text, email, created_at`, email, hash).Scan(&u.ID, &u.Email, &u.CreatedAt)
	var pgerr *pgconn.PgError
	if errors.As(err, &pgerr) && pgerr.Code == "23505" && pgerr.ConstraintName == "users_email_key" {
		return User{}, ErrEmailExists
	}
	return u, err
}
