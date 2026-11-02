package auth

import (
	"bytes"
	"context"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"mime"
	"net/http"
	"net/netip"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/labstack/echo/v5"
)

type SessionHandler struct {
	Service *SessionService
	Limiter *Limiter
	Guard   echo.MiddlewareFunc
}
type requestError struct {
	status int
	code   string
}

func (e *requestError) Error() string { return e.code }

// readFields preserves the existing strict registration JSON contract.
func readFields(c *echo.Context, allowed ...string) (map[string]string, error) {
	return ReadStringFields(c, 8<<10, allowed...)
}

// ReadStringFields applies the same strict flat JSON contract to feature APIs.
// limit is a transport budget, not a business character-count policy.
func ReadStringFields(c *echo.Context, limit int64, allowed ...string) (map[string]string, error) {
	media, _, err := mime.ParseMediaType(c.Request().Header.Get("Content-Type"))
	if err != nil || media != "application/json" {
		return nil, &requestError{415, "unsupported_media_type"}
	}
	body := http.MaxBytesReader(c.Response(), c.Request().Body, limit)
	defer body.Close()
	data, err := io.ReadAll(body)
	if err != nil {
		var large *http.MaxBytesError
		if errors.As(err, &large) {
			return nil, &requestError{413, "request_too_large"}
		}
		return nil, &requestError{400, "invalid_json"}
	}
	bad := &requestError{400, "invalid_json"}
	if !utf8.Valid(data) {
		return nil, bad
	}
	d := json.NewDecoder(bytes.NewReader(data))
	t, err := d.Token()
	if err != nil || t != json.Delim('{') {
		return nil, bad
	}
	values := map[string]string{}
	for d.More() {
		t, err = d.Token()
		if err != nil {
			return nil, bad
		}
		key, ok := t.(string)
		if !ok {
			return nil, bad
		}
		known := false
		for _, a := range allowed {
			if key == a {
				known = true
			}
		}
		if _, duplicate := values[key]; duplicate || !known {
			return nil, bad
		}
		var value *string
		if err = d.Decode(&value); err != nil || value == nil {
			return nil, bad
		}
		values[key] = *value
	}
	if _, err = d.Token(); err != nil {
		return nil, bad
	}
	if _, err = d.Token(); !errors.Is(err, io.EOF) {
		return nil, bad
	}
	return values, nil
}

func WriteFailure(c *echo.Context, err error) error { return authFailure(c, err) }
func ErrorResponse(c *echo.Context, status int, code string) error {
	return failure(c, status, code, http.StatusText(status), nil)
}

func authFailure(c *echo.Context, err error) error {
	status, code := 500, "internal_error"
	var input *requestError
	switch {
	case errors.As(err, &input):
		status, code = input.status, input.code
	case errors.Is(err, ErrCredentials):
		status, code = 401, "invalid_credentials"
	case errors.Is(err, ErrToken):
		status, code = 401, "invalid_token"
	case errors.Is(err, ErrUnverified):
		status, code = 403, "email_verification_required"
	case errors.Is(err, ErrRefreshConflict):
		status, code = 409, "refresh_conflict"
	case errors.Is(err, ErrNickname):
		status, code = 422, "invalid_nickname"
	case errors.Is(err, ErrOIDC):
		status, code = 401, "external_auth_failed"
	case errors.Is(err, ErrProviderDisabled):
		status, code = 503, "provider_unavailable"
	case errors.Is(err, ErrIdentityLinked):
		status, code = 409, "identity_already_linked"
	case errors.Is(err, ErrPasswordPolicy):
		status, code = 422, "invalid_password"
	case errors.Is(err, ErrRateLimit):
		status, code = 429, "rate_limited"
		c.Response().Header().Set("Retry-After", "900")
	case errors.Is(err, ErrBusy):
		status, code = 503, "authentication_busy"
		c.Response().Header().Set("Retry-After", "1")
	}
	if status == 500 {
		slog.Error("authentication failed", "category", "internal", "request_id", c.Response().Header().Get(echo.HeaderXRequestID))
	}
	if status == 401 {
		c.Response().Header().Set("WWW-Authenticate", "Bearer")
	}
	return failure(c, status, code, http.StatusText(status), nil)
}

func RegisterAuthentication(e *echo.Echo, s *SessionService, c Config) *SessionHandler {
	c.RatePerMinute = 30
	c.RateBurst = 10
	h := &SessionHandler{Service: s, Limiter: NewLimiter(c)}
	// Auth routes share an ingress bucket, plus shared DB limits per operation.
	guard := func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c *echo.Context) error {
			c.Response().Header().Set("Cache-Control", "no-store")
			ctx, cancel := context.WithTimeout(c.Request().Context(), 5*time.Second)
			defer cancel()
			c.SetRequest(c.Request().WithContext(ctx))
			if !h.Limiter.Allow(c.RealIP(), time.Now()) {
				c.Response().Header().Set("Retry-After", "2")
				return failure(c, 429, "rate_limited", "Too many attempts.", nil)
			}
			ip, err := netip.ParseAddr(c.RealIP())
			if err != nil {
				return authFailure(c, ErrRateLimit)
			}
			ip = ip.Unmap()
			key := ip.String()
			if ip.Is6() {
				key = netip.PrefixFrom(ip, 64).Masked().String()
			}
			limit, window := 30, time.Minute
			if c.Path() == "/api/v1/auth/email/resend" || c.Path() == "/api/v1/auth/password/reset/request" {
				limit, window = 10, time.Hour
			}
			ok, err := s.Repository.Allow(ctx, "ip/"+c.Path()+"/"+key, limit, window)
			if err != nil {
				return authFailure(c, err)
			}
			if !ok {
				c.Response().Header().Set("Retry-After", strconv.Itoa(int(window.Seconds())))
				return failure(c, 429, "rate_limited", "Too many attempts.", nil)
			}
			return next(c)
		}
	}
	h.Guard = guard
	previous := e.HTTPErrorHandler
	e.HTTPErrorHandler = func(c *echo.Context, err error) {
		path := c.Request().URL.Path
		if path == "/api/v1/auth/register" || !strings.HasPrefix(path, "/api/v1/") {
			previous(c, err)
			return
		}
		if response, _ := echo.UnwrapResponse(c.Response()); response != nil && response.Committed {
			return
		}
		c.Response().Header().Set("Cache-Control", "no-store")
		var coded echo.HTTPStatusCoder
		if errors.As(err, &coded) && coded.StatusCode() >= 400 && coded.StatusCode() < 500 {
			_ = failure(c, coded.StatusCode(), "request_error", http.StatusText(coded.StatusCode()), nil)
			return
		}
		_ = authFailure(c, err)
	}
	e.POST("/api/v1/auth/login", h.Login, guard)
	e.POST("/api/v1/auth/refresh", h.Refresh, guard)
	e.POST("/api/v1/auth/logout", h.Logout, guard)
	e.POST("/api/v1/auth/email/verify", h.Verify, guard)
	e.POST("/api/v1/auth/email/resend", h.Resend, guard)
	e.GET("/api/v1/auth/me", h.Me, guard, h.RequireAuth())
	e.PATCH("/api/v1/users/me/profile", h.Profile, guard, h.RequireAuth())
	return h
}

func (h *SessionHandler) Login(c *echo.Context) error {
	v, err := readFields(c, "email", "password")
	if err != nil {
		return authFailure(c, err)
	}
	a, p, err := h.Service.Login(c.Request().Context(), v["email"], v["password"])
	if err != nil {
		return authFailure(c, err)
	}
	return c.JSON(200, struct {
		User   Account   `json:"user"`
		Tokens TokenPair `json:"tokens"`
	}{a, p})
}
func (h *SessionHandler) Refresh(c *echo.Context) error {
	v, err := readFields(c, "refresh_token", "request_id")
	if err != nil {
		return authFailure(c, err)
	}
	p, err := h.Service.Refresh(c.Request().Context(), v["refresh_token"], v["request_id"])
	if err != nil {
		return authFailure(c, err)
	}
	return c.JSON(200, struct {
		Tokens TokenPair `json:"tokens"`
	}{p})
}
func (h *SessionHandler) Logout(c *echo.Context) error {
	v, err := readFields(c, "refresh_token")
	if err != nil {
		return authFailure(c, err)
	}
	if err = h.Service.Logout(c.Request().Context(), v["refresh_token"]); err != nil {
		return authFailure(c, err)
	}
	return c.NoContent(204)
}
func (h *SessionHandler) Verify(c *echo.Context) error {
	v, err := readFields(c, "token")
	if err != nil {
		return authFailure(c, err)
	}
	if err = h.Service.VerifyEmail(c.Request().Context(), v["token"]); err != nil {
		return authFailure(c, err)
	}
	return c.NoContent(204)
}
func (h *SessionHandler) Resend(c *echo.Context) error {
	v, err := readFields(c, "email")
	if err != nil {
		return authFailure(c, err)
	}
	if err = h.Service.Resend(c.Request().Context(), v["email"]); err != nil {
		return authFailure(c, err)
	}
	return c.NoContent(202)
}
func (h *SessionHandler) Me(c *echo.Context) error {
	return c.JSON(200, map[string]any{"user": c.Get(accountContextKey)})
}
func (h *SessionHandler) Profile(c *echo.Context) error {
	v, err := readFields(c, "nickname")
	if err != nil {
		return authFailure(c, err)
	}
	a, ok := c.Get(accountContextKey).(Account)
	if !ok {
		return authFailure(c, ErrToken)
	}
	a, err = h.Service.Profile(c.Request().Context(), a.ID, v["nickname"])
	if err != nil {
		return authFailure(c, err)
	}
	return c.JSON(200, map[string]any{"user": a})
}

const accountContextKey = "tramas.auth.account"

// CurrentAccount is set only by RequireAuth; request bodies never select an owner.
func CurrentAccount(c *echo.Context) (Account, bool) {
	a, ok := c.Get(accountContextKey).(Account)
	return a, ok
}

// RequireAuth identifies a verified general user. It does not require a profile.
func (h *SessionHandler) RequireAuth() echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c *echo.Context) error {
			c.Response().Header().Set("Cache-Control", "no-store")
			values := c.Request().Header.Values("Authorization")
			if len(values) != 1 {
				return authFailure(c, ErrToken)
			}
			parts := strings.Split(values[0], " ")
			if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") {
				return authFailure(c, ErrToken)
			}
			ctx, cancel := context.WithTimeout(c.Request().Context(), 5*time.Second)
			defer cancel()
			a, err := h.Service.Authenticate(ctx, parts[1])
			if err != nil {
				return authFailure(c, err)
			}
			c.Set(accountContextKey, a)
			return next(c)
		}
	}
}

// RequireProfile follows RequireAuth on ordinary application routes.
func RequireProfile(next echo.HandlerFunc) echo.HandlerFunc {
	return func(c *echo.Context) error {
		a, ok := c.Get(accountContextKey).(Account)
		if !ok {
			return authFailure(c, ErrToken)
		}
		if !a.ProfileCompleted {
			return failure(c, 403, "profile_required", "Profile setup is required.", nil)
		}
		return next(c)
	}
}

// RegisterLocalMailbox is explicitly opt-in, development-only, and key-protected.
func RegisterLocalMailbox(e *echo.Echo, m *MemoryMailbox, key string) {
	e.GET("/dev/auth/mailbox", func(c *echo.Context) error {
		c.Response().Header().Set("Cache-Control", "no-store")
		provided := c.Request().Header.Get("X-Dev-Mailbox-Key")
		if len(key) < 32 || subtle.ConstantTimeCompare(tokenHash(key), tokenHash(provided)) != 1 {
			return c.NoContent(404)
		}
		return c.JSON(200, map[string]any{"messages": m.Messages()})
	})
}
