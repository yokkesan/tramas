# General User Authentication

The backend uses opaque bearer access tokens and rotating refresh tokens stored
only as SHA-256 hashes in PostgreSQL. Registration keeps its existing request and
201 response contract. It does not authenticate the user. Administrator
authentication remains separate. Apple/Google OIDC, password reset, extended
profiles and favorites are described in [backend-features.md](backend-features.md).

## Endpoints

All POST/PATCH bodies are strict JSON objects containing only the listed string
fields. Unknown/duplicate fields, nulls, trailing JSON, invalid UTF-8 and bodies
over 8 KiB are rejected. Send `Content-Type: application/json`.
All authentication responses use `Cache-Control: no-store`.

| Method | Path | Body | Success |
|---|---|---|---|
| POST | `/api/v1/auth/register` | `email`, `password` | 201, existing user envelope |
| POST | `/api/v1/auth/email/verify` | `token` | 204 |
| POST | `/api/v1/auth/email/resend` | `email` | 202, no account disclosure |
| POST | `/api/v1/auth/login` | `email`, `password` | 200, `user` and `tokens` |
| POST | `/api/v1/auth/refresh` | `refresh_token`, `request_id` | 200, `tokens` |
| POST | `/api/v1/auth/logout` | `refresh_token` | 204, idempotent |
| GET | `/api/v1/auth/me` | None; bearer access token | 200, `user` |
| PATCH | `/api/v1/users/me/profile` | `nickname`; bearer access token | 200, `user` |

Bearer authentication uses `Authorization: Bearer <access_token>`. No cookies
are issued. Refresh tokens cannot be used as access tokens or verification tokens.
Logout requires possession of a refresh token, including a still-live session's
previous refresh token during the 60-second retry window; an access token is not
required. This permits logout after access-token expiry and before profile setup.
Unknown/already-revoked well-formed refresh tokens also return 204.

Login example response:

```json
{
  "user": {
    "id": "123e4567-e89b-42d3-a456-426614174000",
    "email": "user@example.com",
    "nickname": null,
    "email_verified": true,
    "profile_completed": false
  },
  "tokens": {
    "access_token": "<43-character base64url token>",
    "refresh_token": "<43-character base64url token>",
    "token_type": "Bearer",
    "access_expires_at": "2026-11-02T00:15:00Z",
    "refresh_expires_at": "2026-12-02T00:00:00Z"
  }
}
```

`GET /auth/me` and profile updates return the same `user` shape. The server owns
verification/profile-completion state; clients cannot submit flags, IDs or roles.
The existing registration response deliberately keeps only id/email/created_at.

## Email verification and delivery

Registration atomically creates the account and a durable delivery job. It does
not wait for mail delivery. Existing unverified accounts get a pending job when
migration 00002 is applied. The verification token is a random 256-bit value,
expires after 24 hours, and can be consumed once. Confirmation does not issue a
session. A successfully delivered replacement invalidates the previous token.

The queue and token state share one row per account. `DeliverOne` uses a row lock
and `SKIP LOCKED`, a five-second operation timeout, and a three-second sender
timeout. Sender implementations must honor context cancellation. Failed sends
preserve the previous token and retry after 1, 2, 4 and 8 minutes, up to five total
attempts. A later resend request can restart an exhausted job. Resend requests
do not reset a job that is already pending.

Delivery is at-least-once: external delivery and a PostgreSQL commit cannot be
atomic. A delivery acknowledged externally followed by DB commit failure can
produce an unusable link; the durable job remains retryable. Receipt immediately
before the commit can also briefly precede token availability. Adapters and
future confirmation screens must support retry/resend, and must not claim exactly
once delivery. Sender error details are never logged or returned.

`VerificationSender` receives email, raw token and expiry. Link creation,
templates and provider configuration belong to that adapter. No URL, website,
deep-link scheme or production provider is hard-coded. A future HTTPS link must
require an explicit POST confirmation; GET must not consume a token because mail
scanners can prefetch links.

### Local development inbox

In development, the API runs the queue dispatcher with `MemoryMailbox`. Messages
exist only in API memory, are never logged or written to disk, and disappear on
restart. At most 100 unexpired messages are retained; a full inbox causes delivery
retry. Restarting clears it; resend is needed for already-delivered lost messages.
The mailbox is intended only for the local loopback development stack.

Set a cryptographically random `AUTH_DEV_MAILBOX_KEY` of at least 32 bytes in the
ignored local environment file and recreate the API to enable:

```http
GET /dev/auth/mailbox
X-Dev-Mailbox-Key: <configured local key>
```

The response contains `messages` with email, token and expires_at. Copy the token
into a POST to `/api/v1/auth/email/verify` for local testing. Treat this header and
the response as secrets. Do not use query parameters or publish the local inbox.
The route is absent if the key is unset; invalid keys receive 404.

Production forbids this key, does not register the inbox, and has no configured
sender. Jobs remain pending and startup logs a fixed warning. Real verification
mail delivery requires a production adapter before public signup can be enabled.
The API currently owns background dispatch/cleanup; the same repository methods
can be moved to the dedicated worker when a provider is connected.

## Login, refresh and logout

Passwords are checked with the stored, bounded Argon2id PHC parameters and a
constant-time result comparison. Registration and login share concurrency and a
128 MiB aggregate Argon2 memory budget, including historical hashes with higher
memory settings. Missing/passwordless accounts use a dummy hash check. Login does
not apply the current registration password blocklist to existing passwords.

Correct credentials for an unverified account return 403
`email_verification_required`, without a session. Wrong passwords and missing
accounts both return 401 `invalid_credentials`.

Access tokens expire in 15 minutes. Refresh tokens expire after 30 days without
refresh; the session has a fixed 90-day absolute deadline. Refresh does not extend
that absolute deadline. Every successful refresh immediately invalidates the old
access token and replaces the refresh token. Logout revokes that session and its
access immediately, without logging out other devices.

For each refresh operation, the Flutter client must generate a fresh random
32-byte base64url-without-padding `request_id` (43 characters). Persist it with the
old refresh token before sending. Serialize refreshes across concurrent requests.
Retry a lost response using the exact same pair for up to 60 seconds. The backend
reproduces the same tokens and original expiry timestamps without storing raw
tokens: replacement values are domain-separated HMAC-SHA256 derivations keyed by
the old refresh token and the random request ID. Only hashes are persisted.

After success, atomically store the new pair and discard the old operation state.
Different request IDs competing for the same old refresh token return 409
`refresh_conflict` to the loser, without revoking the winner. Never respond to that
409 by logging out or deleting a newer pair already saved by another request.
After the retry window, old tokens return 401. If the new pair could not be
recovered, login again. Old-token reuse deliberately does not revoke the session;
this avoids denial of service from delayed requests and does not detect every
case of refresh-token theft. A later rotation ends the previous retry window.

## Nickname and authorization

Nickname is required to use ordinary features, can be changed later, and is not
an identifier. Duplicates are allowed. Optional profile fields are not required.

Both clients and the API must use the following rules:

1. Remove only leading/trailing ASCII U+0020 spaces.
2. Count Unicode scalar values: Go `utf8.RuneCountInString`, Dart `value.runes.length`.
3. Accept 2–20 scalar values. Do not normalize NFC/NFKC or count UTF-16 code units.
4. Reject C0/C1 controls (U+0000–001F, U+007F–009F), U+FFFD and malformed Unicode.
5. Reject values consisting solely of Unicode whitespace (Unicode White_Space).

For example, `😀` counts as one, `😀😀` as two, and `e` plus combining acute accent
counts as two although it looks like one character. Dart `String.length` and
grapheme-counting `characters.length` do not implement this contract.

Profile update stores nickname and profile_completed_at atomically. Later updates
preserve the original completion timestamp. DB checks enforce the length and
nickname/completion consistency; API validation applies the complete policy.

Use `RequireAuth()` to identify a verified user and `RequireProfile` for ordinary
application routes, in that order:

```go
ordinary := e.Group("/api/v1/app", handler.RequireAuth(), auth.RequireProfile)
// Register future ordinary feature routes on this group.
_ = ordinary
```

`me`, profile, refresh and logout do not require profile completion. There are no
ordinary business APIs in the current backend; middleware behavior is tested on
a test route. Health and existing development-only admin/demo routes retain their
existing behavior and are not a production administrator authentication system.

## Errors, throttling and transport

Errors use the existing `{ "error": { "code": "...", "message": "..." } }`
envelope. Branch on code, not the English message.

| Status | Code | Meaning |
|---|---|---|
| 400 | `invalid_json` | Invalid/duplicate/unknown JSON fields |
| 401 | `invalid_credentials` | Login credentials rejected |
| 401 | `invalid_token` | Token missing, malformed, expired, consumed or revoked |
| 403 | `email_verification_required` | Correct password, unverified email |
| 403 | `profile_required` | Ordinary route requested before profile setup |
| 409 | `refresh_conflict` | Another refresh request already won |
| 413 | `request_too_large` | More than 8 KiB |
| 415 | `unsupported_media_type` | JSON content type required |
| 422 | `invalid_nickname` | Nickname policy violation |
| 429 | `rate_limited` | Throttled; includes Retry-After |
| 500 | `internal_error` | Sanitized infrastructure failure |
| 503 | `authentication_busy` | Shared hash capacity full; Retry-After: 1 |

Verification failures intentionally do not distinguish expired, invalid and
consumed tokens. Resend returns the same 202 for nonexistent, verified, suppressed
or invalid email values, unless IP throttling or infrastructure failure occurs.

New routes share a bounded per-process IP ingress bucket (30/minute, burst 10),
plus PostgreSQL fixed-window per-route IP limits (30/minute, resend 10/hour).
Login additionally permits five account attempts per 15 minutes, including
successful attempts. Resend is limited to one/minute and five/hour per normalized
email, silently suppressing excess requests. IPv6 ingress/shared IP limits group
addresses by /64. Limits and token consumption use atomic SQL. Rate keys are
SHA-256 hashes of purpose-separated normalized identifiers, not plaintext; email
hashes are not encryption and do not protect against offline dictionary guessing.
Expired counters and sessions are removed in bounded background batches.

Registration retains its original IP bucket and duplicate-email 409 response;
that existing response reveals registration status. No new account enumeration
response is added by login or resend. See registration.md for the original policy.

Production requires HTTPS. Authorization headers, password bodies, refresh state,
mailbox keys and verification tokens must not be logged. API logging uses
route templates rather than query strings. Caddy's access-log filter removes
request/response headers and token/access_token/refresh_token query fields, using
the [official log filter configuration](https://caddyserver.com/docs/caddyfile/directives/log).
Tokens belong in OS-backed secure
storage on Flutter; no Flutter code was changed. Native clients do not require a
permissive CORS policy; cross-origin browser access remains disabled by default.

## Validation and deployment boundary

Apply migration 00002 explicitly before running the new API. Do not edit or rerun
00001 as a replacement. The app role gains password-hash SELECT and updates only
to verification/profile/timestamp columns. ID/password-hash updates, users DELETE,
DDL and migration-history access remain forbidden. Credentials are never exposed
through the account DTO.

Tests remain in `backend/tests/unit`, `backend/tests/api`, and
`backend/tests/integration`. Run the standalone `compose.test.yaml` stack for
PostgreSQL tests; host-only runs skip those tests without its environment. No
development/production migration is performed by tests.

Deferred work: production mail provider, sender/domain credentials, link landing
screen/deep-link configuration, production OIDC client settings, production storage
and player data source, production admin authentication, and Flutter integration.

## Changed implementation files

```text
backend/cmd/api/main.go
backend/internal/auth/mail.go
backend/internal/auth/password.go
backend/internal/auth/register_handler.go
backend/internal/auth/session_handler.go
backend/internal/auth/session_repository.go
backend/internal/auth/session_service.go
backend/internal/auth/tokens.go
backend/internal/users/repository.go
backend/migrations/00002_authentication.sql
backend/tests/api/auth/session_handler_test.go
backend/tests/integration/auth_test.go
backend/tests/integration/register_test.go
backend/tests/unit/auth/session_test.go
compose.yaml
docs/api/authentication.md
docs/api/registration.md
docs/database/migrations.md
infra/caddy/routes.caddy
```

The pre-existing `.env.example` modification is not part of this implementation.
