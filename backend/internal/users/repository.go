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
	tx, err := r.Pool.Begin(ctx)
	if err != nil {
		return u, err
	}
	defer tx.Rollback(ctx)
	// Account creation and the delivery request must commit together.
	err = tx.QueryRow(ctx, `INSERT INTO users (email, password_hash) VALUES ($1, $2)
		RETURNING id::text, email, created_at`, email, hash).Scan(&u.ID, &u.Email, &u.CreatedAt)
	var pgerr *pgconn.PgError
	if errors.As(err, &pgerr) && pgerr.Code == "23505" && pgerr.ConstraintName == "users_email_key" {
		return User{}, ErrEmailExists
	}
	if err != nil {
		return User{}, err
	}
	if _, err = tx.Exec(ctx, `INSERT INTO auth_email_verifications(user_id) VALUES($1::uuid)`, u.ID); err != nil {
		return User{}, err
	}
	if err = tx.Commit(ctx); err != nil {
		return User{}, err
	}
	return u, nil
}
