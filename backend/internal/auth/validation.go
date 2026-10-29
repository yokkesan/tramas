package auth

import (
	_ "embed"
	"net/mail"
	"os"
	"strings"
	"unicode/utf8"
)

//go:embed common-passwords.txt
var defaultBlocklist string

type Policy struct {
	Min, Max int
	Blocked  map[string]struct{}
}

func NewPolicy(c Config) (Policy, error) {
	text := defaultBlocklist
	if c.BlocklistPath != "" {
		data, err := os.ReadFile(c.BlocklistPath)
		if err != nil {
			return Policy{}, err
		}
		text += "\n" + string(data)
	}
	p := Policy{Min: c.PasswordMin, Max: c.PasswordMax, Blocked: make(map[string]struct{})}
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(line)
		if line != "" && !strings.HasPrefix(line, "#") {
			p.Blocked[strings.ToLower(line)] = struct{}{}
		}
	}
	return p, nil
}

// Email policy is provisionally ASCII, case-insensitive, with surrounding spaces removed.
func NormalizeEmail(value string) (string, bool) {
	value = strings.ToLower(strings.TrimSpace(value))
	if len(value) < 3 || len(value) > 254 {
		return "", false
	}
	for _, ch := range value {
		if ch < 33 || ch > 126 {
			return "", false
		}
	}
	addr, err := mail.ParseAddress(value)
	if err != nil || addr.Address != value || addr.Name != "" {
		return "", false
	}
	local, domain, ok := strings.Cut(value, "@")
	if !ok || len(local) > 64 || strings.HasPrefix(local, "\"") || !strings.Contains(domain, ".") {
		return "", false
	}
	for _, label := range strings.Split(domain, ".") {
		if len(label) == 0 || len(label) > 63 || label[0] == '-' || label[len(label)-1] == '-' {
			return "", false
		}
		for _, ch := range label {
			if !(ch >= 'a' && ch <= 'z' || ch >= '0' && ch <= '9' || ch == '-') {
				return "", false
			}
		}
	}
	return value, true
}

func (p Policy) ValidatePassword(password string) bool {
	if !utf8.ValidString(password) {
		return false
	}
	n := utf8.RuneCountInString(password)
	if n < p.Min || n > p.Max {
		return false
	}
	if _, blocked := p.Blocked[strings.ToLower(password)]; blocked {
		return false
	}
	for _, ch := range password {
		if ch < 32 || ch == 127 || ch == utf8.RuneError {
			return false
		}
	}
	return strings.TrimSpace(password) != ""
}
