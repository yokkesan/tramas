package main

import (
	"context"
	"errors"
	"io/fs"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/labstack/echo/v5"
	"github.com/labstack/echo/v5/middleware"
	"tramas/backend/internal/admin"
	"tramas/backend/internal/auth"
	"tramas/backend/internal/platform"
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
		Skipper: func(c *echo.Context) bool { return c.Request().URL.Path == "/api/v1/auth/register" },
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
	if err = auth.Register(e, users.PostgresRepository{Pool: pool}, registration); err != nil {
		return errors.New("registration configuration failed")
	}
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
