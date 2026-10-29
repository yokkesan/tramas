package migrations

import (
	"context"
	"embed"
	"fmt"
	"io"
	"log/slog"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"
	"github.com/pressly/goose/v3/lock"
)

//go:embed *.sql
var files embed.FS

// Up deliberately exposes no rollback, reset, or down operation.
func Up(ctx context.Context, url, expectedDB, appRole string) error {
	if expectedDB == "" || appRole == "" {
		return fmt.Errorf("expected database and app role are required")
	}
	cfg, err := pgx.ParseConfig(url)
	if err != nil {
		return fmt.Errorf("invalid migration database configuration")
	}
	if cfg.Database != expectedDB {
		return fmt.Errorf("migration database does not match expected database")
	}
	cfg.RuntimeParams["tramas.app_role"] = appRole
	cfg.RuntimeParams["statement_timeout"] = "30000"
	cfg.RuntimeParams["lock_timeout"] = "10000"
	db := stdlib.OpenDB(*cfg)
	defer db.Close()
	db.SetMaxOpenConns(2)
	var actual string
	if err = db.QueryRowContext(ctx, "SELECT current_database()").Scan(&actual); err != nil {
		return err
	}
	if actual != expectedDB {
		return fmt.Errorf("unexpected migration database identity")
	}
	locker, err := lock.NewPostgresSessionLocker()
	if err != nil {
		return err
	}
	provider, err := goose.NewProvider(goose.DialectPostgres, db, files, goose.WithSessionLocker(locker),
		goose.WithSlog(slog.New(slog.NewTextHandler(io.Discard, nil))))
	if err != nil {
		return err
	}
	_, err = provider.Up(ctx)
	return err
}
