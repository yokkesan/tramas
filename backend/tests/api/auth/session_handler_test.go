package auth_test

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/labstack/echo/v5"
	"tramas/backend/internal/auth"
)

type sessionRepository struct {
	auth.AuthRepository
	account auth.Account
	err     error
	calls   int
}

func (r *sessionRepository) Allow(context.Context, string, int, time.Duration) (bool, error) {
	return true, nil
}
func (r *sessionRepository) Authenticate(context.Context, string) (auth.Account, error) {
	r.calls++
	return r.account, r.err
}
func (r *sessionRepository) Profile(_ context.Context, _ string, n string) (auth.Account, error) {
	r.calls++
	r.account.Nickname = &n
	r.account.ProfileCompleted = true
	return r.account, r.err
}
func (r *sessionRepository) Resend(context.Context, string) error      { r.calls++; return r.err }
func (r *sessionRepository) VerifyEmail(context.Context, string) error { r.calls++; return r.err }
func (r *sessionRepository) Logout(context.Context, string) error      { r.calls++; return r.err }

func sessionHTTP(t *testing.T, r *sessionRepository) *echo.Echo {
	t.Helper()
	e := echo.New()
	h := auth.RegisterAuthentication(e, &auth.SessionService{Repository: r}, auth.DefaultConfig())
	e.GET("/ordinary", func(c *echo.Context) error { return c.NoContent(204) }, h.RequireAuth(), auth.RequireProfile)
	return e
}

func authRequest(e *echo.Echo, method, path, body, token string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	req.RemoteAddr = "192.0.2.10:4000"
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	w := httptest.NewRecorder()
	e.ServeHTTP(w, req)
	return w
}

func TestAuthHTTPStrictInput(t *testing.T) {
	for _, tc := range []struct {
		method, path, body string
		status             int
	}{
		{"POST", "/api/v1/auth/email/resend", `{"email":"u@example.com","email":"v@example.com"}`, 400},
		{"POST", "/api/v1/auth/email/resend", `{"email":null}`, 400},
		{"POST", "/api/v1/auth/email/resend", `{"email":1}`, 400},
		{"POST", "/api/v1/auth/email/resend", `{"email":"u@example.com","admin":"true"}`, 400},
		{"POST", "/api/v1/auth/email/resend", `{"email":"u@example.com"}{}`, 400},
		{"POST", "/api/v1/auth/email/resend", strings.Repeat(" ", 8193), 413},
		{"POST", "/api/v1/auth/refresh", `{"refresh_token":"bad","request_id":"bad"}`, 401},
		{"POST", "/api/v1/auth/email/verify", `{"token":"bad"}`, 401},
		{"GET", "/api/v1/auth/email/verify", "", 405},
	} {
		t.Run(tc.path+tc.body[:min(len(tc.body), 20)], func(t *testing.T) {
			r := &sessionRepository{}
			w := authRequest(sessionHTTP(t, r), tc.method, tc.path, tc.body, "")
			if w.Code != tc.status {
				t.Fatalf("got %d %s", w.Code, w.Body.String())
			}
			if r.calls != 0 {
				t.Fatal("invalid request reached repository")
			}
			var v auth.Response
			if err := json.Unmarshal(w.Body.Bytes(), &v); err != nil || v.Error == nil {
				t.Fatal("missing error envelope")
			}
			if w.Header().Get("Cache-Control") != "no-store" {
				t.Fatal("missing no-store")
			}
		})
	}
}

func TestAuthenticationAndProfileAuthorization(t *testing.T) {
	token := base64.RawURLEncoding.EncodeToString(make([]byte, 32))
	r := &sessionRepository{account: auth.Account{ID: "fixed-id", EmailVerified: true}}
	e := sessionHTTP(t, r)
	if w := authRequest(e, "GET", "/api/v1/auth/me", "", ""); w.Code != 401 {
		t.Fatal(w.Code)
	}
	if w := authRequest(e, "GET", "/api/v1/auth/me", "", token); w.Code != 200 || !strings.Contains(w.Body.String(), `"profile_completed":false`) {
		t.Fatal(w.Code, w.Body.String())
	}
	if w := authRequest(e, "GET", "/ordinary", "", token); w.Code != 403 || !strings.Contains(w.Body.String(), "profile_required") {
		t.Fatal(w.Code)
	}
	if w := authRequest(e, "PATCH", "/api/v1/users/me/profile", `{"nickname":"虎ファン"}`, token); w.Code != 200 {
		t.Fatal(w.Code, w.Body.String())
	}
	if w := authRequest(e, "GET", "/ordinary", "", token); w.Code != 204 {
		t.Fatal(w.Code)
	}
	if w := authRequest(e, "POST", "/api/v1/auth/logout", `{"refresh_token":"`+token+`"}`, ""); w.Code != 204 {
		t.Fatal(w.Code)
	}
	r.err = errors.New("postgres secret-token internal-email@example.com")
	if w := authRequest(e, "GET", "/api/v1/auth/me", "", token); w.Code != 500 || strings.Contains(w.Body.String(), "secret-token") || strings.Contains(w.Body.String(), "postgres") {
		t.Fatal("internal error disclosure")
	}
}

func TestLocalMailboxKeyAndCache(t *testing.T) {
	m := &auth.MemoryMailbox{}
	if err := m.SendVerification(context.Background(), auth.VerificationMail{Email: "test@example.com", Token: "secret", ExpiresAt: time.Now().Add(time.Hour)}); err != nil {
		t.Fatal(err)
	}
	e := echo.New()
	key := strings.Repeat("k", 32)
	auth.RegisterLocalMailbox(e, m, key)
	for _, provided := range []string{"", "wrong", key} {
		req := httptest.NewRequest("GET", "/dev/auth/mailbox", nil)
		req.Header.Set("X-Dev-Mailbox-Key", provided)
		w := httptest.NewRecorder()
		e.ServeHTTP(w, req)
		want := 404
		if provided == key {
			want = 200
		}
		if w.Code != want || w.Header().Get("Cache-Control") != "no-store" {
			t.Fatal("mailbox protection failed")
		}
		if provided != key && strings.Contains(w.Body.String(), "secret") {
			t.Fatal("mail token leaked")
		}
	}
}
