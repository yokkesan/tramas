package auth

import (
	"context"
	"errors"
	"log/slog"
	"sync"
	"time"

	"github.com/jackc/pgx/v5"
)

type VerificationMail struct {
	Email     string    `json:"email"`
	Token     string    `json:"token"`
	ExpiresAt time.Time `json:"expires_at"`
	Kind      string    `json:"kind,omitempty"`
}

// Sender owns presentation, link building and external delivery. Tokens must never
// be logged. A production adapter must use authenticated, encrypted transport.
type VerificationSender interface {
	SendVerification(context.Context, VerificationMail) error
}
type PasswordResetSender interface {
	SendPasswordReset(context.Context, VerificationMail) error
}

func (m *MemoryMailbox) SendPasswordReset(ctx context.Context, v VerificationMail) error {
	v.Kind = "password_reset"
	return m.SendVerification(ctx, v)
}

// MemoryMailbox is for development/tests only. Nothing is written to disk or logs.
// Capacity is bounded; full mailboxes cause retryable delivery failures.
type MemoryMailbox struct {
	mu       sync.Mutex
	messages []VerificationMail
	Capacity int
}

func (m *MemoryMailbox) SendVerification(ctx context.Context, v VerificationMail) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	now := time.Now()
	kept := m.messages[:0]
	for _, v := range m.messages {
		if now.Before(v.ExpiresAt) {
			kept = append(kept, v)
		}
	}
	m.messages = kept
	capacity := m.Capacity
	if capacity <= 0 {
		capacity = 100
	}
	if len(m.messages) >= capacity {
		return errors.New("local mailbox full")
	}
	m.messages = append(m.messages, v)
	return nil
}

func (m *MemoryMailbox) Messages() []VerificationMail {
	m.mu.Lock()
	defer m.mu.Unlock()
	return append([]VerificationMail(nil), m.messages...)
}

// DeliverOne locks a job to prevent duplicate dispatchers. Network calls must have
// a short deadline. An existing token is preserved if sending fails.
func (r PostgresAuthRepository) DeliverOne(ctx context.Context, sender VerificationSender) (bool, error) {
	if sender == nil {
		return false, errors.New("mail sender not configured")
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	tx, err := r.Pool.Begin(ctx)
	if err != nil {
		return false, err
	}
	defer tx.Rollback(ctx)
	var id, email string
	var attempts int
	err = tx.QueryRow(ctx, `SELECT v.user_id::text,u.email,v.attempts FROM auth_email_verifications v JOIN users u ON u.id=v.user_id
        WHERE v.pending AND v.next_attempt_at<=now() AND u.email_verified_at IS NULL
        ORDER BY v.next_attempt_at FOR UPDATE OF v SKIP LOCKED LIMIT 1`).Scan(&id, &email, &attempts)
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
	if err = tx.QueryRow(ctx, `SELECT now()+interval '24 hours'`).Scan(&expires); err != nil {
		return false, err
	}
	sendCtx, stop := context.WithTimeout(ctx, 3*time.Second)
	sendErr := sender.SendVerification(sendCtx, VerificationMail{Email: email, Token: token, ExpiresAt: expires, Kind: "email_verification"})
	stop()
	if sendErr != nil {
		// Do not store or return sender errors (may contain credentials or content).
		_, err = tx.Exec(ctx, `UPDATE auth_email_verifications SET attempts=attempts+1,pending=(attempts+1<5),
            next_attempt_at=now()+interval '60 seconds' * power(2,least(attempts,4)) WHERE user_id=$1::uuid`, id)
	} else {
		_, err = tx.Exec(ctx, `UPDATE auth_email_verifications SET token_hash=$2,expires_at=$3,consumed_at=NULL,pending=false,attempts=0 WHERE user_id=$1::uuid`, id, tokenHash(token), expires)
	}
	if err != nil {
		return false, err
	}
	if err = tx.Commit(ctx); err != nil {
		return false, err
	}
	if sendErr != nil {
		slog.Warn("verification delivery deferred", "category", "delivery")
	}
	return true, nil
}

// RunBackground is called by the API owning the local memory mailbox. All work is
// durable in PostgreSQL; a future dedicated worker can use the same methods.
func (r PostgresAuthRepository) RunBackground(ctx context.Context, sender VerificationSender) {
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	maintenance := time.NewTicker(time.Minute)
	defer maintenance.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if sender == nil {
				continue
			}
			for range 10 {
				done, err := r.DeliverOne(ctx, sender)
				if err != nil {
					slog.Error("verification job failed", "category", "internal")
					break
				}
				if !done {
					break
				}
			}
			if resetSender, ok := sender.(PasswordResetSender); ok {
				for range 10 {
					done, err := r.DeliverResetOne(ctx, resetSender)
					if err != nil {
						slog.Error("password reset job failed", "category", "internal")
						break
					}
					if !done {
						break
					}
				}
			}
		case <-maintenance.C:
			work, cancel := context.WithTimeout(ctx, 5*time.Second)
			if err := r.Maintain(work); err != nil {
				slog.Error("auth maintenance failed", "category", "internal")
			}
			cancel()
		}
	}
}
