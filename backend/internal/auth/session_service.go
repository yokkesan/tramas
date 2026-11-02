package auth

import (
	"context"
	"errors"
	"time"
	"unicode/utf8"
)

var (
	ErrCredentials     = errors.New("invalid credentials")
	ErrUnverified      = errors.New("email verification required")
	ErrToken           = errors.New("invalid or expired token")
	ErrRefreshConflict = errors.New("refresh conflict")
	ErrRateLimit       = errors.New("rate limited")
	ErrNickname        = errors.New("invalid nickname")
)

const (
	AccessTTL       = 15 * time.Minute
	RefreshTTL      = 30 * 24 * time.Hour
	SessionTTL      = 90 * 24 * time.Hour
	RefreshRetryTTL = time.Minute
	VerificationTTL = 24 * time.Hour
)

type Account struct {
	ID               string  `json:"id"`
	Email            string  `json:"email"`
	Nickname         *string `json:"nickname"`
	EmailVerified    bool    `json:"email_verified"`
	ProfileCompleted bool    `json:"profile_completed"`
}

type Credentials struct {
	Account Account
	Hash    string
}
type TokenPair struct {
	AccessToken      string    `json:"access_token"`
	RefreshToken     string    `json:"refresh_token"`
	TokenType        string    `json:"token_type"`
	AccessExpiresAt  time.Time `json:"access_expires_at"`
	RefreshExpiresAt time.Time `json:"refresh_expires_at"`
}

type AuthRepository interface {
	Credentials(context.Context, string) (Credentials, error)
	NewSession(context.Context, string, string, string) (TokenPair, error)
	Rotate(context.Context, string, string, string, string) (TokenPair, error)
	Authenticate(context.Context, string) (Account, error)
	Logout(context.Context, string) error
	VerifyEmail(context.Context, string) error
	Resend(context.Context, string) error
	Profile(context.Context, string, string) (Account, error)
	Allow(context.Context, string, int, time.Duration) (bool, error)
}

type SessionService struct {
	Repository AuthRepository
	Hasher     *ArgonHasher
	dummyHash  string
}

func NewSessionService(r AuthRepository, h *ArgonHasher) (*SessionService, error) {
	dummy, err := h.Hash(context.Background(), "tramas nonexistent credential timing placeholder")
	if err != nil {
		return nil, err
	}
	return &SessionService{Repository: r, Hasher: h, dummyHash: dummy}, nil
}

func (s *SessionService) Login(ctx context.Context, email, password string) (Account, TokenPair, error) {
	email, valid := NormalizeEmail(email)
	if !valid || !utf8.ValidString(password) || len(password) > 4096 || password == "" {
		return Account{}, TokenPair{}, ErrCredentials
	}
	allowed, err := s.Repository.Allow(ctx, "login/account/"+email, 5, 15*time.Minute)
	if err != nil {
		return Account{}, TokenPair{}, err
	}
	if !allowed {
		return Account{}, TokenPair{}, ErrRateLimit
	}
	c, err := s.Repository.Credentials(ctx, email)
	if err != nil {
		return Account{}, TokenPair{}, err
	}
	hash := c.Hash
	if hash == "" {
		hash = s.dummyHash
	}
	match, err := s.Hasher.Verify(ctx, password, hash)
	if err != nil {
		return Account{}, TokenPair{}, err
	}
	if !match || c.Hash == "" {
		return Account{}, TokenPair{}, ErrCredentials
	}
	if !c.Account.EmailVerified {
		return Account{}, TokenPair{}, ErrUnverified
	}
	access, err := newToken()
	if err != nil {
		return Account{}, TokenPair{}, err
	}
	refresh, err := newToken()
	if err != nil {
		return Account{}, TokenPair{}, err
	}
	var pair TokenPair
	if secured, ok := s.Repository.(interface {
		NewPasswordSession(context.Context, string, string, string, string) (TokenPair, error)
	}); ok {
		pair, err = secured.NewPasswordSession(ctx, c.Account.ID, c.Hash, access, refresh)
	} else {
		pair, err = s.Repository.NewSession(ctx, c.Account.ID, access, refresh)
	}
	return c.Account, pair, err
}

func (s *SessionService) Refresh(ctx context.Context, old, requestID string) (TokenPair, error) {
	if !validToken(old) || !validToken(requestID) {
		return TokenPair{}, ErrToken
	}
	access, refresh := rotatedTokens(old, requestID)
	return s.Repository.Rotate(ctx, old, requestID, access, refresh)
}

func (s *SessionService) Authenticate(ctx context.Context, token string) (Account, error) {
	if !validToken(token) {
		return Account{}, ErrToken
	}
	return s.Repository.Authenticate(ctx, token)
}

func (s *SessionService) Logout(ctx context.Context, token string) error {
	if !validToken(token) {
		return ErrToken
	}
	return s.Repository.Logout(ctx, token)
}

func (s *SessionService) VerifyEmail(ctx context.Context, token string) error {
	if !validToken(token) {
		return ErrToken
	}
	return s.Repository.VerifyEmail(ctx, token)
}

func (s *SessionService) Resend(ctx context.Context, email string) error {
	email, valid := NormalizeEmail(email)
	if !valid {
		return nil
	}
	// Identical responses even when an address is unknown or throttled.
	allowed, err := s.Repository.Allow(ctx, "resend/minute/"+email, 1, time.Minute)
	if err != nil || !allowed {
		return err
	}
	allowed, err = s.Repository.Allow(ctx, "resend/hour/"+email, 5, time.Hour)
	if err != nil || !allowed {
		return err
	}
	return s.Repository.Resend(ctx, email)
}

func (s *SessionService) Profile(ctx context.Context, id, nickname string) (Account, error) {
	nickname, valid := NormalizeNickname(nickname)
	if !valid {
		return Account{}, ErrNickname
	}
	return s.Repository.Profile(ctx, id, nickname)
}
