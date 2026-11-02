package auth

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/labstack/echo/v5"
)

var ErrPasswordPolicy = errors.New("password policy violation")

type ResetRepository interface {
	RequestReset(context.Context, string) error
	LiveReset(context.Context, string) (bool, error)
	CompleteReset(context.Context, string, string) error
	Audit(context.Context, string, string) error
}
type ResetService struct {
	Repository ResetRepository
	Sessions   *SessionService
	Policy     Policy
}

func (s ResetService) Request(ctx context.Context, email string) error {
	email, ok := NormalizeEmail(email)
	if !ok {
		return nil
	}
	for _, limit := range []struct {
		scope  string
		max    int
		window time.Duration
	}{{"minute", 1, time.Minute}, {"hour", 5, time.Hour}} {
		allowed, err := s.Sessions.Repository.Allow(ctx, "reset/"+limit.scope+"/"+email, limit.max, limit.window)
		if err != nil || !allowed {
			return err
		}
	}
	return s.Repository.RequestReset(ctx, email)
}
func (s ResetService) Complete(ctx context.Context, token, password string) error {
	if !validToken(token) {
		return s.rejected(ctx, ErrToken)
	}
	if !s.Policy.ValidatePassword(password) {
		return ErrPasswordPolicy
	}
	live, err := s.Repository.LiveReset(ctx, token)
	if err != nil {
		return err
	}
	if !live {
		return s.rejected(ctx, ErrToken)
	}
	hash, err := s.Sessions.Hasher.Hash(ctx, password)
	if err != nil {
		return err
	}
	if err = s.Repository.CompleteReset(ctx, token, hash); err != nil {
		return s.rejected(ctx, err)
	}
	slog.Info("password reset completed", "category", "audit")
	return nil
}
func (s ResetService) rejected(ctx context.Context, reason error) error {
	if err := s.Repository.Audit(ctx, "", "password_reset_rejected"); err != nil {
		return err
	}
	return reason
}
func (r PostgresAuthRepository) RequestReset(ctx context.Context, email string) error {
	tx, err := r.Pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	var id string
	err = tx.QueryRow(ctx, `SELECT id::text FROM users WHERE email=$1 AND password_hash IS NOT NULL`, email).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `INSERT INTO auth_password_resets(user_id) VALUES($1::uuid) ON CONFLICT(user_id)
       DO UPDATE SET pending=true,attempts=0,next_attempt_at=now() WHERE NOT auth_password_resets.pending`, id)
	if err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, `INSERT INTO auth_audit_events(user_id,event) VALUES($1::uuid,'password_reset_requested')`, id); err != nil {
		return err
	}
	return tx.Commit(ctx)
}
func (r PostgresAuthRepository) LiveReset(ctx context.Context, token string) (bool, error) {
	var live bool
	err := r.Pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM auth_password_resets WHERE token_hash=$1 AND expires_at>now() AND consumed_at IS NULL)`, tokenHash(token)).Scan(&live)
	return live, err
}
func (r PostgresAuthRepository) CompleteReset(ctx context.Context, token, hash string) error {
	var id *string
	if err := r.Pool.QueryRow(ctx, `SELECT complete_password_reset($1,$2)`, tokenHash(token), hash).Scan(&id); err != nil {
		return err
	}
	if id == nil {
		return ErrToken
	}
	return nil
}
func (r PostgresAuthRepository) Audit(ctx context.Context, userID, event string) error {
	_, err := r.Pool.Exec(ctx, `INSERT INTO auth_audit_events(user_id,event) VALUES(nullif($1,'')::uuid,$2)`, userID, event)
	return err
}
func (r PostgresAuthRepository) DeliverResetOne(ctx context.Context, sender PasswordResetSender) (bool, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	tx, err := r.Pool.Begin(ctx)
	if err != nil {
		return false, err
	}
	defer tx.Rollback(ctx)
	var id, email string
	err = tx.QueryRow(ctx, `SELECT v.user_id::text,u.email FROM auth_password_resets v JOIN users u ON u.id=v.user_id
       WHERE v.pending AND v.next_attempt_at<=now() AND u.password_hash IS NOT NULL ORDER BY v.next_attempt_at FOR UPDATE OF v SKIP LOCKED LIMIT 1`).Scan(&id, &email)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	token, err := newToken()
	if err != nil {
		return false, err
	}
	var expires time.Time
	ttl := r.ResetTTL
	if ttl <= 0 {
		ttl = 30 * time.Minute
	}
	if err = tx.QueryRow(ctx, `SELECT now()+make_interval(secs=>$1)`, ttl.Seconds()).Scan(&expires); err != nil {
		return false, err
	}
	sendCtx, stop := context.WithTimeout(ctx, 3*time.Second)
	sendErr := sender.SendPasswordReset(sendCtx, VerificationMail{Email: email, Token: token, ExpiresAt: expires, Kind: "password_reset"})
	stop()
	if sendErr != nil {
		_, err = tx.Exec(ctx, `UPDATE auth_password_resets SET attempts=attempts+1,pending=(attempts+1<5),next_attempt_at=now()+interval '60 seconds'*power(2,least(attempts,4)) WHERE user_id=$1::uuid`, id)
	} else {
		_, err = tx.Exec(ctx, `UPDATE auth_password_resets SET token_hash=$2,expires_at=$3,consumed_at=NULL,pending=false,attempts=0 WHERE user_id=$1::uuid`, id, tokenHash(token), expires)
	}
	if err != nil {
		return false, err
	}
	if err = tx.Commit(ctx); err != nil {
		return false, err
	}
	if sendErr != nil {
		slog.Warn("password reset delivery deferred", "category", "delivery")
	}
	return true, nil
}
func RegisterReset(e *echo.Echo, h *SessionHandler, s ResetService) {
	e.POST("/api/v1/auth/password/reset/request", func(c *echo.Context) error {
		v, err := readFields(c, "email")
		if err != nil {
			return authFailure(c, err)
		}
		if err = s.Request(c.Request().Context(), v["email"]); err != nil {
			return authFailure(c, err)
		}
		return c.NoContent(202)
	}, h.Guard)
	e.POST("/api/v1/auth/password/reset/complete", func(c *echo.Context) error {
		v, err := readFields(c, "token", "password")
		if err != nil {
			return authFailure(c, err)
		}
		if err = s.Complete(c.Request().Context(), v["token"], v["password"]); err != nil {
			return authFailure(c, err)
		}
		return c.NoContent(204)
	}, h.Guard)
}
