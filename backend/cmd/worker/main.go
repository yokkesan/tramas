package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"sync/atomic"
	"syscall"
	"time"

	"tramas/backend/internal/platform"
)

func main() {
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, nil)))
	if len(os.Args) == 2 && os.Args[1] == "healthcheck" {
		if platform.CheckHTTP("http://127.0.0.1:8081/health") != nil {
			os.Exit(1)
		}
		return
	}
	if err := run(); err != nil {
		slog.Error("worker stopped", "reason", err.Error())
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
	var heartbeat atomic.Int64
	heartbeat.Store(time.Now().Unix())
	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", func(w http.ResponseWriter, r *http.Request) {
		if time.Since(time.Unix(heartbeat.Load(), 0)) > time.Minute || platform.Ping(r.Context(), pool) != nil {
			http.Error(w, "unavailable", http.StatusServiceUnavailable)
			return
		}
		w.WriteHeader(http.StatusOK)
	})
	server := platform.NewHTTPServer("127.0.0.1:8081", mux)
	result := make(chan error, 1)
	go func() { result <- server.ListenAndServe() }()
	ticker := time.NewTicker(15 * time.Second)
	defer ticker.Stop()
	slog.Info("worker started", "mode", "idle; no jobs configured")
	for {
		select {
		case <-ctx.Done():
			shutdown, stop := context.WithTimeout(context.Background(), 10*time.Second)
			defer stop()
			return server.Shutdown(shutdown)
		case err := <-result:
			if !errors.Is(err, http.ErrServerClosed) {
				return err
			}
			return nil
		case <-ticker.C:
			if platform.Ping(ctx, pool) != nil {
				slog.Warn("worker database unavailable")
				continue
			}
			heartbeat.Store(time.Now().Unix())
		}
	}
}
