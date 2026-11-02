package auth

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

type AuthAction struct {
	Purpose, Provider string
	UserID            *string
	NonceHash         []byte
	ExpiresAt         time.Time
}

func (r PostgresAuthRepository) CreateAction(ctx context.Context, token, purpose, provider, userID, nonce string) error {
	_, err := r.Pool.Exec(ctx, `INSERT INTO auth_actions(token_hash,purpose,provider,user_id,nonce_hash,expires_at)
      VALUES($1,$2,nullif($3,''),nullif($4,'')::uuid,$5,now()+interval '5 minutes')`, tokenHash(token), purpose, provider, userID, tokenHash(nonce))
	return err
}
func (r PostgresAuthRepository) Action(ctx context.Context, token string) (AuthAction, error) {
	var a AuthAction
	err := r.Pool.QueryRow(ctx, `SELECT purpose,coalesce(provider,''),user_id::text,nonce_hash,expires_at FROM auth_actions WHERE token_hash=$1 AND consumed_at IS NULL AND expires_at>now()`, tokenHash(token)).Scan(&a.Purpose, &a.Provider, &a.UserID, &a.NonceHash, &a.ExpiresAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return a, ErrOIDC
	}
	return a, err
}
func (r PostgresAuthRepository) ConsumeProof(ctx context.Context, token, userID string) error {
	tag, err := r.Pool.Exec(ctx, `UPDATE auth_actions SET consumed_at=now() WHERE token_hash=$1 AND purpose='reauth_proof' AND user_id=$2::uuid AND consumed_at IS NULL AND expires_at>now()`, tokenHash(token), userID)
	if err == nil && tag.RowsAffected() != 1 {
		return ErrCredentials
	}
	return err
}

func (r PostgresAuthRepository) CreateLinkAction(ctx context.Context, token, provider, userID, nonce, proof string) error {
	tx, err := r.Pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	var id string
	if err = tx.QueryRow(ctx, `SELECT id::text FROM users WHERE id=$1::uuid FOR UPDATE`, userID).Scan(&id); err != nil {
		return err
	}
	tag, err := tx.Exec(ctx, `UPDATE auth_actions SET consumed_at=now() WHERE token_hash=$1 AND purpose='reauth_proof' AND user_id=$2::uuid AND consumed_at IS NULL AND expires_at>now()`, tokenHash(proof), userID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() != 1 {
		return ErrCredentials
	}
	_, err = tx.Exec(ctx, `INSERT INTO auth_actions(token_hash,purpose,provider,user_id,nonce_hash,expires_at) VALUES($1,'oidc_link',$2,$3::uuid,$4,now()+interval '5 minutes')`, tokenHash(token), provider, userID, tokenHash(nonce))
	if err != nil {
		return err
	}
	return tx.Commit(ctx)
}
func (r PostgresAuthRepository) CreatePasswordProof(ctx context.Context, token, userID, expectedHash string) error {
	tx, err := r.Pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	var hash string
	if err = tx.QueryRow(ctx, `SELECT coalesce(password_hash,'') FROM users WHERE id=$1::uuid FOR SHARE`, userID).Scan(&hash); err != nil {
		return err
	}
	if hash != expectedHash {
		return ErrCredentials
	}
	if _, err = tx.Exec(ctx, `INSERT INTO auth_actions(token_hash,purpose,user_id,expires_at) VALUES($1,'reauth_proof',$2::uuid,now()+interval '5 minutes')`, tokenHash(token), userID); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// Subject uniqueness, rather than provider email, arbitrates concurrent sign-ins.
func (r PostgresAuthRepository) CompleteIdentity(ctx context.Context, challenge, purpose, userID string, identity ExternalIdentity) (Account, error) {
	tx, err := r.Pool.Begin(ctx)
	if err != nil {
		return Account{}, err
	}
	defer tx.Rollback(ctx)
	if purpose != "oidc_login" {
		var id string
		if err = tx.QueryRow(ctx, `SELECT id::text FROM users WHERE id=$1::uuid FOR UPDATE`, userID).Scan(&id); err != nil {
			return Account{}, err
		}
	}
	var actionUser *string
	err = tx.QueryRow(ctx, `UPDATE auth_actions SET consumed_at=now() WHERE token_hash=$1 AND purpose=$2 AND provider=$3
      AND consumed_at IS NULL AND expires_at>now() RETURNING user_id::text`, tokenHash(challenge), purpose, identity.Provider).Scan(&actionUser)
	if errors.Is(err, pgx.ErrNoRows) {
		return Account{}, ErrOIDC
	}
	if err != nil {
		return Account{}, err
	}
	if purpose != "oidc_login" && (actionUser == nil || *actionUser != userID) {
		return Account{}, ErrOIDC
	}
	// Serialize create/link for this stable provider subject across replicas.
	if _, err = tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1,0))`, identity.Provider+"/"+identity.Subject); err != nil {
		return Account{}, err
	}
	var existing string
	err = tx.QueryRow(ctx, `SELECT user_id::text FROM auth_identities WHERE provider=$1 AND subject=$2`, identity.Provider, identity.Subject).Scan(&existing)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return Account{}, err
	}
	switch purpose {
	case "oidc_login":
		if existing == "" {
			if err = tx.QueryRow(ctx, `INSERT INTO users(email,password_hash) VALUES(NULL,NULL) RETURNING id::text`).Scan(&existing); err != nil {
				return Account{}, err
			}
			if _, err = tx.Exec(ctx, `INSERT INTO auth_identities(provider,subject,user_id,provider_email) VALUES($1,$2,$3::uuid,nullif($4,''))`, identity.Provider, identity.Subject, existing, identity.Email); err != nil {
				return Account{}, err
			}
		}
		userID = existing
	case "oidc_link":
		if existing != "" && existing != userID {
			return Account{}, ErrIdentityLinked
		}
		if existing == "" {
			_, err = tx.Exec(ctx, `INSERT INTO auth_identities(provider,subject,user_id,provider_email) VALUES($1,$2,$3::uuid,nullif($4,''))`, identity.Provider, identity.Subject, userID, identity.Email)
			var pgerr *pgconn.PgError
			if errors.As(err, &pgerr) && pgerr.Code == "23505" {
				return Account{}, ErrIdentityLinked
			}
			if err != nil {
				return Account{}, err
			}
		}
		if _, err = tx.Exec(ctx, `INSERT INTO auth_audit_events(user_id,event) VALUES($1::uuid,'identity_linked')`, userID); err != nil {
			return Account{}, err
		}
	case "oidc_reauth":
		if existing == "" || existing != userID {
			return Account{}, ErrCredentials
		}
	default:
		return Account{}, ErrOIDC
	}
	a, err := scanAccount(tx.QueryRow(ctx, `SELECT `+accountColumns+` FROM users WHERE id=$1::uuid`, userID))
	if err != nil {
		return Account{}, err
	}
	if err = tx.Commit(ctx); err != nil {
		return Account{}, err
	}
	return a, nil
}
