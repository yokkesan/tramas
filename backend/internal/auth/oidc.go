package auth

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/coreos/go-oidc/v3/oidc"
)

var ErrOIDC = errors.New("external authentication rejected")
var ErrProviderDisabled = errors.New("external provider not configured")
var ErrIdentityLinked = errors.New("identity already linked")

type ExternalIdentity struct{ Provider, Subject, Email string }
type IdentityVerifier interface {
	Verify(context.Context, string, string, []byte) (ExternalIdentity, error)
}

type OIDCVerifier struct {
	Verifiers map[string][]*oidc.IDTokenVerifier
	ClientIDs map[string][]string
}

// Production endpoints are fixed, not selected from untrusted JWT headers/URLs.
func NewOIDCVerifier(ctx context.Context, appleIDs, googleIDs string) *OIDCVerifier {
	client := &http.Client{Timeout: 3 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	ctx = oidc.ClientContext(ctx, client)
	result := &OIDCVerifier{Verifiers: map[string][]*oidc.IDTokenVerifier{}, ClientIDs: map[string][]string{}}
	for _, p := range []struct{ name, issuer, jwks, ids string }{
		{"apple", "https://appleid.apple.com", "https://appleid.apple.com/auth/keys", appleIDs},
		{"google", "https://accounts.google.com", "https://www.googleapis.com/oauth2/v3/certs", googleIDs},
	} {
		keys := oidc.NewRemoteKeySet(ctx, p.jwks)
		for _, id := range strings.Split(p.ids, ",") {
			if id = strings.TrimSpace(id); id != "" {
				result.Verifiers[p.name] = append(result.Verifiers[p.name], oidc.NewVerifier(p.issuer, keys, &oidc.Config{ClientID: id, SupportedSigningAlgs: []string{"RS256"}}))
				result.ClientIDs[p.name] = append(result.ClientIDs[p.name], id)
			}
		}
	}
	return result
}

func (v *OIDCVerifier) Enabled(provider string) bool { return len(v.Verifiers[provider]) > 0 }

func (v *OIDCVerifier) Verify(ctx context.Context, provider, raw string, nonceHash []byte) (ExternalIdentity, error) {
	verifiers := v.Verifiers[provider]
	if len(verifiers) == 0 {
		return ExternalIdentity{}, ErrProviderDisabled
	}
	if len(raw) > 8000 {
		return ExternalIdentity{}, ErrOIDC
	}
	for i, verifier := range verifiers {
		token, err := verifier.Verify(ctx, raw)
		if err != nil {
			continue
		}
		if token.Subject == "" || len(token.Subject) > 1024 || token.Nonce == "" || subtle.ConstantTimeCompare(tokenHash(token.Nonce), nonceHash) != 1 {
			return ExternalIdentity{}, ErrOIDC
		}
		now := time.Now()
		if token.IssuedAt.IsZero() || token.IssuedAt.After(now.Add(30*time.Second)) || !token.Expiry.After(token.IssuedAt) {
			return ExternalIdentity{}, ErrOIDC
		}
		var claims struct {
			Email    string          `json:"email"`
			Verified json.RawMessage `json:"email_verified"`
			AZP      string          `json:"azp"`
			NBF      *int64          `json:"nbf"`
		}
		if err = token.Claims(&claims); err != nil {
			return ExternalIdentity{}, ErrOIDC
		}
		if claims.NBF != nil && now.Unix() < *claims.NBF {
			return ExternalIdentity{}, ErrOIDC
		}
		// Multiple audiences require an authorized-party claim naming an audience.
		if len(token.Audience) > 1 && claims.AZP == "" {
			return ExternalIdentity{}, ErrOIDC
		}
		if claims.AZP != "" {
			ids := v.ClientIDs[provider]
			if i >= len(ids) || claims.AZP != ids[i] {
				return ExternalIdentity{}, ErrOIDC
			}
		}
		email := ""
		if string(claims.Verified) == "true" || string(claims.Verified) == `"true"` {
			email, _ = NormalizeEmail(claims.Email)
		}
		return ExternalIdentity{provider, token.Subject, email}, nil
	}
	return ExternalIdentity{}, ErrOIDC
}
