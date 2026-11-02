package integration_test

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/labstack/echo/v5"
	"tramas/backend/internal/auth"
	"tramas/backend/internal/users"
	"tramas/backend/migrations"
)

func authTestPools(t *testing.T) (*pgxpool.Pool, *pgxpool.Pool) {
	t.Helper()
	if os.Getenv("TEST_DATABASE_URL") == "" {
		t.Skip("isolated test DB not configured; run compose.test.yaml")
	}
	if os.Getenv("TEST_DB_NAME") != "tramas_test" {
		t.Fatal("refusing non-test database")
	}
	open := func(key, role string) *pgxpool.Pool {
		t.Helper()
		cfg, err := pgxpool.ParseConfig(os.Getenv(key))
		if err != nil {
			t.Fatal("invalid test configuration")
		}
		if cfg.ConnConfig.Database != "tramas_test" || cfg.ConnConfig.Host != "postgres_test" || cfg.ConnConfig.User != role {
			t.Fatal("unsafe database identity")
		}
		pool, err := pgxpool.NewWithConfig(context.Background(), cfg)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(pool.Close)
		var db, user string
		var super bool
		if err = pool.QueryRow(context.Background(), `SELECT current_database(),current_user,(SELECT rolsuper FROM pg_roles WHERE rolname=current_user)`).Scan(&db, &user, &super); err != nil || db != "tramas_test" || user != role || super {
			t.Fatal("database identity assertion failed")
		}
		return pool
	}
	owner := open("TEST_MIGRATION_DATABASE_URL", "tramas_testmigrator")
	if err := migrations.Up(context.Background(), os.Getenv("TEST_MIGRATION_DATABASE_URL"), "tramas_test", "tramas_testapp"); err != nil {
		t.Fatal(err)
	}
	return open("TEST_DATABASE_URL", "tramas_testapp"), owner
}

func randomAuthToken(t *testing.T) string {
	t.Helper()
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		t.Fatal(err)
	}
	return base64.RawURLEncoding.EncodeToString(b)
}

type failedSender struct{}

func (failedSender) SendVerification(context.Context, auth.VerificationMail) error {
	return errors.New("SMTP password secret should never escape")
}

func TestAuthenticationPostgres(t *testing.T) {
	app, owner := authTestPools(t)
	ctx := context.Background()
	repository := auth.PostgresAuthRepository{Pool: app}
	config := auth.DefaultConfig()
	config.RateBurst = 1000
	config.RatePerMinute = 1000
	hasher, err := auth.NewArgonHasher(config)
	if err != nil {
		t.Fatal(err)
	}
	service, err := auth.NewSessionService(repository, hasher)
	if err != nil {
		t.Fatal(err)
	}
	e := echo.New()
	if err = auth.RegisterWithHasher(e, users.PostgresRepository{Pool: app}, config, hasher); err != nil {
		t.Fatal(err)
	}
	handler := auth.RegisterAuthentication(e, service, config)
	handler.Limiter = auth.NewLimiter(config)
	e.GET("/ordinary", func(c *echo.Context) error { return c.NoContent(204) }, handler.RequireAuth(), auth.RequireProfile)
	request := func(method, path string, body any, token string) *httptest.ResponseRecorder {
		data, _ := json.Marshal(body)
		req := httptest.NewRequest(method, path, bytes.NewReader(data))
		req.RemoteAddr = "198.51.100.15:5432"
		req.Header.Set("Content-Type", "application/json")
		if token != "" {
			req.Header.Set("Authorization", "Bearer "+token)
		}
		w := httptest.NewRecorder()
		e.ServeHTTP(w, req)
		return w
	}
	expect := func(w *httptest.ResponseRecorder, status int) {
		t.Helper()
		if w.Code != status {
			t.Fatalf("status %d expected %d: %s", w.Code, status, w.Body.String())
		}
	}
	decodePair := func(w *httptest.ResponseRecorder) auth.TokenPair {
		t.Helper()
		expect(w, 200)
		var v struct {
			Tokens auth.TokenPair `json:"tokens"`
		}
		if err := json.Unmarshal(w.Body.Bytes(), &v); err != nil {
			t.Fatal(err)
		}
		if len(v.Tokens.AccessToken) != 43 || len(v.Tokens.RefreshToken) != 43 || v.Tokens.AccessToken == v.Tokens.RefreshToken || v.Tokens.TokenType != "Bearer" {
			t.Fatal("invalid token response")
		}
		return v.Tokens
	}
	suffix := strings.ToLower(randomAuthToken(t)[:12])
	email := "auth-" + suffix + "@example.com"
	password := "a unique integration password " + suffix
	registered := request("POST", "/api/v1/auth/register", map[string]string{"email": email, "password": password}, "")
	expect(registered, 201)
	var registration auth.Response
	if err = json.Unmarshal(registered.Body.Bytes(), &registration); err != nil || registration.User == nil {
		t.Fatal("missing user")
	}
	id := registration.User.ID
	if strings.Contains(registered.Body.String(), "nickname") || strings.Contains(registered.Body.String(), "token") {
		t.Fatal("registration contract changed")
	}
	login := func(address, pw string) *httptest.ResponseRecorder {
		return request("POST", "/api/v1/auth/login", map[string]string{"email": address, "password": pw}, "")
	}
	expect(login(email, password), 403)
	invalid := login(email, "incorrect password")
	unknown := login("unknown-"+email, "incorrect password")
	expect(invalid, 401)
	expect(unknown, 401)
	if invalid.Body.String() != unknown.Body.String() {
		t.Fatal("credential errors enumerate accounts")
	}

	expedite := func() {
		t.Helper()
		if _, err := owner.Exec(ctx, `UPDATE auth_email_verifications SET next_attempt_at=now()-interval '1 day' WHERE user_id=$1::uuid`, id); err != nil {
			t.Fatal(err)
		}
	}
	expedite()
	if done, err := repository.DeliverOne(ctx, failedSender{}); !done || err != nil {
		t.Fatal("failed mail not retained for retry", err)
	}
	var attempts int
	var pending bool
	if err = owner.QueryRow(ctx, `SELECT attempts,pending FROM auth_email_verifications WHERE user_id=$1::uuid`, id).Scan(&attempts, &pending); err != nil || attempts != 1 || !pending {
		t.Fatal("delivery failure state", err)
	}
	mailbox := &auth.MemoryMailbox{Capacity: 100}
	deliver := func() auth.VerificationMail {
		t.Helper()
		expedite()
		if done, err := repository.DeliverOne(ctx, mailbox); !done || err != nil {
			t.Fatal("delivery failed", err)
		}
		messages := mailbox.Messages()
		for i := len(messages) - 1; i >= 0; i-- {
			if messages[i].Email == email {
				return messages[i]
			}
		}
		t.Fatal("missing verification mail")
		return auth.VerificationMail{}
	}
	firstMail := deliver()
	expect(request("POST", "/api/v1/auth/email/resend", map[string]string{"email": email}, ""), 202)
	// An immediate second resend and an unknown address have identical responses.
	expect(request("POST", "/api/v1/auth/email/resend", map[string]string{"email": email}, ""), 202)
	expect(request("POST", "/api/v1/auth/email/resend", map[string]string{"email": "unknown-" + email}, ""), 202)
	expedite()
	if _, err = repository.DeliverOne(ctx, failedSender{}); err != nil {
		t.Fatal(err)
	}
	var stored []byte
	if err = owner.QueryRow(ctx, `SELECT token_hash FROM auth_email_verifications WHERE user_id=$1::uuid`, id).Scan(&stored); err != nil {
		t.Fatal(err)
	}
	firstHash := sha256.Sum256([]byte(firstMail.Token))
	if !bytes.Equal(stored, firstHash[:]) {
		t.Fatal("send failure invalidated old token")
	}
	finalMail := deliver()
	if firstMail.Token == finalMail.Token {
		t.Fatal("resend reused token")
	}
	expect(request("POST", "/api/v1/auth/email/verify", map[string]string{"token": firstMail.Token}, ""), 401)
	// Expiry is enforced in PostgreSQL, not trusted from a client timestamp.
	if _, err = owner.Exec(ctx, `UPDATE auth_email_verifications SET expires_at=now()-interval '1 second' WHERE user_id=$1::uuid`, id); err != nil {
		t.Fatal(err)
	}
	expect(request("POST", "/api/v1/auth/email/verify", map[string]string{"token": finalMail.Token}, ""), 401)
	if _, err = owner.Exec(ctx, `UPDATE auth_email_verifications SET expires_at=now()+interval '1 hour' WHERE user_id=$1::uuid`, id); err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	statuses := make(chan int, 2)
	for range 2 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			statuses <- request("POST", "/api/v1/auth/email/verify", map[string]string{"token": finalMail.Token}, "").Code
		}()
	}
	wg.Wait()
	close(statuses)
	counts := map[int]int{}
	for status := range statuses {
		counts[status]++
	}
	if counts[204] != 1 || counts[401] != 1 {
		t.Fatalf("verification not single-use: %v", counts)
	}

	pair := decodePair(login(email, password))
	device2 := decodePair(login(email, password))
	expect(request("GET", "/api/v1/auth/me", nil, pair.RefreshToken), 401)
	expect(request("POST", "/api/v1/auth/refresh", map[string]string{"refresh_token": pair.AccessToken, "request_id": randomAuthToken(t)}, ""), 401)
	expect(request("GET", "/api/v1/auth/me", nil, pair.AccessToken), 200)
	expect(request("GET", "/ordinary", nil, pair.AccessToken), 403)
	expect(request("PATCH", "/api/v1/users/me/profile", map[string]string{"nickname": "a"}, pair.AccessToken), 422)
	profile := request("PATCH", "/api/v1/users/me/profile", map[string]string{"nickname": " 虎ファン😀 "}, pair.AccessToken)
	expect(profile, 200)
	if !strings.Contains(profile.Body.String(), `"profile_completed":true`) {
		t.Fatal("profile state missing")
	}
	expect(request("GET", "/ordinary", nil, pair.AccessToken), 204)
	expect(request("PATCH", "/api/v1/users/me/profile", map[string]string{"nickname": "変更可能"}, pair.AccessToken), 200)
	// Duplicate nicknames are intentionally allowed.
	other, err := (users.PostgresRepository{Pool: app}).Create(ctx, "nickname-"+email, "$argon2id$test")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = owner.Exec(ctx, `UPDATE users SET email_verified_at=now() WHERE id=$1::uuid`, other.ID); err != nil {
		t.Fatal(err)
	}
	if _, err = service.Profile(ctx, other.ID, "変更可能"); err != nil {
		t.Fatal("duplicate nickname rejected", err)
	}

	requestID := randomAuthToken(t)
	responses := make(chan *httptest.ResponseRecorder, 6)
	for range 6 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			responses <- request("POST", "/api/v1/auth/refresh", map[string]string{"refresh_token": pair.RefreshToken, "request_id": requestID}, "")
		}()
	}
	wg.Wait()
	close(responses)
	var rotated auth.TokenPair
	for w := range responses {
		got := decodePair(w)
		if rotated.AccessToken != "" && got != rotated {
			t.Fatal("concurrent refresh returned different results")
		}
		rotated = got
	}
	expect(request("GET", "/api/v1/auth/me", nil, pair.AccessToken), 401)
	expect(request("GET", "/api/v1/auth/me", nil, rotated.AccessToken), 200)
	expect(request("POST", "/api/v1/auth/refresh", map[string]string{"refresh_token": pair.RefreshToken, "request_id": randomAuthToken(t)}, ""), 409)
	expect(request("GET", "/api/v1/auth/me", nil, rotated.AccessToken), 200)
	retry := decodePair(request("POST", "/api/v1/auth/refresh", map[string]string{"refresh_token": pair.RefreshToken, "request_id": requestID}, ""))
	if retry != rotated {
		t.Fatal("lost response retry changed tokens or deadlines")
	}
	if _, err = owner.Exec(ctx, `UPDATE auth_sessions SET retry_until=now()-interval '1 second' WHERE user_id=$1::uuid`, id); err != nil {
		t.Fatal(err)
	}
	expect(request("POST", "/api/v1/auth/refresh", map[string]string{"refresh_token": pair.RefreshToken, "request_id": requestID}, ""), 401)
	expect(request("GET", "/api/v1/auth/me", nil, rotated.AccessToken), 200)
	logoutBody := map[string]string{"refresh_token": rotated.RefreshToken}
	expect(request("POST", "/api/v1/auth/logout", logoutBody, ""), 204)
	expect(request("POST", "/api/v1/auth/logout", logoutBody, ""), 204)
	expect(request("GET", "/api/v1/auth/me", nil, rotated.AccessToken), 401)
	expect(request("POST", "/api/v1/auth/refresh", map[string]string{"refresh_token": rotated.RefreshToken, "request_id": randomAuthToken(t)}, ""), 401)
	expect(request("GET", "/api/v1/auth/me", nil, device2.AccessToken), 200)
	hash := sha256.Sum256([]byte(device2.RefreshToken))
	if _, err = owner.Exec(ctx, `UPDATE auth_sessions SET access_expires_at=now()-interval '1 second' WHERE refresh_hash=$1`, hash[:]); err != nil {
		t.Fatal(err)
	}
	expect(request("GET", "/api/v1/auth/me", nil, device2.AccessToken), 401)
	device2 = decodePair(request("POST", "/api/v1/auth/refresh", map[string]string{"refresh_token": device2.RefreshToken, "request_id": randomAuthToken(t)}, ""))
	hash = sha256.Sum256([]byte(device2.RefreshToken))
	if _, err = owner.Exec(ctx, `UPDATE auth_sessions SET absolute_expires_at=now()-interval '1 second' WHERE refresh_hash=$1`, hash[:]); err != nil {
		t.Fatal(err)
	}
	expect(request("POST", "/api/v1/auth/refresh", map[string]string{"refresh_token": device2.RefreshToken, "request_id": randomAuthToken(t)}, ""), 401)
	if _, err = owner.Exec(ctx, `UPDATE auth_sessions SET absolute_expires_at=now()+interval '1 hour',refresh_expires_at=now()-interval '1 second' WHERE refresh_hash=$1`, hash[:]); err != nil {
		t.Fatal(err)
	}
	expect(request("POST", "/api/v1/auth/refresh", map[string]string{"refresh_token": device2.RefreshToken, "request_id": randomAuthToken(t)}, ""), 401)

	expect(login(email, "incorrect"), 401)
	for range 2 {
		expect(login(email, "incorrect"), 429)
	} // five account attempts per 15-minute window, including successful logins
	if err = migrations.Up(ctx, os.Getenv("TEST_MIGRATION_DATABASE_URL"), "tramas_test", "tramas_testapp"); err != nil {
		t.Fatal(err)
	}
	var preserved string
	if err = owner.QueryRow(ctx, `SELECT id::text FROM users WHERE email=$1`, email).Scan(&preserved); err != nil || preserved != id {
		t.Fatal("migration changed UUID")
	}
	if err = repository.Maintain(ctx); err != nil {
		t.Fatal(err)
	}
}

func TestSharedRateLimitConcurrent(t *testing.T) {
	app, _ := authTestPools(t)
	r := auth.PostgresAuthRepository{Pool: app}
	key := "concurrency/" + randomAuthToken(t)
	var wg sync.WaitGroup
	results := make(chan bool, 12)
	errs := make(chan error, 12)
	for range 12 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			ok, err := r.Allow(context.Background(), key, 3, auth.AccessTTL)
			results <- ok
			errs <- err
		}()
	}
	wg.Wait()
	close(results)
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	accepted := 0
	for ok := range results {
		if ok {
			accepted++
		}
	}
	if accepted != 3 {
		t.Fatalf("shared rate limit accepted %d", accepted)
	}
}

func TestRefreshCompetitionAndHashStorage(t *testing.T) {
	app, owner := authTestPools(t)
	ctx := context.Background()
	repo := auth.PostgresAuthRepository{Pool: app}
	email := "refresh-race-" + strings.ToLower(randomAuthToken(t)[:10]) + "@example.com"
	u, err := (users.PostgresRepository{Pool: app}).Create(ctx, email, "$argon2id$fixture")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = owner.Exec(ctx, `UPDATE users SET email_verified_at=now() WHERE id=$1::uuid`, u.ID); err != nil {
		t.Fatal(err)
	}
	original, err := repo.NewSession(ctx, u.ID, randomAuthToken(t), randomAuthToken(t))
	if err != nil {
		t.Fatal(err)
	}
	service := auth.SessionService{Repository: repo}
	var wg sync.WaitGroup
	type result struct {
		pair      auth.TokenPair
		err       error
		requestID string
	}
	results := make(chan result, 2)
	start := make(chan struct{})
	for range 2 {
		requestID := randomAuthToken(t)
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			p, err := service.Refresh(ctx, original.RefreshToken, requestID)
			results <- result{p, err, requestID}
		}()
	}
	close(start)
	wg.Wait()
	close(results)
	won, lost := 0, 0
	var winner result
	for r := range results {
		switch {
		case r.err == nil:
			won++
			winner = r
		case errors.Is(r.err, auth.ErrRefreshConflict):
			lost++
		default:
			t.Fatal(r.err)
		}
	}
	if won != 1 || lost != 1 {
		t.Fatalf("refresh race: %d won, %d lost", won, lost)
	}
	retried, err := service.Refresh(ctx, original.RefreshToken, winner.requestID)
	if err != nil || retried != winner.pair {
		t.Fatal("winning operation could not be retried", err)
	}
	var accessHash, refreshHash, previousHash, requestHash []byte
	if err = owner.QueryRow(ctx, `SELECT access_hash,refresh_hash,previous_refresh_hash,refresh_request_hash FROM auth_sessions WHERE user_id=$1::uuid`, u.ID).Scan(&accessHash, &refreshHash, &previousHash, &requestHash); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		stored []byte
		raw    string
	}{{accessHash, winner.pair.AccessToken}, {refreshHash, winner.pair.RefreshToken}, {previousHash, original.RefreshToken}, {requestHash, winner.requestID}} {
		hash := sha256.Sum256([]byte(tc.raw))
		if !bytes.Equal(tc.stored, hash[:]) || bytes.Equal(tc.stored, []byte(tc.raw)) {
			t.Fatal("credential storage is not hash-only")
		}
	}
	// Last refresh is clamped to the existing absolute deadline.
	if _, err = owner.Exec(ctx, `UPDATE auth_sessions SET absolute_expires_at=now()+interval '5 minutes' WHERE user_id=$1::uuid`, u.ID); err != nil {
		t.Fatal(err)
	}
	clamped, err := service.Refresh(ctx, winner.pair.RefreshToken, randomAuthToken(t))
	if err != nil {
		t.Fatal(err)
	}
	if !clamped.AccessExpiresAt.Equal(clamped.RefreshExpiresAt) {
		t.Fatal("absolute session deadline extended")
	}
}

func TestRegistrationAndMailJobAtomicity(t *testing.T) {
	app, owner := authTestPools(t)
	ctx := context.Background()
	// This is the isolated test role, verified by authTestPools before mutation.
	if _, err := owner.Exec(ctx, `REVOKE INSERT ON auth_email_verifications FROM tramas_testapp`); err != nil {
		t.Fatal(err)
	}
	defer func() {
		if _, err := owner.Exec(ctx, `GRANT INSERT ON auth_email_verifications TO tramas_testapp`); err != nil {
			t.Error(err)
		}
	}()
	email := "queue-rollback-" + strings.ToLower(randomAuthToken(t)[:10]) + "@example.com"
	if _, err := (users.PostgresRepository{Pool: app}).Create(ctx, email, "$argon2id$fixture"); err == nil {
		t.Fatal("queue insert failure accepted")
	}
	var count int
	if err := owner.QueryRow(ctx, `SELECT count(*) FROM users WHERE email=$1`, email).Scan(&count); err != nil || count != 0 {
		t.Fatal("account committed without delivery job", err)
	}
}
