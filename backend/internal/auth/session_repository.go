package auth

import (
	"bytes"
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type PostgresAuthRepository struct {
	Pool     *pgxpool.Pool
	ResetTTL time.Duration
}

const accountColumns = `id::text,coalesce(email,''),nickname,email_verified_at IS NOT NULL,profile_completed_at IS NOT NULL`

func scanAccount(row pgx.Row) (Account, error) {
	var a Account
	err := row.Scan(&a.ID, &a.Email, &a.Nickname, &a.EmailVerified, &a.ProfileCompleted)
	return a, err
}

func (r PostgresAuthRepository) Credentials(ctx context.Context, email string) (Credentials, error) {
	var c Credentials
	err := r.Pool.QueryRow(ctx, `SELECT `+accountColumns+`,coalesce(password_hash,'') FROM users WHERE email=$1`, email).Scan(
		&c.Account.ID, &c.Account.Email, &c.Account.Nickname, &c.Account.EmailVerified, &c.Account.ProfileCompleted, &c.Hash)
	if errors.Is(err, pgx.ErrNoRows) {
		return Credentials{}, nil
	}
	return c, err
}

func (r PostgresAuthRepository) NewSession(ctx context.Context, id, access, refresh string) (TokenPair, error) {
	p := TokenPair{AccessToken: access, RefreshToken: refresh, TokenType: "Bearer"}
	err := r.Pool.QueryRow(ctx, `INSERT INTO auth_sessions(user_id,access_hash,refresh_hash,access_expires_at,refresh_expires_at,absolute_expires_at)
        SELECT id,$2,$3,now()+interval '15 minutes',now()+interval '30 days',now()+interval '90 days'
        FROM users WHERE id=$1::uuid AND (email_verified_at IS NOT NULL OR EXISTS(SELECT 1 FROM auth_identities WHERE user_id=users.id))
        RETURNING access_expires_at,refresh_expires_at`, id, tokenHash(access), tokenHash(refresh)).Scan(&p.AccessExpiresAt, &p.RefreshExpiresAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return TokenPair{}, ErrUnverified
	}
	return p, err
}

func (r PostgresAuthRepository) Rotate(ctx context.Context, old, requestID, access, refresh string) (TokenPair, error) {
	tx, err := r.Pool.Begin(ctx)
	if err != nil {
		return TokenPair{}, err
	}
	defer tx.Rollback(ctx)
	var id string
	var current, previous, requestHash []byte
	var retryUntil *time.Time
	var accessExpiry, refreshExpiry, absoluteExpiry, now time.Time
	err = tx.QueryRow(ctx, `SELECT id::text,refresh_hash,previous_refresh_hash,refresh_request_hash,retry_until,
        access_expires_at,refresh_expires_at,absolute_expires_at,now() FROM auth_sessions
        WHERE (refresh_hash=$1 OR previous_refresh_hash=$1) AND revoked_at IS NULL FOR UPDATE`, tokenHash(old)).Scan(
		&id, &current, &previous, &requestHash, &retryUntil, &accessExpiry, &refreshExpiry, &absoluteExpiry, &now)
	if errors.Is(err, pgx.ErrNoRows) {
		return TokenPair{}, ErrToken
	}
	if err != nil {
		return TokenPair{}, err
	}
	if !now.Before(refreshExpiry) || !now.Before(absoluteExpiry) {
		return TokenPair{}, ErrToken
	}
	if bytes.Equal(previous, tokenHash(old)) {
		if retryUntil == nil || !now.Before(*retryUntil) {
			return TokenPair{}, ErrToken
		}
		if !bytes.Equal(requestHash, tokenHash(requestID)) {
			return TokenPair{}, ErrRefreshConflict
		}
		// The returned credentials must still be the current pair.
		if !bytes.Equal(current, tokenHash(refresh)) {
			return TokenPair{}, ErrToken
		}
	} else {
		err = tx.QueryRow(ctx, `UPDATE auth_sessions SET previous_refresh_hash=refresh_hash,refresh_request_hash=$2,
            refresh_hash=$3,access_hash=$4,retry_until=now()+interval '60 seconds',
            access_expires_at=least(now()+interval '15 minutes',absolute_expires_at),
            refresh_expires_at=least(now()+interval '30 days',absolute_expires_at)
            WHERE id=$1::uuid RETURNING access_expires_at,refresh_expires_at`, id, tokenHash(requestID), tokenHash(refresh), tokenHash(access)).Scan(&accessExpiry, &refreshExpiry)
		if err != nil {
			return TokenPair{}, err
		}
	}
	if err = tx.Commit(ctx); err != nil {
		return TokenPair{}, err
	}
	return TokenPair{access, refresh, "Bearer", accessExpiry, refreshExpiry}, nil
}

func (r PostgresAuthRepository) Authenticate(ctx context.Context, token string) (Account, error) {
	a, err := scanAccount(r.Pool.QueryRow(ctx, `SELECT u.id::text,coalesce(u.email,''),u.nickname,u.email_verified_at IS NOT NULL,u.profile_completed_at IS NOT NULL
        FROM users u JOIN auth_sessions s ON s.user_id=u.id WHERE s.access_hash=$1 AND s.revoked_at IS NULL
        AND s.access_expires_at>now() AND s.refresh_expires_at>now() AND s.absolute_expires_at>now()
        AND (u.email_verified_at IS NOT NULL OR EXISTS(SELECT 1 FROM auth_identities WHERE user_id=u.id))`, tokenHash(token)))
	if errors.Is(err, pgx.ErrNoRows) {
		return Account{}, ErrToken
	}
	return a, err
}

func (r PostgresAuthRepository) Logout(ctx context.Context, token string) error {
	_, err := r.Pool.Exec(ctx, `UPDATE auth_sessions SET revoked_at=coalesce(revoked_at,now()) WHERE refresh_hash=$1
        OR (previous_refresh_hash=$1 AND retry_until>now())`, tokenHash(token))
	return err
}

func (r PostgresAuthRepository) VerifyEmail(ctx context.Context, token string) error {
	tx, err := r.Pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	var id string
	err = tx.QueryRow(ctx, `UPDATE auth_email_verifications SET consumed_at=now(),pending=false
        WHERE token_hash=$1 AND consumed_at IS NULL AND expires_at>now() RETURNING user_id::text`, tokenHash(token)).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrToken
	}
	if err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, `UPDATE users SET email_verified_at=coalesce(email_verified_at,now()),updated_at=now() WHERE id=$1::uuid`, id); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (r PostgresAuthRepository) Resend(ctx context.Context, email string) error {
	_, err := r.Pool.Exec(ctx, `INSERT INTO auth_email_verifications(user_id)
        SELECT id FROM users WHERE email=$1 AND email_verified_at IS NULL
        ON CONFLICT(user_id) DO UPDATE SET pending=true,attempts=0,next_attempt_at=now()
        WHERE NOT auth_email_verifications.pending`, email)
	return err
}

func (r PostgresAuthRepository) Profile(ctx context.Context, id, nickname string) (Account, error) {
	return scanAccount(r.Pool.QueryRow(ctx, `UPDATE users SET nickname=$2,profile_completed_at=coalesce(profile_completed_at,now()),updated_at=now()
        WHERE id=$1::uuid AND (email_verified_at IS NOT NULL OR EXISTS(SELECT 1 FROM auth_identities WHERE user_id=users.id)) RETURNING `+accountColumns, id, nickname))
}

func (r PostgresAuthRepository) Allow(ctx context.Context, key string, limit int, window time.Duration) (bool, error) {
	var hits int
	err := r.Pool.QueryRow(ctx, `INSERT INTO auth_rate_limits(key,hits,expires_at) VALUES($1,1,now()+make_interval(secs=>$2))
        ON CONFLICT(key) DO UPDATE SET hits=CASE WHEN auth_rate_limits.expires_at<=now() THEN 1 ELSE least(auth_rate_limits.hits+1,$3+1) END,
        expires_at=CASE WHEN auth_rate_limits.expires_at<=now() THEN excluded.expires_at ELSE auth_rate_limits.expires_at END RETURNING hits`,
		tokenHash(key), window.Seconds(), limit).Scan(&hits)
	return hits <= limit, err
}

// Maintenance is bounded; expired rows cannot grow indefinitely under normal load.
func (r PostgresAuthRepository) Maintain(ctx context.Context) error {
	_, err := r.Pool.Exec(ctx, `DELETE FROM auth_rate_limits WHERE key IN (SELECT key FROM auth_rate_limits WHERE expires_at<now() LIMIT 1000)`)
	if err != nil {
		return err
	}
	_, err = r.Pool.Exec(ctx, `DELETE FROM auth_sessions WHERE id IN (SELECT id FROM auth_sessions
        WHERE absolute_expires_at<now() OR refresh_expires_at<now() OR revoked_at<now()-interval '1 day' LIMIT 1000)`)
	if err != nil {
		return err
	}
	_, err = r.Pool.Exec(ctx, `DELETE FROM auth_actions WHERE token_hash IN (SELECT token_hash FROM auth_actions WHERE expires_at<now() LIMIT 1000)`)
	return err
}
