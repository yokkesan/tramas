package main

import (
	"context"
	"encoding/json"
	"errors"
	"io/fs"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/labstack/echo/v5"
	"github.com/labstack/echo/v5/middleware"
	"tramas/backend/internal/admin"
	"tramas/backend/internal/auth"
	"tramas/backend/internal/favorites"
	"tramas/backend/internal/platform"
	"tramas/backend/internal/profile"
	"tramas/backend/internal/users"
	"tramas/backend/web"
)

func main() {
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, nil)))
	if len(os.Args) == 2 && os.Args[1] == "healthcheck" {
		if platform.CheckHTTP("http://127.0.0.1:8080/health") != nil {
			os.Exit(1)
		}
		return
	}
	if err := run(); err != nil {
		// Driver errors can contain connection details; never log the DSN or raw error.
		slog.Error("API stopped", "reason", err.Error())
		os.Exit(1)
	}
}

func run() error {
	cfg, err := platform.LoadConfig()
	if err != nil {
		return err
	}
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	pool, err := platform.OpenDatabase(ctx, cfg.DatabaseURL)
	if err != nil {
		return errors.New("database connection failed")
	}
	defer pool.Close()
	registration, err := auth.LoadConfig()
	if err != nil {
		return err
	}
	e := echo.New()
	if len(registration.TrustedProxies) > 0 {
		options := []echo.TrustOption{echo.TrustLoopback(false), echo.TrustLinkLocal(false), echo.TrustPrivateNet(false)}
		for _, network := range registration.TrustedProxies {
			options = append(options, echo.TrustIPRange(network))
		}
		e.IPExtractor = echo.ExtractIPFromXFFHeader(options...)
	}
	e.Use(middleware.Recover(), middleware.RequestID(), middleware.RequestLoggerWithConfig(middleware.RequestLoggerConfig{
		LogStatus: true, LogMethod: true, LogRoutePath: true, LogLatency: true, HandleError: true,
		LogValuesFunc: func(c *echo.Context, v middleware.RequestLoggerValues) error {
			// Log a route template, never query strings, bodies, headers or raw errors.
			slog.Info("request", "method", v.Method, "route", v.RoutePath, "status", v.Status, "latency", v.Latency)
			return nil
		},
	}))
	e.Use(middleware.BodyLimitWithConfig(middleware.BodyLimitConfig{LimitBytes: 1 << 20,
		Skipper: func(c *echo.Context) bool {
			return c.Request().URL.Path == "/api/v1/auth/register" || c.Request().Method == "PUT" && strings.HasPrefix(c.Request().URL.Path, "/api/v1/users/me/profile/images/")
		},
	}))
	e.Use(func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c *echo.Context) error {
			h := c.Response().Header()
			h.Set("X-Content-Type-Options", "nosniff")
			h.Set("X-Frame-Options", "DENY")
			h.Set("Referrer-Policy", "no-referrer")
			h.Set("Cache-Control", "no-store")
			h.Set("Content-Security-Policy", "default-src 'self'; script-src 'self'; style-src 'self'; object-src 'none'; base-uri 'none'; frame-ancestors 'none'")
			return next(c)
		}
	})
	hasher, err := auth.NewArgonHasher(registration)
	if err != nil {
		return errors.New("authentication configuration failed")
	}
	if err = auth.RegisterWithHasher(e, users.PostgresRepository{Pool: pool}, registration, hasher); err != nil {
		return errors.New("registration configuration failed")
	}
	authRepository := auth.PostgresAuthRepository{Pool: pool}
	if raw := os.Getenv("PASSWORD_RESET_TOKEN_TTL"); raw != "" {
		ttl, err := time.ParseDuration(raw)
		if err != nil || ttl < time.Minute || ttl > 24*time.Hour {
			return errors.New("invalid PASSWORD_RESET_TOKEN_TTL")
		}
		authRepository.ResetTTL = ttl
	}
	sessions, err := auth.NewSessionService(authRepository, hasher)
	if err != nil {
		return errors.New("authentication initialization failed")
	}
	authHandler := auth.RegisterAuthentication(e, sessions, registration)
	policy, err := auth.NewPolicy(registration)
	if err != nil {
		return errors.New("password policy configuration failed")
	}
	auth.RegisterReset(e, authHandler, auth.ResetService{Repository: authRepository, Sessions: sessions, Policy: policy})
	auth.RegisterExternal(e, authHandler, auth.ExternalService{Repository: authRepository, Sessions: sessions, Verifier: auth.NewOIDCVerifier(ctx, os.Getenv("OIDC_APPLE_CLIENT_IDS"), os.Getenv("OIDC_GOOGLE_CLIENT_IDS"))})
	var imageStorage profile.Storage
	if root := os.Getenv("PROFILE_STORAGE_ROOT"); root != "" {
		if cfg.Environment != "development" {
			return errors.New("local profile storage is development only")
		}
		files, err := profile.NewFileStorage(root)
		if err != nil {
			return errors.New("local profile storage unavailable")
		}
		defer files.Close()
		imageStorage = files
	}
	standards := []profile.StandardImage{}
	if raw := os.Getenv("PROFILE_STANDARD_IMAGES"); raw != "" {
		if err = json.Unmarshal([]byte(raw), &standards); err != nil {
			return errors.New("invalid PROFILE_STANDARD_IMAGES")
		}
	}
	for _, v := range standards {
		if v.ID == "" || v.Kind != "avatar" && v.Kind != "header" || !strings.HasPrefix(v.URL, "https://") && !strings.HasPrefix(v.URL, "/") {
			return errors.New("invalid standard image configuration")
		}
	}
	decodeBudget := int64(64 << 20)
	if raw := os.Getenv("PROFILE_DECODE_BUDGET_BYTES"); raw != "" {
		value, err := strconv.ParseInt(raw, 10, 64)
		if err != nil || value < 1<<20 || value > 256<<20 {
			return errors.New("invalid PROFILE_DECODE_BUDGET_BYTES")
		}
		decodeBudget = value
	}
	profileService := profile.Service{Repository: profile.PostgresRepository{Pool: pool}, Storage: imageStorage, Validator: profile.NewImageValidator(profile.CommandHEICValidator{}, decodeBudget), Standards: standards}
	profile.Register(e, authHandler, profileService)
	favorites.Register(e, authHandler, favorites.Service{Repository: favorites.PostgresRepository{Pool: pool}})
	var sender auth.VerificationSender
	if cfg.Environment == "development" {
		mailbox := &auth.MemoryMailbox{Capacity: 100}
		sender = mailbox
		if key := os.Getenv("AUTH_DEV_MAILBOX_KEY"); key != "" {
			if len(key) < 32 {
				return errors.New("AUTH_DEV_MAILBOX_KEY must contain at least 32 bytes")
			}
			auth.RegisterLocalMailbox(e, mailbox, key)
		}
	} else {
		if os.Getenv("AUTH_DEV_MAILBOX_KEY") != "" {
			return errors.New("local mailbox is forbidden in production")
		}
		slog.Warn("verification mail delivery is not configured", "category", "configuration")
	}
	backgroundDone := make(chan struct{})
	go func() { defer close(backgroundDone); authRepository.RunBackground(ctx, sender) }()
	defer func() { cancel(); <-backgroundDone }()
	cleanupDone := make(chan struct{})
	go func() {
		defer close(cleanupDone)
		ticker := time.NewTicker(time.Minute)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				work, stop := context.WithTimeout(ctx, 5*time.Second)
				profileService.Cleanup(work, "")
				stop()
			}
		}
	}()
	defer func() { cancel(); <-cleanupDone }()
	e.GET("/health", func(c *echo.Context) error {
		if platform.Ping(c.Request().Context(), pool) != nil {
			return c.JSON(http.StatusServiceUnavailable, map[string]string{"status": "unavailable"})
		}
		return c.JSON(http.StatusOK, map[string]string{"status": "ok", "database": "ok"})
	})
	if cfg.Environment == "development" {
		admin.Register(e, pool)
		assets, err := fs.Sub(web.Assets, "static")
		if err != nil {
			return err
		}
		e.GET("/static/*", echo.WrapHandler(http.StripPrefix("/static/", http.FileServer(http.FS(assets)))))
	}
	server := platform.NewHTTPServer(":8080", e)
	result := make(chan error, 1)
	go func() { result <- server.ListenAndServe() }()
	slog.Info("API started", "address", ":8080", "environment", cfg.Environment)
	select {
	case err := <-result:
		if !errors.Is(err, http.ErrServerClosed) {
			return err
		}
	case <-ctx.Done():
		shutdown, stop := context.WithTimeout(context.Background(), 10*time.Second)
		defer stop()
		return server.Shutdown(shutdown)
	}
	return nil
}
