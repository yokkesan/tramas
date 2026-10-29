package migrations_test

import (
	"context"
	"testing"
	"tramas/backend/migrations"
)

func TestDatabaseIdentityGuard(t *testing.T) {
	if err := migrations.Up(context.Background(), "postgres://unused:unused@invalid/tramas_dev", "tramas_test", "tramas_testapp"); err == nil {
		t.Fatal("mismatched database accepted")
	}
	if err := migrations.Up(context.Background(), "invalid", "", "tramas_testapp"); err == nil {
		t.Fatal("empty expected database accepted")
	}
}
