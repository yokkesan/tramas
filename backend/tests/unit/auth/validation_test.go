package auth_test

import (
	"strings"
	"testing"
	"tramas/backend/internal/auth"
)

func TestEmailPolicy(t *testing.T) {
	for _, tt := range []struct {
		input, want string
		valid       bool
	}{
		{" User@Example.COM ", "user@example.com", true}, {"o'hara@example.com", "o'hara@example.com", true},
		{"", "", false}, {"bad", "", false}, {"user@localhost", "", false},
		{"Name <user@example.com>", "", false}, {"user@example.com\r\nBcc:other@example.com", "", false},
		{"a..b@example.com", "", false}, {"a@-example.com", "", false}, {"a@example..com", "", false},
		{"ユーザー@example.com", "", false}, {"a@例.jp", "", false},
		{strings.Repeat("a", 65) + "@example.com", "", false},
	} {
		got, ok := auth.NormalizeEmail(tt.input)
		if got != tt.want || ok != tt.valid {
			t.Errorf("email case mismatch: %q", tt.input)
		}
	}
}

func TestPasswordPolicy(t *testing.T) {
	p, err := auth.NewPolicy(auth.DefaultConfig())
	if err != nil {
		t.Fatal(err)
	}
	for _, tt := range []struct {
		input string
		valid bool
	}{
		{"", false}, {"short", false}, {strings.Repeat(" ", 16), false},
		{"passwordpassword", false}, {"PASSWORDPASSWORD", false},
		{"a good long passphrase", true}, {"a good long passphrase ", true},
		{strings.Repeat("虎", 15), true}, {strings.Repeat("虎", 14), false},
		{strings.Repeat("虎", 128), true}, {strings.Repeat("虎", 129), false},
		{"long passphrase\x00secret", false}, {"long passphrase\xffsecret", false},
	} {
		if p.ValidatePassword(tt.input) != tt.valid {
			t.Errorf("password policy mismatch for case of length %d", len(tt.input))
		}
	}
}

func TestConfigAndCustomBlocklist(t *testing.T) {
	t.Setenv("REGISTER_PASSWORD_MIN", "16")
	t.Setenv("REGISTER_RATE_PER_MINUTE", "9")
	t.Setenv("TRUSTED_PROXY_CIDRS", "192.0.2.0/24")
	c, err := auth.LoadConfig()
	if err != nil || c.PasswordMin != 16 || c.RatePerMinute != 9 || len(c.TrustedProxies) != 1 {
		t.Fatal("config overrides failed")
	}
	t.Setenv("REGISTER_ARGON_MEMORY_KIB", "1")
	if _, err = auth.LoadConfig(); err == nil {
		t.Fatal("unsafe memory accepted")
	}
	c = auth.DefaultConfig()
	c.BlocklistPath = t.TempDir() + "/missing"
	if _, err = auth.NewPolicy(c); err == nil {
		t.Fatal("missing blocklist silently accepted")
	}
}
