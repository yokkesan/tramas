package main

import (
	"context"
	"fmt"
	"os"
	"time"

	"tramas/backend/migrations"
)

func main() {
	if len(os.Args) != 2 || os.Args[1] != "up" {
		fmt.Fprintln(os.Stderr, "usage: migrate up (forward-only)")
		os.Exit(2)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	if err := migrations.Up(ctx, os.Getenv("MIGRATION_DATABASE_URL"), os.Getenv("MIGRATION_EXPECTED_DB"), os.Getenv("APP_DB_USER")); err != nil {
		fmt.Fprintln(os.Stderr, "migration failed; verify database identity, role permissions and migration state")
		os.Exit(1)
	}
	fmt.Println("migrations applied")
}
