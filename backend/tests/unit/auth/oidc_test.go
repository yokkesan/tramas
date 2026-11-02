package auth_test

import (
	"context"
	"strings"
	"testing"
	"time"
	"tramas/backend/internal/auth"
	"tramas/backend/tests/testutil"
)

func TestOIDCVerificationBoundaries(t *testing.T) {
	fixture := testutil.NewOIDC(t)
	other := testutil.NewOIDC(t)
	for _, provider := range []string{"apple", "google"} {
		t.Run(provider, func(t *testing.T) {
			for _, tc := range []struct {
				name  string
				patch map[string]any
				valid bool
			}{
				{"valid", nil, true}, {"apple string boolean", map[string]any{"email_verified": "true"}, true},
				{"wrong issuer", map[string]any{"iss": "https://attacker.example"}, false},
				{"wrong audience", map[string]any{"aud": "attacker"}, false},
				{"expired", map[string]any{"exp": time.Now().Add(-time.Minute).Unix()}, false},
				{"missing expiry", map[string]any{"exp": nil}, false},
				{"wrong nonce", map[string]any{"nonce": "wrong"}, false},
				{"missing nonce", map[string]any{"nonce": nil}, false},
				{"missing subject", map[string]any{"sub": nil}, false},
				{"missing issued at", map[string]any{"iat": nil}, false},
				{"future issued at", map[string]any{"iat": time.Now().Add(time.Minute).Unix()}, false},
				{"future not before", map[string]any{"nbf": time.Now().Add(time.Minute).Unix()}, false},
				{"audience confusion", map[string]any{"aud": []string{"test-client", "attacker"}, "azp": "attacker"}, false},
				{"multiple audience missing azp", map[string]any{"aud": []string{"test-client", "other"}}, false},
				{"multiple audience valid", map[string]any{"aud": []string{"test-client", "other"}, "azp": "test-client"}, true},
			} {
				t.Run(tc.name, func(t *testing.T) {
					raw := fixture.Token(t, provider, "stable-subject", "nonce-value", "user@example.com", tc.patch)
					identity, err := fixture.Verifier.Verify(context.Background(), provider, raw, testutil.NonceHash("nonce-value"))
					if (err == nil) != tc.valid {
						t.Fatalf("verification valid=%v expected=%v", err == nil, tc.valid)
					}
					if err == nil && (identity.Subject != "stable-subject" || identity.Provider != provider) {
						t.Fatal("identity mismatch")
					}
				})
			}
			raw := other.Token(t, provider, "stable-subject", "nonce-value", "user@example.com", nil)
			if _, err := fixture.Verifier.Verify(context.Background(), provider, raw, testutil.NonceHash("nonce-value")); err == nil {
				t.Fatal("wrong signature accepted")
			}
			raw = fixture.Token(t, provider, "stable-subject", "nonce-value", "user@example.com", map[string]any{"email_verified": false})
			identity, err := fixture.Verifier.Verify(context.Background(), provider, raw, testutil.NonceHash("nonce-value"))
			if err != nil || identity.Email != "" {
				t.Fatal("unverified provider email trusted")
			}
		})
	}
	for _, alg := range []string{"none", "HS256"} {
		claims := map[string]any{"iss": "https://accounts.google.com", "aud": "test-client", "sub": "s", "nonce": "n", "iat": time.Now().Unix(), "exp": time.Now().Add(time.Hour).Unix()}
		raw := testutil.Sign(t, fixture.Key, alg, claims)
		if _, err := fixture.Verifier.Verify(context.Background(), "google", raw, testutil.NonceHash("n")); err == nil {
			t.Fatal("algorithm confusion accepted")
		}
	}
	if _, err := auth.NewOIDCVerifier(context.Background(), "", "").Verify(context.Background(), "google", strings.Repeat("x", 100), nil); err != auth.ErrProviderDisabled {
		t.Fatal("unconfigured provider enabled")
	}
}
