package auth

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"strings"
	"unicode"
	"unicode/utf8"
)

func newToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

func validToken(v string) bool {
	if len(v) != 43 {
		return false
	}
	b, err := base64.RawURLEncoding.Strict().DecodeString(v)
	return err == nil && len(b) == 32
}

func tokenHash(v string) []byte { h := sha256.Sum256([]byte(v)); return h[:] }

// Derivation allows a lost refresh response to be reproduced without persisting
// plaintext credentials. The caller must supply the SAME random request ID.
func rotatedTokens(old, requestID string) (string, string) {
	derive := func(label string) string {
		h := hmac.New(sha256.New, []byte(old))
		h.Write([]byte("tramas/refresh/v1/" + label + "/" + requestID))
		return base64.RawURLEncoding.EncodeToString(h.Sum(nil))
	}
	return derive("access"), derive("refresh")
}

// NormalizeNickname counts Unicode scalar values (Dart: nickname.runes.length).
// Trim only ASCII space for identical behavior on both platforms. No NFC folding.
func NormalizeNickname(v string) (string, bool) {
	if !utf8.ValidString(v) {
		return "", false
	}
	v = strings.Trim(v, " ")
	n := utf8.RuneCountInString(v)
	if n < 2 || n > 20 {
		return "", false
	}
	for _, r := range v {
		if unicode.IsControl(r) || r == utf8.RuneError {
			return "", false
		}
	}
	if strings.TrimSpace(v) == "" {
		return "", false
	}
	return v, true
}
