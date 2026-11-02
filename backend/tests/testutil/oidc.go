package testutil

import (
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"github.com/coreos/go-oidc/v3/oidc"
	"testing"
	"time"
	"tramas/backend/internal/auth"
)

type OIDCFixture struct {
	Key      *rsa.PrivateKey
	Verifier *auth.OIDCVerifier
}

func NewOIDC(t *testing.T) *OIDCFixture {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	v := &auth.OIDCVerifier{Verifiers: map[string][]*oidc.IDTokenVerifier{}, ClientIDs: map[string][]string{}}
	for provider, issuer := range map[string]string{"apple": "https://appleid.apple.com", "google": "https://accounts.google.com"} {
		v.Verifiers[provider] = []*oidc.IDTokenVerifier{oidc.NewVerifier(issuer, &oidc.StaticKeySet{PublicKeys: []crypto.PublicKey{&key.PublicKey}}, &oidc.Config{ClientID: "test-client", SupportedSigningAlgs: []string{"RS256"}})}
		v.ClientIDs[provider] = []string{"test-client"}
	}
	return &OIDCFixture{key, v}
}
func (f *OIDCFixture) Token(t *testing.T, provider, subject, nonce, email string, patch map[string]any) string {
	t.Helper()
	issuer := "https://accounts.google.com"
	if provider == "apple" {
		issuer = "https://appleid.apple.com"
	}
	claims := map[string]any{"iss": issuer, "aud": "test-client", "sub": subject, "nonce": nonce, "email": email, "email_verified": true, "iat": time.Now().Unix(), "exp": time.Now().Add(time.Hour).Unix()}
	for k, v := range patch {
		if v == nil {
			delete(claims, k)
		} else {
			claims[k] = v
		}
	}
	return Sign(t, f.Key, "RS256", claims)
}
func Sign(t *testing.T, key *rsa.PrivateKey, alg string, claims map[string]any) string {
	t.Helper()
	header, _ := json.Marshal(map[string]string{"alg": alg, "typ": "JWT", "kid": "test-key"})
	payload, _ := json.Marshal(claims)
	part := base64.RawURLEncoding.EncodeToString(header) + "." + base64.RawURLEncoding.EncodeToString(payload)
	hash := sha256.Sum256([]byte(part))
	signature, err := rsa.SignPKCS1v15(rand.Reader, key, crypto.SHA256, hash[:])
	if err != nil {
		t.Fatal(err)
	}
	return part + "." + base64.RawURLEncoding.EncodeToString(signature)
}
func NonceHash(n string) []byte { v := sha256.Sum256([]byte(n)); return v[:] }
