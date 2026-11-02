# Backend Feature Verification — 2026-11-02

## Scope and result

Implemented and tested the confirmed backend scope using the existing Go/Echo/
PostgreSQL authentication infrastructure. No Flutter edits, production connection,
production migration, commit or push were performed. Existing uncommitted work
was preserved, including the pre-existing .env.example edit.

The complete API list, payloads and Flutter contracts are in
[backend-features.md](api/backend-features.md); the original authentication
contracts remain in [authentication.md](api/authentication.md).

## Development database and deployment

- Confirmed Compose project tramas-dev and actual database tramas_dev.
- Initially found two users and migration 00001 applied.
- Created and read-validated PostgreSQL custom-format backups before 00002 and
  again before 00003–00005. Both live in the local temporary directory
  `/private/tmp/tramas-dev-backup-20261102-37f65104/`:
  `before-authentication.dump` and `before-features.dump`.
- Backup directory mode is 0700; dump files are 0600. These are temporary local
  development backups, not an established durable production backup policy.
- Applied 00002 first. Rebuilt API/Caddy, verified healthy startup and eight live
  HTTP checks covering registration, verification, login, me, nickname, refresh,
  logout and refusal of the revoked access token.
- Applied 00003, 00004 and 00005 after the additional backup. Verified migration
  history through version 5. Rebuilt/restarted development API/Caddy successfully.
- Compared both original user rows against the pre-change dump after all changes:
  ID, email, password_hash, email_verified_at, created_at and updated_at match the
  backup byte-for-byte. Existing data was not deleted or reset.
- Live checks added two explicitly synthetic development accounts; total users
  are now four. They were retained rather than deleting development records.
- Development players table remains empty. No fictional production/development
  players were inserted. Favorite lifecycle fixtures were used only in tramas_test.
- Generated AUTH_DEV_MAILBOX_KEY in ignored local .env to enable the key-protected
  local inbox; its value was never printed. Development media uses a separate
  named volume; the existing PostgreSQL volume was preserved.

## Automated tests

Standalone test DB: postgres_test / tramas_test, using verified non-superuser
application and migration roles. No development database volumes were attached.

```sh
docker compose --env-file .env.test -f compose.test.yaml run --rm --build test -race -count=1 -json ./...
go vet ./...
go mod verify
```

Result: **133 passed, zero failed, zero skipped test cases**; 34 top-level tests
and 99 subtests. This includes the original 58 cases. Packages without test files
are not counted as skipped test cases. There were no race reports. Static analysis
and module verification passed.

Verified areas include:

- Both providers' signed ID tokens: issuer, audience, signature, nonce, expiry,
  issued-at/not-before, multi-audience authorized party and algorithm rejection.
- One-use/expired challenges, stable provider subjects, provider separation,
  concurrent first sign-in, explicit linking and proof consumption; matching email
  does not merge accounts. No production provider was contacted by tests.
- Self/public profile separation, ownership, nickname completion, optional long
  biography, parameterized SQL and rejection of owner-field injection.
- JPEG/PNG decoding, CRC/container endings, disguised/truncated/appended files,
  decoded memory limits, oversized uploads, file traversal and symlink isolation.
- Generated HEIC fixture decoding in the test container, and an additional direct
  codec check under the configured limits in the actual development API container.
  The generated fixture was never inserted as production data.
- Current image access and physical deletion, standard-image slot validation,
  cross-user mutation protection and incomplete-owner image privacy.
- Concurrent max-three favorites, main change/idempotency/history, transfer and
  retirement slot removal, no automatic main replacement and owner-only history.
- Reset request non-disclosure/throttling, delivery failure/retry, password policy,
  expiry/single consumption, audit transaction, global session revocation and stale
  password/proof rejection after reset.
- Anonymous access rejection across all new protected endpoint patterns.

Test log: `/private/tmp/tramas-features-tests.jsonl`. Test-container resources were
stopped after validation; only the development stack was left running.

## Additional live HTTP verification

Nineteen more checks passed against http://127.0.0.1:8080 after deployment:
registration/verification/login, extended profile get/update, image upload/download/
delete, empty eligible players, own favorites/history, standard catalog, reset
request/completion, rejection of the old session, new-password login, logout and
safe 503 responses for both unconfigured OIDC providers.

The tested password, tokens, email and inbox key were checked against API/Caddy
logs and did not appear. Caddy removes request/response headers and known secret
query fields. Live results without secrets are recorded at
`/private/tmp/tramas-dev-feature-results.json`.

## Pending connections and decisions

- Real Apple/Google client IDs, Flutter SDK setup and real provider sign-in tests.
- Production mail sender, domains/credentials and verification/reset landing
  pages or app links. Local mail adapters are not actual external delivery.
- Production object storage and processing isolation/lifecycle. Local storage is
  development-only. Native HEIC subprocess resource limits are not a full sandbox;
  cross-store crash orphan collection still needs a deployment design.
- Approved standard image catalog/assets and authoritative player ingestion.
- Product biography length and image crop/output/resolution policy; none was
  invented. Decode resource limits are configurable infrastructure safeguards.
- Email change, deletion and unlink policies listed in backend-features.md. Those
  endpoints are intentionally absent pending retention/recovery/security decisions.
- Audit retention and production monitoring/backup policies remain undecided.

## Files changed in this implementation

Modified existing files (including the previous uncommitted authentication work):

```text
backend/cmd/api/main.go
backend/go.mod
backend/go.sum
backend/internal/auth/mail.go
backend/internal/auth/session_handler.go
backend/internal/auth/session_repository.go
backend/internal/auth/session_service.go
compose.dev.yaml
compose.yaml
docs/api/authentication.md
docs/database/migrations.md
infra/caddy/routes.caddy
infra/docker/api/Dockerfile
infra/docker/test/Dockerfile
```

Added:

```text
backend/internal/auth/external_handler.go
backend/internal/auth/external_repository.go
backend/internal/auth/external_service.go
backend/internal/auth/oidc.go
backend/internal/auth/password_session.go
backend/internal/auth/reset.go
backend/internal/favorites/handler.go
backend/internal/favorites/repository.go
backend/internal/favorites/service.go
backend/internal/profile/handler.go
backend/internal/profile/image.go
backend/internal/profile/repository.go
backend/internal/profile/service.go
backend/internal/profile/storage.go
backend/migrations/00003_external_auth_and_password_reset.sql
backend/migrations/00004_profiles_and_media.sql
backend/migrations/00005_players_and_favorites.sql
backend/tests/integration/features_test.go
backend/tests/testutil/oidc.go
backend/tests/unit/auth/oidc_test.go
backend/tests/unit/profile/image_test.go
docs/api/backend-features.md
docs/backend-feature-verification.md
```

Ignored local .env also received the development inbox key. No other existing
environment values were replaced. Previous registration/00002 files remain in the
worktree, with the previous .env.example modification left untouched.
