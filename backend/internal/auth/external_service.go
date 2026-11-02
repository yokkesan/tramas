package auth

import (
	"context"
	"time"
)

type ExternalRepository interface {
	CreateAction(context.Context, string, string, string, string, string) error
	Action(context.Context, string) (AuthAction, error)
	ConsumeProof(context.Context, string, string) error
	CreateLinkAction(context.Context, string, string, string, string, string) error
	CreatePasswordProof(context.Context, string, string, string) error
	CompleteIdentity(context.Context, string, string, string, ExternalIdentity) (Account, error)
}
type ExternalService struct {
	Repository ExternalRepository
	Sessions   *SessionService
	Verifier   IdentityVerifier
}
type Challenge struct {
	Token     string `json:"challenge_token"`
	Nonce     string `json:"nonce"`
	ExpiresIn int    `json:"expires_in"`
}

func (s ExternalService) Challenge(ctx context.Context, provider, purpose, userID, proof string) (Challenge, error) {
	if provider != "apple" && provider != "google" {
		return Challenge{}, ErrOIDC
	}
	if enabled, ok := s.Verifier.(interface{ Enabled(string) bool }); ok && !enabled.Enabled(provider) {
		return Challenge{}, ErrProviderDisabled
	}
	if purpose != "login" && purpose != "link" && purpose != "reauth" {
		return Challenge{}, ErrOIDC
	}
	if purpose != "login" && userID == "" {
		return Challenge{}, ErrCredentials
	}
	if purpose == "link" {
		if !validToken(proof) {
			return Challenge{}, ErrCredentials
		}
	}
	token, err := newToken()
	if err != nil {
		return Challenge{}, err
	}
	nonce, err := newToken()
	if err != nil {
		return Challenge{}, err
	}
	if purpose == "login" {
		userID = ""
	}
	if purpose == "link" {
		err = s.Repository.CreateLinkAction(ctx, token, provider, userID, nonce, proof)
	} else {
		err = s.Repository.CreateAction(ctx, token, "oidc_"+purpose, provider, userID, nonce)
	}
	if err != nil {
		return Challenge{}, err
	}
	return Challenge{token, nonce, 300}, nil
}
func (s ExternalService) Complete(ctx context.Context, provider, purpose, userID, challenge, raw string) (Account, TokenPair, string, error) {
	if !validToken(challenge) {
		return Account{}, TokenPair{}, "", ErrOIDC
	}
	a, err := s.Repository.Action(ctx, challenge)
	if err != nil {
		return Account{}, TokenPair{}, "", err
	}
	if a.Purpose != "oidc_"+purpose || a.Provider != provider {
		return Account{}, TokenPair{}, "", ErrOIDC
	}
	if purpose != "login" && (a.UserID == nil || *a.UserID != userID) {
		return Account{}, TokenPair{}, "", ErrOIDC
	}
	identity, err := s.Verifier.Verify(ctx, provider, raw, a.NonceHash)
	if err != nil {
		return Account{}, TokenPair{}, "", err
	}
	account, err := s.Repository.CompleteIdentity(ctx, challenge, a.Purpose, userID, identity)
	if err != nil {
		return Account{}, TokenPair{}, "", err
	}
	if purpose == "reauth" {
		proof, err := s.proof(ctx, account.ID)
		return account, TokenPair{}, proof, err
	}
	if purpose == "link" {
		return account, TokenPair{}, "", nil
	}
	access, err := newToken()
	if err != nil {
		return Account{}, TokenPair{}, "", err
	}
	refresh, err := newToken()
	if err != nil {
		return Account{}, TokenPair{}, "", err
	}
	pair, err := s.Sessions.Repository.NewSession(ctx, account.ID, access, refresh)
	return account, pair, "", err
}
func (s ExternalService) proof(ctx context.Context, userID string) (string, error) {
	token, err := newToken()
	if err != nil {
		return "", err
	}
	err = s.Repository.CreateAction(ctx, token, "reauth_proof", "", userID, "")
	return token, err
}
func (s ExternalService) Reauthenticate(ctx context.Context, a Account, password string) (string, error) {
	if len(password) > 4096 {
		return "", ErrCredentials
	}
	ok, err := s.Sessions.Repository.Allow(ctx, "reauth/"+a.ID, 5, 15*time.Minute)
	if err != nil {
		return "", err
	}
	if !ok {
		return "", ErrRateLimit
	}
	credential, err := s.Sessions.Repository.Credentials(ctx, a.Email)
	if err != nil {
		return "", err
	}
	if credential.Account.ID != a.ID || credential.Hash == "" {
		return "", ErrCredentials
	}
	matched, err := s.Sessions.Hasher.Verify(ctx, password, credential.Hash)
	if err != nil {
		return "", err
	}
	if !matched {
		return "", ErrCredentials
	}
	proof, err := newToken()
	if err != nil {
		return "", err
	}
	err = s.Repository.CreatePasswordProof(ctx, proof, a.ID, credential.Hash)
	return proof, err
}
