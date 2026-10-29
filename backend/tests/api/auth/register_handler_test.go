package auth_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http/httptest"
	"strings"
	"testing"
	"tramas/backend/internal/auth"

	"github.com/labstack/echo/v5"
	"tramas/backend/internal/users"
)

type fakeRepository struct {
	email, hash string
	err         error
	called      bool
}

func (r *fakeRepository) Create(_ context.Context, email, hash string) (users.User, error) {
	r.called = true
	r.email = email
	r.hash = hash
	return users.User{ID: "123e4567-e89b-42d3-a456-426614174000", Email: email}, r.err
}

type fakeHasher struct {
	err    error
	called bool
}

func (h *fakeHasher) Hash(_ context.Context, _ string) (string, error) {
	h.called = true
	return "$argon2id$test", h.err
}

func testHandler(repository *fakeRepository, hasher *fakeHasher, logs *bytes.Buffer) *echo.Echo {
	c := auth.DefaultConfig()
	c.RateBurst = 1000
	c.RatePerMinute = 1000
	p, _ := auth.NewPolicy(c)
	h := auth.Handler{Service: auth.RegistrationService{Repository: repository, Hasher: hasher, Policy: p}, Limiter: auth.NewLimiter(c), RetryAfter: 1, Logger: slog.New(slog.NewTextHandler(logs, nil))}
	e := echo.New()
	e.POST("/api/v1/auth/register", h.Handle)
	return e
}

func send(e *echo.Echo, body, contentType string, chunked bool) *httptest.ResponseRecorder {
	r := httptest.NewRequest("POST", "/api/v1/auth/register", strings.NewReader(body))
	r.RemoteAddr = "192.0.2.1:1234"
	r.Header.Set("Content-Type", contentType)
	if chunked {
		r.ContentLength = -1
	}
	w := httptest.NewRecorder()
	e.ServeHTTP(w, r)
	return w
}

func TestRegisterHTTPValidationAndNoPrivilegeInput(t *testing.T) {
	valid := `{"email":" User@Example.COM ","password":"a unique long passphrase"}`
	for _, tt := range []struct {
		name, body, ct string
		status         int
	}{
		{"success", valid, "application/json", 201},
		{"empty", "{}", "application/json", 422},
		{"missing email", `{"password":"a unique long passphrase"}`, "application/json", 422},
		{"invalid email", `{"email":"bad","password":"a unique long passphrase"}`, "application/json", 422},
		{"short password", `{"email":"user@example.com","password":"short"}`, "application/json", 422},
		{"blocked password", `{"email":"user@example.com","password":"passwordpassword"}`, "application/json", 422},
		{"malformed", "{", "application/json", 400},
		{"empty body", "", "application/json", 400},
		{"null", "null", "application/json", 400},
		{"null password", `{"email":"user@example.com","password":null}`, "application/json", 400},
		{"numeric password", `{"email":"user@example.com","password":123}`, "application/json", 400},
		{"admin role", `{"email":"user@example.com","password":"a unique long passphrase","role":"admin"}`, "application/json", 400},
		{"admin flag", `{"is_admin":true}`, "application/json", 400},
		{"verified email", `{"email_verified_at":"2026-01-01"}`, "application/json", 400},
		{"provided id", `{"id":"chosen-id"}`, "application/json", 400},
		{"duplicate key", `{"email":"a@example.com","email":"b@example.com","password":"a unique long passphrase"}`, "application/json", 400},
		{"trailing document", valid + "{}", "application/json", 400},
		{"invalid UTF8", `{"email":"a@example.com","password":"long passphrase` + string([]byte{255}) + `"}`, "application/json", 400},
		{"content type", valid, "text/plain", 415},
		{"large", strings.Repeat(" ", 9000), "application/json", 413},
		{"very large", strings.Repeat(" ", 2<<20), "application/json", 413},
	} {
		t.Run(tt.name, func(t *testing.T) {
			for _, chunked := range []bool{false, true} {
				r := &fakeRepository{}
				h := &fakeHasher{}
				var logs bytes.Buffer
				w := send(testHandler(r, h, &logs), tt.body, tt.ct, chunked)
				if w.Code != tt.status {
					t.Fatalf("status %d, expected %d: %s", w.Code, tt.status, w.Body.String())
				}
				var response auth.Response
				if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
					t.Fatal(err)
				}
				if w.Header().Get("Cache-Control") != "no-store" || w.Header().Get("Set-Cookie") != "" {
					t.Fatal("cache/cookie invariant failed")
				}
				if tt.status == 201 {
					if r.email != "user@example.com" || r.hash != "$argon2id$test" || response.User == nil || response.Error != nil {
						t.Fatal("incorrect registration")
					}
				} else {
					if r.called || h.called || response.Error == nil || response.User != nil {
						t.Fatal("invalid request reached persistence/hash")
					}
				}
			}
		})
	}
}

func TestRegisterErrorsNeverExposeDetails(t *testing.T) {
	secret := "driver-secret password SQLSTATE connection-string"
	for _, tt := range []struct {
		name       string
		repo, hash error
		status     int
		code       string
	}{
		{"duplicate", users.ErrEmailExists, nil, 409, "email_already_registered"},
		{"database", errors.New(secret), nil, 500, "internal_error"},
		{"hash", nil, errors.New(secret), 500, "internal_error"},
		{"busy", nil, auth.ErrBusy, 503, "registration_busy"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			var logs bytes.Buffer
			r := &fakeRepository{err: tt.repo}
			h := &fakeHasher{err: tt.hash}
			w := send(testHandler(r, h, &logs), `{"email":"user@example.com","password":"a unique long passphrase"}`, "application/json", false)
			if w.Code != tt.status || !strings.Contains(w.Body.String(), tt.code) {
				t.Fatal("incorrect error mapping")
			}
			for _, s := range []string{secret, "a unique long passphrase", "user@example.com", "$argon2id$test"} {
				if strings.Contains(w.Body.String(), s) || strings.Contains(logs.String(), s) {
					t.Fatal("sensitive value leaked")
				}
			}
		})
	}
}

func TestRegisterRateResponse(t *testing.T) {
	c := auth.DefaultConfig()
	p, _ := auth.NewPolicy(c)
	var logs bytes.Buffer
	h := auth.Handler{Service: auth.RegistrationService{Repository: &fakeRepository{}, Hasher: &fakeHasher{}, Policy: p}, Limiter: auth.NewLimiter(c), RetryAfter: 12, Logger: slog.New(slog.NewTextHandler(&logs, nil))}
	e := echo.New()
	e.POST("/api/v1/auth/register", h.Handle)
	for i := range 3 {
		w := send(e, "{}", "application/json", false)
		if i == 2 && (w.Code != 429 || w.Header().Get("Retry-After") != "12") {
			t.Fatal("rate limit response invalid")
		}
	}
}

func TestRegisterRouteErrorEnvelope(t *testing.T) {
	e := echo.New()
	if err := auth.Register(e, &fakeRepository{}, auth.DefaultConfig()); err != nil {
		t.Fatal(err)
	}
	w := httptest.NewRecorder()
	e.ServeHTTP(w, httptest.NewRequest("GET", "/api/v1/auth/register", nil))
	var response auth.Response
	if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if w.Code != 405 || response.Error == nil || response.Error.Code != "request_error" {
		t.Fatal("non-uniform route error")
	}
}
