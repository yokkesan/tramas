package auth_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"tramas/backend/internal/auth"
)

func TestPasswordVerification(t *testing.T) {
	h, err := auth.NewArgonHasher(auth.DefaultConfig())
	if err != nil {
		t.Fatal(err)
	}
	hash, err := h.Hash(context.Background(), "a correct long password")
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		password, hash string
		want           bool
	}{
		{"a correct long password", hash, true}, {"wrong", hash, false}, {"a correct long password ", hash, false},
		{"x", "$argon2id$v=19$m=4294967295,t=2,p=1$AAAAAAAAAAAAAAAAAAAAAA$AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA", false},
		{"x", strings.Replace(hash, "v=19", "v=16", 1), false},
		{"x", strings.Replace(hash, "m=19456", "m=19456extra", 1), false},
		{"x", hash + "suffix", false}, {"x", "", false},
	} {
		got, err := h.Verify(context.Background(), tc.password, tc.hash)
		if err != nil || got != tc.want {
			t.Fatalf("verification got %v / %v", got, err)
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err = h.Verify(ctx, "x", hash); !errors.Is(err, context.Canceled) {
		t.Fatal("cancellation ignored")
	}
}

func TestNicknameUnicodeScalarPolicy(t *testing.T) {
	for _, tc := range []struct {
		input, want string
		valid       bool
	}{
		{"ab", "ab", true}, {" 日本語 ", "日本語", true}, {"😀😀", "😀😀", true},
		{"e\u0301", "e\u0301", true}, // two scalar values, one rendered grapheme
		{strings.Repeat("虎", 20), strings.Repeat("虎", 20), true},
		{"a", "", false}, {"😀", "", false}, {"é", "", false}, {"　　", "", false},
		{strings.Repeat("😀", 21), "", false}, {"a\nb", "", false}, {"ab\t", "", false},
		{"a\ufffdb", "", false}, {string([]byte{255, 255}), "", false},
	} {
		got, valid := auth.NormalizeNickname(tc.input)
		if valid != tc.valid || (valid && got != tc.want) {
			t.Errorf("nickname %q got %q/%v", tc.input, got, valid)
		}
	}
}

func TestInvalidTokensNeverReachRepository(t *testing.T) {
	s := &auth.SessionService{} // nil repository: invalid input must stop before I/O
	for _, token := range []string{"", strings.Repeat("a", 42), strings.Repeat("+", 43), strings.Repeat("_", 43)} {
		if _, err := s.Authenticate(context.Background(), token); !errors.Is(err, auth.ErrToken) {
			t.Fatal(err)
		}
		if _, err := s.Refresh(context.Background(), token, token); !errors.Is(err, auth.ErrToken) {
			t.Fatal(err)
		}
		if err := s.VerifyEmail(context.Background(), token); !errors.Is(err, auth.ErrToken) {
			t.Fatal(err)
		}
		if err := s.Logout(context.Background(), token); !errors.Is(err, auth.ErrToken) {
			t.Fatal(err)
		}
	}
}
