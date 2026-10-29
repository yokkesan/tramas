# User Registration API

`POST /api/v1/auth/register`

Content type: `application/json`. Maximum body size: 8 KiB.

```json
{
  "email": "user@example.com",
  "password": "a unique long passphrase"
}
```

Only `email` and `password` are accepted. Unknown fields, duplicate JSON keys,
non-string values, null values, invalid UTF-8, and trailing JSON documents are rejected.

## Responses

Successful registration returns HTTP 201:

```json
{
  "user": {
    "id": "123e4567-e89b-42d3-a456-426614174000",
    "email": "user@example.com",
    "created_at": "2026-10-29T00:00:00Z"
  }
}
```

The UUID is assigned by PostgreSQL. Registration does not verify the email address,
log the user in, set an authentication cookie, or issue any token.

Errors use the same envelope:

```json
{
  "error": {
    "code": "validation_error",
    "message": "Invalid registration input.",
    "fields": {
      "email": "invalid"
    }
  }
}
```

| Status | Code | Meaning |
|---|---|---|
| 400 | `invalid_json` | Invalid JSON, unknown or duplicate field |
| 405 | `request_error` | Unsupported HTTP method |
| 409 | `email_already_registered` | Email already registered |
| 413 | `request_too_large` | Request body exceeds 8 KiB |
| 415 | `unsupported_media_type` | Content type is not JSON |
| 422 | `validation_error` | Invalid email or password |
| 429 | `rate_limited` | Registration rate limit exceeded |
| 500 | `internal_error` | Generic internal failure |
| 503 | `registration_busy` | Hashing concurrency limit reached |

429 and 503 include `Retry-After` in seconds. Responses use `Cache-Control: no-store`.
Clients should branch on `code`, not the English message. Error fields never echo
submitted values. The 409 response intentionally distinguishes registered emails;
account enumeration remains a tradeoff of this response contract.

## Provisional Policy

These settings are temporary defaults, not the final authentication specification.

- Email: trim surrounding whitespace, lowercase the entire address, require an
  ASCII address with a dotted domain; maximum 254 bytes and local part 64 bytes.
  Display names, quoted local parts and internationalized addresses are not supported.
- Password: 15–128 Unicode code points; no mandatory character-class combination.
  Passwords are not trimmed, case-folded, normalized, or truncated before hashing.
  Empty/whitespace-only values, controls, and replacement characters are rejected.
- A small embedded list rejects known weak/example passwords. It is a provisional
  baseline, not a comprehensive breached-password corpus. An additional local list
  can be configured; its source, coverage and update process require review before public release.
- Argon2id: version 19, memory 19 MiB, iterations 2, parallelism 1, random 16-byte
  salt, 32-byte output. Parameters and salt are stored in PHC format.
- At most two hashing operations run concurrently per API process. Excess requests
  are rejected without queuing. The service context has a five-second deadline;
  an Argon2 operation already running finishes before its slot is released.
- Rate limit: per-IP token bucket, refill five tokens per minute, burst two. IPv6
  addresses share a bucket per /64. The in-memory client map is bounded and expires
  idle entries. Limits reset on process restart and are not shared across replicas.
  Multiple API replicas require a shared limiter or equivalent edge enforcement.

| Environment variable | Default |
|---|---:|
| `REGISTER_PASSWORD_MIN` | 15 |
| `REGISTER_PASSWORD_MAX` | 128 |
| `REGISTER_ARGON_MEMORY_KIB` | 19456 |
| `REGISTER_ARGON_ITERATIONS` | 2 |
| `REGISTER_ARGON_PARALLELISM` | 1 |
| `REGISTER_HASH_CONCURRENCY` | 2 |
| `REGISTER_RATE_PER_MINUTE` | 5 |
| `REGISTER_RATE_BURST` | 2 |
| `REGISTER_RATE_MAX_CLIENTS` | 10000 |
| `REGISTER_BLOCKLIST_PATH` | Empty; optional additional UTF-8 list |

Each non-comment blocklist line is one blocked password, compared case-insensitively
for screening only. Mount an additional list read-only and set its container path.
Invalid configurations fail API startup. Argon settings and aggregate hashing memory
have safety bounds to match the current container resource limits.

The Argon2 defaults follow the [OWASP baseline](https://cheatsheetseries.owasp.org/cheatsheets/Password_Storage_Cheat_Sheet.html).
Calibration on the deployment hardware and a final password policy remain pending.

## Proxy and Transport

The API trusts forwarded IPs only from `TRUSTED_PROXY_CIDRS`. Compose sets this to
Caddy's fixed address on a dedicated internal proxy network. Caddy replaces untrusted
forwarded-address input. The dynamic allocation range excludes Caddy's fixed address.
The API has no published host port. Never configure all clients or all private
networks as trusted proxies.

Local Caddy accepts HTTP on `127.0.0.1:8080`. Production configuration requires a
separate environment file and HTTPS domain. No production deployment was performed.
Docker-internal database traffic currently uses the isolated network without TLS;
database TLS and the deployment threat model still require production review.

API access logs contain method, route template, status and latency, without request
bodies, query strings, credentials or raw driver errors. Registration failures are
logged by category rather than raw error text.

## Flutter Integration

No Flutter code was changed.

| Environment | Base URL |
|---|---|
| iOS Simulator | `http://127.0.0.1:8080` |
| Android Emulator | `http://10.0.2.2:8080` |
| Production | Configured HTTPS origin; not yet assigned |

Append `/api/v1/auth/register`, send JSON, and handle the status/code combinations
above. No authorization header is required or returned. A 201 result means the
account was created; it does not mean the user is authenticated or email-verified.
Physical-device access is not configured because the local port is loopback-only.

Email verification/delivery, login, sessions, access/refresh tokens, password reset,
Apple/Google authentication and account linking are outside this implementation.

## Implementation Files

```text
.env.example
backend/cmd/api/main.go
backend/cmd/migrate/main.go
backend/go.mod
backend/go.sum
backend/tests/integration/register_test.go
backend/internal/auth/common-passwords.txt
backend/internal/auth/config.go
backend/internal/auth/password.go
backend/tests/unit/auth/password_test.go
backend/internal/auth/rate_limit.go
backend/tests/unit/auth/rate_limit_test.go
backend/internal/auth/register_handler.go
backend/tests/api/auth/register_handler_test.go
backend/internal/auth/register_service.go
backend/internal/auth/validation.go
backend/tests/unit/auth/validation_test.go
backend/internal/users/repository.go
backend/internal/users/user.go
backend/migrations/00001_create_users.sql
backend/migrations/migrate.go
backend/tests/unit/migrations/migrate_test.go
compose.migrate.yaml
compose.test.yaml
compose.yaml
docs/api/registration.md
docs/database/migrations.md
infra/docker/migrate/Dockerfile
infra/docker/postgres/02-migrator.sql
infra/docker/test/Dockerfile
scripts/setup-dev-env.py
scripts/setup-test-env.py
```
