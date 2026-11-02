package integration_test

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"image"
	"image/png"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/labstack/echo/v5"
	"tramas/backend/internal/auth"
	"tramas/backend/internal/favorites"
	"tramas/backend/internal/profile"
	"tramas/backend/internal/users"
	"tramas/backend/tests/testutil"
)

var featureIP atomic.Uint32

type featureEnv struct {
	t          *testing.T
	App, Owner *pgxpool.Pool
	E          *echo.Echo
	Auth       auth.PostgresAuthRepository
	Sessions   *auth.SessionService
	External   auth.ExternalService
	OIDC       *testutil.OIDCFixture
	Profiles   profile.Service
	Favorites  favorites.PostgresRepository
	Root, IP   string
}

func newFeatures(t *testing.T) *featureEnv {
	t.Helper()
	app, owner := authTestPools(t)
	r := auth.PostgresAuthRepository{Pool: app}
	cfg := auth.DefaultConfig()
	cfg.RateBurst = 1000
	cfg.RatePerMinute = 1000
	hasher, err := auth.NewArgonHasher(cfg)
	if err != nil {
		t.Fatal(err)
	}
	s, err := auth.NewSessionService(r, hasher)
	if err != nil {
		t.Fatal(err)
	}
	e := echo.New()
	h := auth.RegisterAuthentication(e, s, cfg)
	h.Limiter = auth.NewLimiter(cfg)
	fixture := testutil.NewOIDC(t)
	external := auth.ExternalService{Repository: r, Sessions: s, Verifier: fixture.Verifier}
	auth.RegisterExternal(e, h, external)
	policy, err := auth.NewPolicy(cfg)
	if err != nil {
		t.Fatal(err)
	}
	auth.RegisterReset(e, h, auth.ResetService{Repository: r, Sessions: s, Policy: policy})
	root := t.TempDir()
	store, err := profile.NewFileStorage(root)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	profiles := profile.Service{Repository: profile.PostgresRepository{Pool: app}, Storage: store, Validator: profile.NewImageValidator(profile.CommandHEICValidator{}, 64<<20), Standards: []profile.StandardImage{{ID: "test-avatar", Kind: "avatar", URL: "https://example.invalid/avatar.png"}}}
	profile.Register(e, h, profiles)
	fr := favorites.PostgresRepository{Pool: app}
	favorites.Register(e, h, favorites.Service{Repository: fr})
	return &featureEnv{t, app, owner, e, r, s, external, fixture, profiles, fr, root, fmt.Sprintf("203.0.113.%d", featureIP.Add(1))}
}
func (f *featureEnv) request(method, path string, body any, token string) *httptest.ResponseRecorder {
	f.t.Helper()
	var data []byte
	if b, ok := body.([]byte); ok {
		data = b
	} else {
		data, _ = json.Marshal(body)
	}
	return f.raw(method, path, data, "application/json", token)
}
func (f *featureEnv) raw(method, path string, data []byte, contentType, token string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, bytes.NewReader(data))
	req.RemoteAddr = f.IP + ":1234"
	req.Header.Set("Content-Type", contentType)
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	w := httptest.NewRecorder()
	f.E.ServeHTTP(w, req)
	return w
}
func (f *featureEnv) expect(w *httptest.ResponseRecorder, status int) {
	f.t.Helper()
	if w.Code != status {
		f.t.Fatalf("status %d want %d (response withheld to avoid credential output)", w.Code, status)
	}
}
func (f *featureEnv) account(complete bool) (users.User, string, auth.TokenPair) {
	f.t.Helper()
	ctx := context.Background()
	password := "feature test unique password " + randomAuthToken(f.t)
	hash, err := f.Sessions.Hasher.Hash(ctx, password)
	if err != nil {
		f.t.Fatal(err)
	}
	u, err := (users.PostgresRepository{Pool: f.App}).Create(ctx, "feature-"+strings.ToLower(randomAuthToken(f.t)[:12])+"@example.com", hash)
	if err != nil {
		f.t.Fatal(err)
	}
	if _, err = f.Owner.Exec(ctx, `UPDATE users SET email_verified_at=now() WHERE id=$1::uuid`, u.ID); err != nil {
		f.t.Fatal(err)
	}
	if complete {
		if _, err = f.Auth.Profile(ctx, u.ID, "テスト利用者"); err != nil {
			f.t.Fatal(err)
		}
	}
	pair, err := f.Auth.NewSession(ctx, u.ID, randomAuthToken(f.t), randomAuthToken(f.t))
	if err != nil {
		f.t.Fatal(err)
	}
	return u, password, pair
}
func decodeJSON[T any](t *testing.T, w *httptest.ResponseRecorder) T {
	t.Helper()
	var v T
	if err := json.Unmarshal(w.Body.Bytes(), &v); err != nil {
		t.Fatal("invalid response JSON")
	}
	return v
}
func (f *featureEnv) challenge(provider, purpose, token, proof string) auth.Challenge {
	f.t.Helper()
	w := f.request("POST", "/api/v1/auth/oidc/"+provider+"/challenge", map[string]string{"purpose": purpose, "reauth_token": proof}, token)
	f.expect(w, 200)
	return decodeJSON[auth.Challenge](f.t, w)
}
func (f *featureEnv) oidcLogin(provider, subject, email string) (auth.Account, auth.TokenPair) {
	f.t.Helper()
	c := f.challenge(provider, "login", "", "")
	raw := f.OIDC.Token(f.t, provider, subject, c.Nonce, email, nil)
	w := f.request("POST", "/api/v1/auth/oidc/"+provider+"/login", map[string]string{"challenge_token": c.Token, "id_token": raw}, "")
	f.expect(w, 200)
	v := decodeJSON[struct {
		User   auth.Account   `json:"user"`
		Tokens auth.TokenPair `json:"tokens"`
	}](f.t, w)
	return v.User, v.Tokens
}

func TestExternalIdentityFlowsPostgres(t *testing.T) {
	f := newFeatures(t)
	ctx := context.Background()
	local, password, localPair := f.account(true)
	subject := "subject-" + randomAuthToken(t)
	external, pair := f.oidcLogin("google", subject, local.Email)
	if external.ID == local.ID || external.Email != "" || external.ProfileCompleted {
		t.Fatal("email auto-link or unintended profile completion")
	}
	again, _ := f.oidcLogin("google", subject, "changed@example.com")
	if again.ID != external.ID {
		t.Fatal("provider subject did not retain UUID")
	}
	apple, _ := f.oidcLogin("apple", subject, local.Email)
	if apple.ID == external.ID || apple.ID == local.ID {
		t.Fatal("provider separation lost")
	}
	f.expect(f.request("GET", "/api/v1/players", nil, pair.AccessToken), 403)
	f.expect(f.request("PATCH", "/api/v1/users/me/profile", map[string]string{"nickname": "外部利用者"}, pair.AccessToken), 200)
	c := f.challenge("google", "login", "", "")
	raw := f.OIDC.Token(t, "google", subject, c.Nonce, local.Email, nil)
	f.expect(f.request("POST", "/api/v1/auth/oidc/google/login", map[string]string{"challenge_token": c.Token, "id_token": raw}, ""), 200)
	f.expect(f.request("POST", "/api/v1/auth/oidc/google/login", map[string]string{"challenge_token": c.Token, "id_token": raw}, ""), 401)
	expired := f.challenge("google", "login", "", "")
	hash := sha256.Sum256([]byte(expired.Token))
	if _, err := f.Owner.Exec(ctx, `UPDATE auth_actions SET expires_at=now()-interval '1 second' WHERE token_hash=$1`, hash[:]); err != nil {
		t.Fatal(err)
	}
	f.expect(f.request("POST", "/api/v1/auth/oidc/google/login", map[string]string{"challenge_token": expired.Token, "id_token": f.OIDC.Token(t, "google", subject, expired.Nonce, local.Email, nil)}, ""), 401)
	f.expect(f.request("POST", "/api/v1/auth/oidc/google/challenge", map[string]string{"purpose": "link"}, localPair.AccessToken), 401)
	f.expect(f.request("POST", "/api/v1/auth/reauthenticate", map[string]string{"password": "wrong"}, localPair.AccessToken), 401)
	reauth := f.request("POST", "/api/v1/auth/reauthenticate", map[string]string{"password": password}, localPair.AccessToken)
	f.expect(reauth, 200)
	proof := decodeJSON[map[string]any](t, reauth)["reauth_token"].(string)
	link := f.challenge("google", "link", localPair.AccessToken, proof)
	f.expect(f.request("POST", "/api/v1/auth/oidc/google/challenge", map[string]string{"purpose": "link", "reauth_token": proof}, localPair.AccessToken), 401)
	f.expect(f.request("POST", "/api/v1/auth/oidc/google/link", map[string]string{"challenge_token": link.Token, "id_token": f.OIDC.Token(t, "google", subject, link.Nonce, local.Email, nil)}, localPair.AccessToken), 409)
	reauth = f.request("POST", "/api/v1/auth/reauthenticate", map[string]string{"password": password}, localPair.AccessToken)
	f.expect(reauth, 200)
	proof = decodeJSON[map[string]any](t, reauth)["reauth_token"].(string)
	link = f.challenge("google", "link", localPair.AccessToken, proof)
	newSubject := "linked-" + randomAuthToken(t)
	f.expect(f.request("POST", "/api/v1/auth/oidc/google/link", map[string]string{"challenge_token": link.Token, "id_token": f.OIDC.Token(t, "google", newSubject, link.Nonce, local.Email, nil)}, localPair.AccessToken), 204)
	linked, _ := f.oidcLogin("google", newSubject, local.Email)
	if linked.ID != local.ID {
		t.Fatal("explicit linking changed account UUID")
	}
	// External-only accounts can reauthenticate through an existing provider.
	reauthChallenge := f.challenge("google", "reauth", pair.AccessToken, "")
	rw := f.request("POST", "/api/v1/auth/oidc/google/reauth", map[string]string{"challenge_token": reauthChallenge.Token, "id_token": f.OIDC.Token(t, "google", subject, reauthChallenge.Nonce, local.Email, nil)}, pair.AccessToken)
	f.expect(rw, 200)
	proof = decodeJSON[map[string]any](t, rw)["reauth_token"].(string)
	link = f.challenge("apple", "link", pair.AccessToken, proof)
	f.expect(f.request("POST", "/api/v1/auth/oidc/apple/link", map[string]string{"challenge_token": link.Token, "id_token": f.OIDC.Token(t, "apple", "other-"+subject, link.Nonce, local.Email, nil)}, pair.AccessToken), 204)
}

func TestConcurrentExternalFirstLogin(t *testing.T) {
	f := newFeatures(t)
	ctx := context.Background()
	subject := "race-" + randomAuthToken(t)
	var wg sync.WaitGroup
	ids := make(chan string, 2)
	errs := make(chan error, 2)
	for range 2 {
		c, err := f.External.Challenge(ctx, "google", "login", "", "")
		if err != nil {
			t.Fatal(err)
		}
		raw := f.OIDC.Token(t, "google", subject, c.Nonce, "same@example.com", nil)
		wg.Add(1)
		go func() {
			defer wg.Done()
			a, _, _, err := f.External.Complete(ctx, "google", "login", "", c.Token, raw)
			ids <- a.ID
			errs <- err
		}()
	}
	wg.Wait()
	close(ids)
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	first := ""
	for id := range ids {
		if id == "" || first != "" && id != first {
			t.Fatal("duplicate external users")
		}
		first = id
	}
}

func TestProfilesAndImageOwnershipPostgres(t *testing.T) {
	f := newFeatures(t)
	u, _, pair := f.account(false)
	other, _, otherPair := f.account(true)
	f.expect(f.request("GET", "/api/v1/users/me/profile", nil, pair.AccessToken), 200)
	f.expect(f.request("PUT", "/api/v1/users/me/profile", map[string]string{"biography": strings.Repeat("自己紹介", 3000)}, pair.AccessToken), 200)
	f.expect(f.request("GET", "/api/v1/players", nil, pair.AccessToken), 403)
	var imageData bytes.Buffer
	if err := png.Encode(&imageData, image.NewRGBA(image.Rect(0, 0, 16, 16))); err != nil {
		t.Fatal(err)
	}
	upload := f.raw("PUT", "/api/v1/users/me/profile/images/avatar", imageData.Bytes(), "image/png", pair.AccessToken)
	f.expect(upload, 200)
	media := decodeJSON[struct {
		Image profile.Image `json:"image"`
	}](t, upload).Image
	ownImage := f.request("GET", "/api/v1/profile/images/"+media.ID, nil, pair.AccessToken)
	f.expect(ownImage, 200)
	if !bytes.Equal(ownImage.Body.Bytes(), imageData.Bytes()) {
		t.Fatal("image bytes changed")
	}
	f.expect(f.request("GET", "/api/v1/profile/images/"+media.ID, nil, otherPair.AccessToken), 404)
	f.expect(f.request("PUT", "/api/v1/users/me/profile", map[string]string{"nickname": "プロフィール"}, pair.AccessToken), 200)
	f.expect(f.request("GET", "/api/v1/profile/images/"+media.ID, nil, otherPair.AccessToken), 200)
	public := f.request("GET", "/api/v1/users/"+u.ID+"/profile", nil, otherPair.AccessToken)
	f.expect(public, 200)
	if strings.Contains(public.Body.String(), u.Email) || strings.Contains(public.Body.String(), "storage_key") || strings.Contains(public.Body.String(), "password") {
		t.Fatal("private profile fields exposed")
	}
	f.expect(f.request("PUT", "/api/v1/users/me/profile", map[string]string{"user_id": other.ID, "nickname": "攻撃"}, pair.AccessToken), 400)
	f.expect(f.request("PUT", "/api/v1/users/"+u.ID+"/profile", map[string]string{"nickname": "攻撃"}, otherPair.AccessToken), 405)
	f.expect(f.raw("PUT", "/api/v1/users/me/profile/images/header", imageData.Bytes(), "image/jpeg", pair.AccessToken), 422)
	f.expect(f.raw("PUT", "/api/v1/users/me/profile/images/header", []byte("<svg/>"), "image/png", pair.AccessToken), 422)
	f.expect(f.raw("PUT", "/api/v1/users/me/profile/images/header", []byte("<svg/>"), "image/svg+xml", pair.AccessToken), 415)
	f.expect(f.raw("PUT", "/api/v1/users/me/profile/images/header", []byte(strings.Repeat("x", profile.MaxImageBytes+1)), "image/png", pair.AccessToken), 413)
	f.expect(f.request("DELETE", "/api/v1/users/me/profile/images/avatar", nil, otherPair.AccessToken), 204)
	f.expect(f.request("GET", "/api/v1/profile/images/"+media.ID, nil, pair.AccessToken), 200)
	f.expect(f.request("DELETE", "/api/v1/users/me/profile/images/avatar", nil, pair.AccessToken), 204)
	f.expect(f.request("GET", "/api/v1/profile/images/"+media.ID, nil, pair.AccessToken), 404)
	entries, err := os.ReadDir(f.Root)
	if err != nil || len(entries) != 0 {
		t.Fatal("deleted image bytes retained", err)
	}
	f.expect(f.request("PUT", "/api/v1/users/me/profile/images/avatar/standard", map[string]string{"image_id": "test-avatar"}, pair.AccessToken), 204)
	f.expect(f.request("PUT", "/api/v1/users/me/profile/images/header/standard", map[string]string{"image_id": "test-avatar"}, pair.AccessToken), 422)
	f.expect(f.request("GET", "/api/v1/profile/standard-images", nil, pair.AccessToken), 200)
	// Parameterized biography preserves apostrophes as data.
	f.expect(f.request("PUT", "/api/v1/users/me/profile", map[string]string{"biography": "'; DROP TABLE users; --"}, pair.AccessToken), 200)
}

type failedResetSender struct{}

func (failedResetSender) SendPasswordReset(context.Context, auth.VerificationMail) error {
	return fmt.Errorf("provider secret: never log this")
}
func TestPasswordResetAndSessionRevocation(t *testing.T) {
	f := newFeatures(t)
	ctx := context.Background()
	u, password, pair := f.account(true)
	old, err := f.Auth.Credentials(ctx, u.Email)
	if err != nil {
		t.Fatal(err)
	}
	proofResponse := f.request("POST", "/api/v1/auth/reauthenticate", map[string]string{"password": password}, pair.AccessToken)
	f.expect(proofResponse, 200)
	proof := decodeJSON[map[string]any](t, proofResponse)["reauth_token"].(string)
	known := f.request("POST", "/api/v1/auth/password/reset/request", map[string]string{"email": u.Email}, "")
	unknown := f.request("POST", "/api/v1/auth/password/reset/request", map[string]string{"email": "unknown-" + u.Email}, "")
	f.expect(known, 202)
	f.expect(unknown, 202)
	if known.Body.String() != unknown.Body.String() {
		t.Fatal("reset request enumerates email")
	}
	f.expect(f.request("POST", "/api/v1/auth/password/reset/request", map[string]string{"email": u.Email}, ""), 202)
	if done, err := f.Auth.DeliverResetOne(ctx, failedResetSender{}); err != nil || !done {
		t.Fatal(err)
	}
	if _, err = f.Owner.Exec(ctx, `UPDATE auth_password_resets SET next_attempt_at=now()-interval '1 day' WHERE user_id=$1::uuid`, u.ID); err != nil {
		t.Fatal(err)
	}
	mailbox := &auth.MemoryMailbox{}
	if done, err := f.Auth.DeliverResetOne(ctx, mailbox); err != nil || !done {
		t.Fatal(err)
	}
	messages := mailbox.Messages()
	if len(messages) != 1 || messages[0].Kind != "password_reset" {
		t.Fatal("reset mail missing")
	}
	token := messages[0].Token
	newPassword := "a newly generated secure password " + randomAuthToken(t)
	f.expect(f.request("POST", "/api/v1/auth/password/reset/complete", map[string]string{"token": token, "password": "short"}, ""), 422)
	if _, err = f.Owner.Exec(ctx, `UPDATE auth_password_resets SET expires_at=now()-interval '1 second' WHERE user_id=$1::uuid`, u.ID); err != nil {
		t.Fatal(err)
	}
	f.expect(f.request("POST", "/api/v1/auth/password/reset/complete", map[string]string{"token": token, "password": newPassword}, ""), 401)
	if _, err = f.Owner.Exec(ctx, `UPDATE auth_password_resets SET expires_at=now()+interval '1 hour' WHERE user_id=$1::uuid`, u.ID); err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	statuses := make(chan int, 2)
	for range 2 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			statuses <- f.request("POST", "/api/v1/auth/password/reset/complete", map[string]string{"token": token, "password": newPassword}, "").Code
		}()
	}
	wg.Wait()
	close(statuses)
	counts := map[int]int{}
	for status := range statuses {
		counts[status]++
	}
	if counts[204] != 1 || counts[401] != 1 {
		t.Fatalf("reset is not single use: %v", counts)
	}
	f.expect(f.request("GET", "/api/v1/auth/me", nil, pair.AccessToken), 401)
	f.expect(f.request("POST", "/api/v1/auth/refresh", map[string]string{"refresh_token": pair.RefreshToken, "request_id": randomAuthToken(t)}, ""), 401)
	if _, err = f.Auth.NewPasswordSession(ctx, u.ID, old.Hash, randomAuthToken(t), randomAuthToken(t)); err != auth.ErrCredentials {
		t.Fatal("old password resurrected a session", err)
	}
	if err = f.Auth.CreatePasswordProof(ctx, randomAuthToken(t), u.ID, old.Hash); err != auth.ErrCredentials {
		t.Fatal("old password reauthentication survived reset", err)
	}
	if err = f.Auth.ConsumeProof(ctx, proof, u.ID); err != auth.ErrCredentials {
		t.Fatal("old reauthentication proof survived reset", err)
	}
	f.expect(f.request("POST", "/api/v1/auth/login", map[string]string{"email": u.Email, "password": password}, ""), 401)
	f.expect(f.request("POST", "/api/v1/auth/login", map[string]string{"email": u.Email, "password": newPassword}, ""), 200)
	var completed, requested int
	if err = f.Owner.QueryRow(ctx, `SELECT count(*) FILTER(WHERE event='password_reset_completed'),count(*) FILTER(WHERE event='password_reset_requested') FROM auth_audit_events WHERE user_id=$1::uuid`, u.ID).Scan(&completed, &requested); err != nil || completed != 1 || requested != 1 {
		t.Fatal("reset audit not atomic", err)
	}
}

func TestFavoritesLifecycleAndConcurrency(t *testing.T) {
	f := newFeatures(t)
	ctx := context.Background()
	u, _, pair := f.account(true)
	other, _, otherPair := f.account(true)
	// Only this isolated DB receives fictional fixtures.
	ids := []string{}
	source := "test-fixture-" + randomAuthToken(t)
	for i := 0; i < 5; i++ {
		var id string
		if err := f.Owner.QueryRow(ctx, `INSERT INTO players(source,source_id,display_name,team_code,career_status) VALUES($1,$2,$3,'hanshin_tigers','active') RETURNING id::text`, source, fmt.Sprint(i), fmt.Sprintf("isolated fixture %d", i)).Scan(&id); err != nil {
			t.Fatal(err)
		}
		ids = append(ids, id)
	}
	var wg sync.WaitGroup
	results := make(chan error, 5)
	for _, id := range ids {
		wg.Add(1)
		go func() { defer wg.Done(); results <- f.Favorites.Add(ctx, u.ID, id) }()
	}
	wg.Wait()
	close(results)
	added, limited := 0, 0
	for err := range results {
		if err == nil {
			added++
		} else if err == favorites.ErrLimit {
			limited++
		} else {
			t.Fatal(err)
		}
	}
	if added != 3 || limited != 2 {
		t.Fatalf("max-three race %d/%d", added, limited)
	}
	current, err := f.Favorites.List(ctx, u.ID, false)
	if err != nil || len(current) != 3 {
		t.Fatal("wrong active count", err)
	}
	first, second := current[0].Player.ID, current[1].Player.ID
	f.expect(f.request("PUT", "/api/v1/users/me/favorites/"+first+"/main", nil, pair.AccessToken), 204)
	f.expect(f.request("PUT", "/api/v1/users/me/favorites/"+second+"/main", nil, pair.AccessToken), 204)
	f.expect(f.request("PUT", "/api/v1/users/me/favorites/"+second+"/main", nil, pair.AccessToken), 204)
	history, err := f.Favorites.MainHistory(ctx, u.ID, nil)
	if err != nil || len(history) != 2 {
		t.Fatal("main history missing/idempotency broken", err)
	}
	if _, err = f.Owner.Exec(ctx, `UPDATE players SET career_status='transferred',team_code='other_team' WHERE id=$1::uuid`, second); err != nil {
		t.Fatal(err)
	}
	current, err = f.Favorites.List(ctx, u.ID, true)
	if err != nil || len(current) != 2 {
		t.Fatal("departed player still occupies slot", err)
	}
	for _, v := range current {
		if v.Main {
			t.Fatal("main auto-selected")
		}
	}
	all, err := f.Favorites.History(ctx, u.ID, nil)
	if err != nil || len(all) != 3 {
		t.Fatal("registration history lost", err)
	}
	ended := false
	for _, v := range all {
		if v.Player.ID == second {
			ended = v.EndedAt != nil && v.EndReason != nil && *v.EndReason == "ineligible"
		}
	}
	if !ended {
		t.Fatal("transfer history not closed")
	}
	history, err = f.Favorites.MainHistory(ctx, u.ID, nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, m := range history {
		if m.EndedAt == nil {
			t.Fatal("departed main retained")
		}
	}
	f.expect(f.request("POST", "/api/v1/users/me/favorites", map[string]string{"player_id": second}, pair.AccessToken), 422)
	replacement := ""
	for _, id := range ids {
		found := false
		for _, v := range all {
			if id == v.Player.ID {
				found = true
			}
		}
		if !found {
			replacement = id
			break
		}
	}
	f.expect(f.request("POST", "/api/v1/users/me/favorites", map[string]string{"player_id": replacement}, pair.AccessToken), 204)
	f.expect(f.request("DELETE", "/api/v1/users/me/favorites/"+first, nil, otherPair.AccessToken), 204)
	others, err := f.Favorites.List(ctx, other.ID, false)
	if err != nil || len(others) != 0 {
		t.Fatal("cross-user mutation")
	}
	current, err = f.Favorites.List(ctx, u.ID, false)
	if err != nil || len(current) != 3 {
		t.Fatal("cross-user removal changed owner")
	}
	f.expect(f.request("GET", "/api/v1/users/"+u.ID+"/favorites", nil, otherPair.AccessToken), 200)
	f.expect(f.request("GET", "/api/v1/users/me/favorites/history", nil, pair.AccessToken), 200)
	f.expect(f.request("GET", "/api/v1/users/me/favorites/main-history", nil, pair.AccessToken), 200)
	f.expect(f.request("GET", "/api/v1/users/"+u.ID+"/favorites/history", nil, otherPair.AccessToken), 404)
	f.expect(f.request("DELETE", "/api/v1/users/me/favorites/main", nil, pair.AccessToken), 204)
	// Retired/departed active slots are closed by authoritative DB updates too.
	if _, err = f.Owner.Exec(ctx, `UPDATE players SET career_status='retired' WHERE id=$1::uuid`, first); err != nil {
		t.Fatal(err)
	}
	current, err = f.Favorites.List(ctx, u.ID, false)
	if err != nil || len(current) != 2 {
		t.Fatal("retired player retained")
	}
	f.expect(f.request("GET", "/api/v1/players", nil, pair.AccessToken), 200)
}

func TestNewProtectedAPIsRejectAnonymous(t *testing.T) {
	f := newFeatures(t)
	for _, tc := range []struct{ method, path string }{
		{"POST", "/api/v1/auth/reauthenticate"}, {"POST", "/api/v1/auth/oidc/google/link"}, {"POST", "/api/v1/auth/oidc/google/reauth"},
		{"GET", "/api/v1/users/me/profile"}, {"PUT", "/api/v1/users/me/profile"}, {"GET", "/api/v1/users/00000000-0000-0000-0000-000000000000/profile"},
		{"PUT", "/api/v1/users/me/profile/images/avatar"}, {"DELETE", "/api/v1/users/me/profile/images/avatar"}, {"PUT", "/api/v1/users/me/profile/images/avatar/standard"},
		{"GET", "/api/v1/profile/standard-images"}, {"GET", "/api/v1/profile/images/00000000-0000-0000-0000-000000000000"},
		{"GET", "/api/v1/players"}, {"GET", "/api/v1/users/me/favorites"}, {"POST", "/api/v1/users/me/favorites"}, {"DELETE", "/api/v1/users/me/favorites/00000000-0000-0000-0000-000000000000"},
		{"PUT", "/api/v1/users/me/favorites/00000000-0000-0000-0000-000000000000/main"}, {"DELETE", "/api/v1/users/me/favorites/main"},
		{"GET", "/api/v1/users/me/favorites/history"}, {"GET", "/api/v1/users/me/favorites/main-history"}, {"GET", "/api/v1/users/00000000-0000-0000-0000-000000000000/favorites"},
	} {
		t.Run(tc.method+" "+tc.path, func(t *testing.T) {
			w := f.request(tc.method, tc.path, map[string]string{}, "")
			if w.Code != 401 {
				t.Fatalf("anonymous status %d", w.Code)
			}
		})
	}
}
