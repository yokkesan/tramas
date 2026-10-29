package integration_test

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/labstack/echo/v5"
	"golang.org/x/crypto/argon2"
	"tramas/backend/internal/auth"
	"tramas/backend/internal/users"
	"tramas/backend/migrations"
)

func TestRegistrationPostgres(t *testing.T) {
	if os.Getenv("TEST_DATABASE_URL") == "" {
		t.Skip("isolated test DB not configured; run compose.test.yaml")
	}
	ctx := context.Background()
	if os.Getenv("TEST_DB_NAME") != "tramas_test" {
		t.Fatal("refusing non-test database")
	}
	open := func(key, user string) *pgxpool.Pool {
		t.Helper()
		cfg, err := pgxpool.ParseConfig(os.Getenv(key))
		if err != nil {
			t.Fatal("invalid test configuration")
		}
		if cfg.ConnConfig.Database != "tramas_test" || cfg.ConnConfig.User != user || cfg.ConnConfig.Host != "postgres_test" {
			t.Fatal("unsafe test DB identity")
		}
		p, err := pgxpool.NewWithConfig(ctx, cfg)
		if err != nil {
			t.Fatal("test connection failed")
		}
		t.Cleanup(p.Close)
		var db, role string
		var super bool
		err = p.QueryRow(ctx, `SELECT current_database(),current_user,(SELECT rolsuper FROM pg_roles WHERE rolname=current_user)`).Scan(&db, &role, &super)
		if err != nil || db != "tramas_test" || role != user || super {
			t.Fatal("test DB/role assertion failed")
		}
		return p
	}
	owner := open("TEST_MIGRATION_DATABASE_URL", "tramas_testmigrator")
	if err := migrations.Up(ctx, os.Getenv("TEST_MIGRATION_DATABASE_URL"), "tramas_test", "tramas_testapp"); err != nil {
		t.Fatal(err)
	}
	app := open("TEST_DATABASE_URL", "tramas_testapp")
	c := auth.DefaultConfig()
	c.RateBurst = 1000
	c.RatePerMinute = 1000
	e := echo.New()
	if err := auth.Register(e, users.PostgresRepository{Pool: app}, c); err != nil {
		t.Fatal(err)
	}
	suffixBytes := make([]byte, 8)
	if _, err := rand.Read(suffixBytes); err != nil {
		t.Fatal(err)
	}
	suffix := hex.EncodeToString(suffixBytes)
	email := "registration-" + suffix + "@example.com"
	password := "an integration test passphrase " + suffix
	post := func(address string) *httptest.ResponseRecorder {
		body, _ := json.Marshal(map[string]string{"email": address, "password": password})
		r := httptest.NewRequest("POST", "/api/v1/auth/register", strings.NewReader(string(body)))
		r.RemoteAddr = "192.0.2.1:1234"
		r.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		e.ServeHTTP(w, r)
		return w
	}
	w := post(" " + strings.ToUpper(email) + " ")
	if w.Code != 201 {
		t.Fatalf("registration status %d: %s", w.Code, w.Body.String())
	}
	var response auth.Response
	if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if response.User == nil || len(response.User.ID) != 36 || response.User.ID[14] != '4' || response.User.Email != email || w.Header().Get("Set-Cookie") != "" {
		t.Fatal("invalid user or unexpected authentication")
	}
	id := response.User.ID
	var hash string
	var unverified bool
	if err := owner.QueryRow(ctx, `SELECT password_hash,email_verified_at IS NULL FROM users WHERE id=$1::uuid`, id).Scan(&hash, &unverified); err != nil {
		t.Fatal(err)
	}
	if !unverified || strings.Contains(hash, password) || !strings.HasPrefix(hash, "$argon2id$v=19$") {
		t.Fatal("verification/hash invariant violated")
	}
	parts := strings.Split(hash, "$")
	if len(parts) != 6 {
		t.Fatal("bad PHC hash")
	}
	salt, _ := base64.RawStdEncoding.DecodeString(parts[4])
	key, _ := base64.RawStdEncoding.DecodeString(parts[5])
	computed := argon2.IDKey([]byte(password), salt, c.ArgonIterations, c.ArgonMemory, c.ArgonParallelism, 32)
	if len(salt) != 16 || subtle.ConstantTimeCompare(key, computed) != 1 {
		t.Fatal("password verification failed")
	}
	if w = post(email); w.Code != 409 {
		t.Fatalf("duplicate status %d", w.Code)
	}
	if w = post("o'hara-" + suffix + "@example.com"); w.Code != 201 {
		t.Fatal("parameterized email insert failed")
	}
	var count int
	if err := owner.QueryRow(ctx, `SELECT count(*) FROM information_schema.columns WHERE table_schema='public' AND table_name='users' AND column_name IN ('role','is_admin','admin')`).Scan(&count); err != nil || count != 0 {
		t.Fatal("admin privilege fields unexpectedly present")
	}
	for _, statement := range []string{
		`INSERT INTO users (id,email,password_hash) VALUES(gen_random_uuid(),'forbidden@example.com','$argon2id$test')`,
		`INSERT INTO users (email,password_hash,email_verified_at) VALUES('forbidden@example.com','$argon2id$test',now())`,
		`UPDATE users SET id=gen_random_uuid()`, `DELETE FROM users`,
		`SELECT password_hash FROM users`, `SELECT * FROM goose_db_version`,
		`CREATE TABLE forbidden(id int)`,
	} {
		_, err := app.Exec(ctx, statement)
		pgerr, ok := err.(*pgconn.PgError)
		if !ok || pgerr.Code != "42501" {
			t.Fatalf("expected privilege denial, got %v", err)
		}
	}
	// Exercise the DB uniqueness constraint directly under a larger concurrent burst.
	repository := users.PostgresRepository{Pool: app}
	concurrentEmail := "race-" + suffix + "@example.com"
	var wg sync.WaitGroup
	results := make(chan error, 12)
	start := make(chan struct{})
	for range 12 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			_, err := repository.Create(ctx, concurrentEmail, hash)
			results <- err
		}()
	}
	close(start)
	wg.Wait()
	close(results)
	success, duplicates := 0, 0
	for err := range results {
		if err == nil {
			success++
		} else if err == users.ErrEmailExists {
			duplicates++
		} else {
			t.Fatal(err)
		}
	}
	if success != 1 || duplicates != 11 {
		t.Fatalf("concurrent results %d/%d", success, duplicates)
	}
	// Also exercise two concurrent full HTTP registrations with real hashing.
	statuses := make(chan int, 2)
	for range 2 {
		wg.Add(1)
		go func() { defer wg.Done(); statuses <- post("http-race-" + suffix + "@example.com").Code }()
	}
	wg.Wait()
	close(statuses)
	got := map[int]int{}
	for status := range statuses {
		got[status]++
	}
	if got[201] != 1 || got[409] != 1 {
		t.Fatalf("HTTP concurrency statuses: %v", got)
	}
	// Migration re-execution must preserve both schema and existing data.
	if err := migrations.Up(ctx, os.Getenv("TEST_MIGRATION_DATABASE_URL"), "tramas_test", "tramas_testapp"); err != nil {
		t.Fatal(err)
	}
	var same string
	if err := owner.QueryRow(ctx, `SELECT id::text FROM users WHERE email=$1`, email).Scan(&same); err != nil || same != id {
		t.Fatal("migration rerun changed account identity")
	}
	// Closed connection pool simulates a real repository failure at the HTTP boundary.
	broken, _ := pgxpool.New(ctx, os.Getenv("TEST_DATABASE_URL"))
	broken.Close()
	fault := echo.New()
	if err := auth.Register(fault, users.PostgresRepository{Pool: broken}, c); err != nil {
		t.Fatal(err)
	}
	body := fmt.Sprintf(`{"email":"fault-%s@example.com","password":%q}`, suffix, password)
	r := httptest.NewRequest("POST", "/api/v1/auth/register", strings.NewReader(body))
	r.RemoteAddr = "192.0.2.1:1234"
	r.Header.Set("Content-Type", "application/json")
	fw := httptest.NewRecorder()
	fault.ServeHTTP(fw, r)
	if fw.Code != 500 || strings.Contains(fw.Body.String(), "pool") || strings.Contains(fw.Body.String(), password) || strings.Contains(fw.Body.String(), "postgres") {
		t.Fatal("DB failure response leaked internals")
	}
}
