package auth

import (
	"context"
	"errors"
	"github.com/jackc/pgx/v5"
)

// Serialize session creation against password reset and check the exact credential
// verified before hashing. A login racing a reset cannot resurrect an old password.
func (r PostgresAuthRepository) NewPasswordSession(ctx context.Context, id, hash, access, refresh string) (TokenPair, error) {
	tx, err := r.Pool.Begin(ctx)
	if err != nil {
		return TokenPair{}, err
	}
	defer tx.Rollback(ctx)
	var current string
	err = tx.QueryRow(ctx, `SELECT coalesce(password_hash,'') FROM users WHERE id=$1::uuid AND email_verified_at IS NOT NULL FOR SHARE`, id).Scan(&current)
	if errors.Is(err, pgx.ErrNoRows) || err == nil && current != hash {
		return TokenPair{}, ErrCredentials
	}
	if err != nil {
		return TokenPair{}, err
	}
	p := TokenPair{AccessToken: access, RefreshToken: refresh, TokenType: "Bearer"}
	err = tx.QueryRow(ctx, `INSERT INTO auth_sessions(user_id,access_hash,refresh_hash,access_expires_at,refresh_expires_at,absolute_expires_at)
       VALUES($1::uuid,$2,$3,now()+interval '15 minutes',now()+interval '30 days',now()+interval '90 days') RETURNING access_expires_at,refresh_expires_at`, id, tokenHash(access), tokenHash(refresh)).Scan(&p.AccessExpiresAt, &p.RefreshExpiresAt)
	if err != nil {
		return TokenPair{}, err
	}
	if err = tx.Commit(ctx); err != nil {
		return TokenPair{}, err
	}
	return p, nil
}
