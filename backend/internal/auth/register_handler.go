package auth

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"mime"
	"net/http"
	"strconv"
	"time"
	"unicode/utf8"

	"github.com/labstack/echo/v5"
	"tramas/backend/internal/users"
)

type APIError struct {
	Code    string            `json:"code"`
	Message string            `json:"message"`
	Fields  map[string]string `json:"fields,omitempty"`
}
type Response struct {
	User  *users.User `json:"user,omitempty"`
	Error *APIError   `json:"error,omitempty"`
}

func failure(c *echo.Context, status int, code, message string, fields map[string]string) error {
	return c.JSON(status, Response{Error: &APIError{Code: code, Message: message, Fields: fields}})
}

type Handler struct {
	Service    RegistrationService
	Limiter    *Limiter
	RetryAfter int
	Logger     *slog.Logger
}

func Register(e *echo.Echo, repository users.Repository, c Config) error {
	if err := c.Validate(); err != nil {
		return err
	}
	policy, err := NewPolicy(c)
	if err != nil {
		return err
	}
	hasher, err := NewArgonHasher(c)
	if err != nil {
		return err
	}
	h := Handler{Service: RegistrationService{Repository: repository, Hasher: hasher, Policy: policy}, Limiter: NewLimiter(c),
		RetryAfter: (60 + c.RatePerMinute - 1) / c.RatePerMinute, Logger: slog.Default()}
	previous := e.HTTPErrorHandler
	e.HTTPErrorHandler = func(ctx *echo.Context, err error) {
		if ctx.Request().URL.Path != "/api/v1/auth/register" {
			previous(ctx, err)
			return
		}
		if response, _ := echo.UnwrapResponse(ctx.Response()); response != nil && response.Committed {
			return
		}
		status := http.StatusInternalServerError
		var coded echo.HTTPStatusCoder
		if errors.As(err, &coded) && coded.StatusCode() >= 400 && coded.StatusCode() <= 599 {
			status = coded.StatusCode()
		}
		code := "request_error"
		if status >= 500 {
			code = "internal_error"
		}
		_ = failure(ctx, status, code, http.StatusText(status), nil)
	}
	e.POST("/api/v1/auth/register", h.Handle)
	return nil
}

func (h Handler) Handle(c *echo.Context) error {
	c.Response().Header().Set("Cache-Control", "no-store")
	if !h.Limiter.Allow(c.RealIP(), time.Now()) {
		c.Response().Header().Set("Retry-After", strconv.Itoa(h.RetryAfter))
		return failure(c, 429, "rate_limited", "Too many registration attempts.", nil)
	}
	mediaType, _, err := mime.ParseMediaType(c.Request().Header.Get("Content-Type"))
	if err != nil || mediaType != "application/json" {
		return failure(c, 415, "unsupported_media_type", "Content-Type must be application/json.", nil)
	}
	const maxBody = 8 << 10
	if c.Request().ContentLength > maxBody {
		return failure(c, 413, "request_too_large", "Request body is too large.", nil)
	}
	body := http.MaxBytesReader(c.Response(), c.Request().Body, maxBody)
	defer body.Close()
	data, err := io.ReadAll(body)
	if err != nil {
		return malformed(c, err)
	}
	if !utf8.Valid(data) {
		return malformed(c, nil)
	}
	// Reject duplicate keys as well as unknown fields; decode only into a registration DTO.
	decoder := json.NewDecoder(bytes.NewReader(data))
	token, err := decoder.Token()
	if err != nil {
		return malformed(c, err)
	}
	if token != json.Delim('{') {
		return malformed(c, nil)
	}
	values := map[string]string{}
	for decoder.More() {
		token, err = decoder.Token()
		if err != nil {
			return malformed(c, err)
		}
		key, ok := token.(string)
		if !ok || key != "email" && key != "password" {
			return malformed(c, nil)
		}
		if _, duplicate := values[key]; duplicate {
			return malformed(c, nil)
		}
		var value *string
		if err = decoder.Decode(&value); err != nil {
			return malformed(c, err)
		}
		if value == nil {
			return malformed(c, nil)
		}
		values[key] = *value
	}
	if _, err = decoder.Token(); err != nil {
		return malformed(c, err)
	}
	if _, err = decoder.Token(); !errors.Is(err, io.EOF) {
		return malformed(c, err)
	}
	u, err := h.Service.Register(c.Request().Context(), values["email"], values["password"])
	if err == nil {
		return c.JSON(http.StatusCreated, Response{User: &u})
	}
	var validation *ValidationError
	switch {
	case errors.As(err, &validation):
		return failure(c, 422, "validation_error", "Invalid registration input.", validation.Fields)
	case errors.Is(err, users.ErrEmailExists):
		return failure(c, 409, "email_already_registered", "This email address is already registered.", nil)
	case errors.Is(err, ErrBusy):
		c.Response().Header().Set("Retry-After", "1")
		return failure(c, 503, "registration_busy", "Registration is temporarily unavailable.", nil)
	default:
		// Never log raw errors: pgx errors can include email, SQL values or credentials.
		h.Logger.Error("registration failed", "request_id", c.Response().Header().Get(echo.HeaderXRequestID), "category", "internal")
		return failure(c, 500, "internal_error", "Registration could not be completed.", nil)
	}
}

func malformed(c *echo.Context, err error) error {
	var tooLarge *http.MaxBytesError
	if errors.As(err, &tooLarge) {
		return failure(c, 413, "request_too_large", "Request body is too large.", nil)
	}
	return failure(c, 400, "invalid_json", "Invalid registration JSON.", nil)
}
