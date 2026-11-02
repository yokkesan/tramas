# User Backend Features

This extends [authentication.md](authentication.md). Existing registration and
authentication response contracts remain in place. Flutter is unchanged. All
`/api/v1` responses use the existing sanitized error envelope and no-store policy.
General users must authenticate; "public" profiles/favorites are visible to other
authenticated users whose initial nickname setup is complete, not anonymous web
visitors. Owner IDs always come from authentication, never request bodies.

## Apple / Google OIDC

The supported mobile flow uses the official Apple AuthenticationServices / Google
Sign-In SDK to obtain an ID token, then verifies it in Go and issues the existing
opaque Go session. Firebase Authentication and Supabase Auth are not used. This
does not implement a browser OAuth authorization-code redirect/exchange endpoint.

| Method | Path | Body / result |
|---|---|---|
| POST | `/api/v1/auth/oidc/:provider/challenge` | `purpose`, optional `reauth_token`; returns challenge_token, nonce, expires_in |
| POST | `/api/v1/auth/oidc/:provider/login` | challenge_token, id_token; returns existing user/tokens envelope |
| POST | `/api/v1/auth/reauthenticate` | Bearer + password; returns reauth_token, expires_in |
| POST | `/api/v1/auth/oidc/:provider/reauth` | Bearer + challenge_token, id_token; returns reauth_token, expires_in |
| POST | `/api/v1/auth/oidc/:provider/link` | Bearer + challenge_token, id_token; 204 |

`provider` is apple or google. All bodies are strict flat string-only JSON, maximum
8 KiB. Supported challenge purposes are login, reauth and link. Challenges and
reauth proofs expire in five minutes and are single use. Each challenge has an
independent random nonce. Only hashes of challenges, nonces and proofs persist.

For login:

1. Request `{ "purpose": "login" }`.
2. Pass the returned nonce unchanged in the provider authorization request through
   its official SDK. The ID token nonce claim must exactly match this value;
   configure SDK hashing behavior explicitly rather than applying an extra hash.
3. Send challenge_token and the provider's id_token to the login endpoint.
4. Store the returned access/refresh pair and use the existing refresh protocol.
   If profile_completed is false, request nickname setup.

The coreos/go-oidc verifier validates RS256 signatures using fixed official HTTPS
JWKS endpoints, issuer, accepted client audience and expiry. Additional checks
cover subject, nonce, required issued-at (no more than 30 seconds in the future),
not-before and authorized-party/client matching for multi-audience tokens. JWT
headers cannot select a URL or algorithm. Provider/JWKS errors and raw JWTs are
never logged. Keys are cached by the OIDC library; network calls have a timeout.

Identity lookup uses `(provider, subject)`, with a separate immutable internal
UUID. New external-only accounts have NULL local email/password, while a verified
provider email is private identity metadata. Thus an email matching a local
account, or matching the other provider, never merges users. The legacy `user`
DTO returns `email: ""` and `email_verified: false` for an external-only account;
this flag describes local email verification, not whether OIDC authentication
succeeded. RequireAuth independently accepts a registered external identity.

For explicit linking, first obtain a fresh reauth proof using the current
account's password or a fresh OIDC challenge for an identity already linked to
that exact account. Then request a link challenge with the bearer token and
reauth_token. That proof is consumed atomically when creating the bound challenge.
Completing a link requires the same authenticated account plus the new provider
token. A subject owned by another internal account receives 409
`identity_already_linked`; no account merge occurs. At most one identity per
provider is attached to an account. Password reset invalidates outstanding proofs
and bound challenges. Reauthentication and reset serialize against the user row.

Missing provider configuration returns 503 `provider_unavailable`; malformed,
expired, replayed, mismatched or invalid provider tokens return 401
`external_auth_failed`. Link/reauth require a bearer token but do not require
nickname completion. Reauthentication is limited to five password attempts per
account per 15 minutes, in addition to shared authentication IP limits.

Configuration:

| Variable | Meaning |
|---|---|
| `OIDC_APPLE_CLIENT_IDS` | Comma-separated allowed Apple client audiences; empty disables Apple |
| `OIDC_GOOGLE_CLIENT_IDS` | Comma-separated allowed Google client audiences; empty disables Google |

No production client IDs, private keys or secrets are embedded. ID-token
verification uses provider public keys and needs no client secret/private signing
key. Any future authorization-code exchange must load those secrets separately.
Official references: [Google backend ID-token authentication](https://developers.google.com/identity/sign-in/web/backend-auth),
[Apple user verification](https://developer.apple.com/documentation/signinwithapple/verifying-a-user),
[go-oidc](https://github.com/coreos/go-oidc).

## Profiles

| Method | Path | Authentication / result |
|---|---|---|
| GET | `/api/v1/users/me/profile` | Bearer, including incomplete setup; profile |
| PUT | `/api/v1/users/me/profile` | Bearer; provided nickname/biography fields updated |
| PATCH | `/api/v1/users/me/profile` | Existing nickname-only API; existing user response |
| GET | `/api/v1/users/:user_id/profile` | Completed caller; completed target's public profile |

PUT accepts nickname and/or biography as strings. Omitted fields are preserved;
`biography: ""` clears it. Nickname uses the existing 2–20 Unicode scalar-value
policy (`runes.length` in Dart), duplicates allowed. Updating biography alone
does not complete initial setup. Successful nickname update sets completion
atomically and later changes preserve its original timestamp.

There is no new business character limit for biography. The JSON transport budget
is 1 MiB to bound request memory; this is not a final biography length policy.
Clients must render biography as plain text, not trusted HTML. The public DTO
contains id, nickname, biography, current avatar/header metadata and standard image
IDs; it excludes email, credentials, provider subjects, storage keys and history.
Invalid UUIDs and non-public targets return 404. No update-by-other-user-ID route
exists; unknown owner/privilege fields in a body are rejected.

## Profile and header images

| Method | Path | Body / result |
|---|---|---|
| PUT | `/api/v1/users/me/profile/images/:kind` | Raw image bytes; returns image metadata |
| DELETE | `/api/v1/users/me/profile/images/:kind` | Clears selection; 204 |
| PUT | `/api/v1/users/me/profile/images/:kind/standard` | `{ "image_id": "configured-id" }`; 204 |
| GET | `/api/v1/profile/standard-images` | Configured standard image catalog |
| GET | `/api/v1/profile/images/:image_id` | Authenticated image download |

`kind` is avatar or header. Send one raw file per PUT with Content-Type image/jpeg,
image/png or image/heic; this endpoint does not use multipart, client filenames,
URLs or base64 JSON. Maximum file size is exactly 10 MiB (10 * 1024 * 1024 bytes).
Both Content-Length and streamed bodies are bounded. Claimed MIME must match the
validated format. Filename extension is not trusted or used for storage.

JPEG/PNG headers and complete decoding are checked, PNG chunk CRCs and complete
JPEG/PNG container endings are validated, and trailing/duplicate-ending payloads
are rejected. HEIC requires a HEVC HEIF brand, valid top-level box boundaries and
successful libheif decoding. SVG, GIF, AVIF and renamed non-images are rejected.
Decode work is serialized per API process. Excess work returns 503 `image_busy`.

Uploaded bytes are retained unchanged. No product crop, output resolution,
re-encoding format or set of standard images is fixed. A configurable decode
memory budget protects against decompression bombs; the default is 64 MiB with
a conservative 16-bytes-per-pixel estimate. Images exceeding that safety budget
receive 413 even if compressed bytes fit 10 MiB. This is an infrastructure limit,
not a settled product resolution requirement. Operators can adjust
`PROFILE_DECODE_BUDGET_BYTES` (1–256 MiB) with corresponding container resources.

HEIC validation uses a native subprocess with an empty credential environment,
five-second wall timeout, four-second CPU limit, 128 MiB address-space limit and
scratch-output size limit. A temporary decoded PNG is discarded and never becomes
the stored image. This provides resource containment, not a full security sandbox.
Production image processing/isolation remains part of the storage deployment
decision; the current production path has no storage adapter and rejects uploads.

`Storage` abstracts Put/Open/Delete. Development uses a private `os.Root` directory
on the `profile_media_dev` named volume, with random 256-bit keys and no static
filesystem route. File and directory traversal/symlink escape are rejected.
Only currently selected images are readable. Other users must have completed
setup and the image owner must have a public completed profile; the owner can
preview their own images before setup completion.

Image selection and metadata updates are transactional. Replaced/deleted objects
become unreadable immediately and are deleted afterward; failed deletes retain
metadata for bounded background retries. Database failure after a storage write
attempts object cleanup. A crash between external storage write and metadata
creation can still leave an untracked object; production object lifecycle/orphan
collection must be specified with the eventual storage adapter. No distributed
storage/database atomicity is claimed.

`PROFILE_STANDARD_IMAGES` is a configurable JSON catalog, e.g. objects with id,
kind and HTTPS/relative URL. It defaults to an empty array: no standard image kinds
or actual assets have been invented. Selection accepts only an ID configured for
that slot and clears an uploaded image selection. DELETE clears either selection.

Images return Content-Type and nosniff; no client-supplied filename is reflected.
There are no endpoints to attach an arbitrary uploaded object ID or modify
another user's image. Invalid content returns 422 `invalid_image`, excessive
resource/file size 413 `image_too_large`, and missing storage 503
`storage_unavailable`. Storage paths and errors are never returned or logged.

## Favorite players

All these routes require authentication and completed nickname setup. Protection
against automated abuse remains in effect; there is no business quota on lifetime
favorite or main changes.

| Method | Path | Result |
|---|---|---|
| GET | `/api/v1/players` | Eligible active Hanshin players |
| GET | `/api/v1/users/me/favorites` | Current favorites |
| POST | `/api/v1/users/me/favorites` | `{ "player_id": "uuid" }`; add, idempotent; 204 |
| DELETE | `/api/v1/users/me/favorites/:player_id` | Remove, idempotent; 204 |
| PUT | `/api/v1/users/me/favorites/:player_id/main` | Select a current favorite as main; 204 |
| DELETE | `/api/v1/users/me/favorites/main` | Clear main; 204 |
| GET | `/api/v1/users/:user_id/favorites` | Public current favorites only |
| GET | `/api/v1/users/me/favorites/history` | Owner's registration history |
| GET | `/api/v1/users/me/favorites/main-history` | Owner's main-selection history |

History pages contain at most 50 entries, newest first, and next_cursor (empty at
the end). Pass it as `?cursor=...` for the next page. Each page remains scoped to
the authenticated user, irrespective of any supplied query owner ID. Favorite
history includes registered_at, ended_at and end_reason (removed/ineligible).
Main history records each selection's started_at and ended_at. Re-selecting the
same current main is idempotent rather than creating duplicate history.

User-row locking and DB triggers enforce a maximum of three current favorites.
A unique partial index allows at most one active main selection. Player membership
is locked before user mutation. A narrowly scoped SECURITY DEFINER read-lock
function permits this without granting player updates to the app role.
The normal API role has SELECT only on players; authoritative ingestion is separate.

Updating a player to transferred/retired/departed, or moving away from internal
team code hanshin_tigers, closes current registrations and main-history intervals
in the same transaction. No replacement main is selected. Past records remain,
and public lists join current eligible membership as an additional check. Adding
an ineligible player returns 422 `ineligible_player`; a full current list returns
409 `favorite_limit`. Selecting an unregistered player as main returns 404.

There was no existing player DB model/source in this checkout; the HTML mockup is
explicitly non-authoritative. The players table therefore uses internal UUIDs and
stable `(source, source_id)` keys with name/team/career status. `PlayerSource` and
`UpsertPlayer` define the ingestion boundary; no production source is connected,
no ingestion HTTP endpoint is exposed, and no fictional players are seeded.
Development `/players` consequently returns an empty list until approved data is
imported. Test fixtures exist only in the isolated test database.

## Password reset

| Method | Path | Body / result |
|---|---|---|
| POST | `/api/v1/auth/password/reset/request` | email; always 202 for valid JSON absent infrastructure/IP errors |
| POST | `/api/v1/auth/password/reset/complete` | token, password; 204 |

Requests never distinguish unknown, passwordless/external-only, suppressed or
known local accounts. Per-email limits are one/minute and five/hour; IP limits
are ten/hour in addition to the shared ingress bucket. Only a local password
credential can be reset; matching external-provider metadata is not a credential.

Reset delivery extends the existing sender boundary with PasswordResetSender.
MemoryMailbox implements both sender interfaces and marks reset messages with
`kind: "password_reset"` (verification messages use email_verification). A durable
per-user queue retries with the existing five-attempt schedule. Failures preserve
the previously issued token; successful replacement invalidates it.

Tokens are random 256-bit values, hash-only at rest, one-use, with a configurable
30-minute default. `PASSWORD_RESET_TOKEN_TTL` accepts a duration from one minute
through 24 hours; the default is an implementation safety setting, not a final
product recovery policy. Link/screen/provider configuration is still external.

The new password must satisfy the existing Argon2id registration policy. Successful
reset changes the hash, consumes the reset token, revokes every existing session,
invalidates outstanding reauthentication proofs/challenges and appends a success
audit event in a single database transaction. It does not auto-login or mark a
local email verified. Login/proof creation rechecks the credential under a user
row lock to prevent an old password racing a reset from creating fresh access.

The app role still cannot directly UPDATE users.password_hash. A restricted
SECURITY DEFINER function requires the live hashed reset token and performs the
scoped mutation. PUBLIC execution is revoked. Audit rows record only event type,
internal user UUID where known and timestamp; no passwords, tokens, emails or raw
provider errors. Reset requests, rejected completions and successful completions
are audited. Audit retention has not been decided and no purge policy is invented.

## Deferred account operations

No email-change, account-delete or identity-unlink endpoints are introduced.
Their required decisions are:

- Email change: current/new email proof, collision handling, pending request
  lifetime, provider email distinction, notification and session invalidation.
- Account deletion: recoverability/grace period, profile/media/favorite-history
  deletion or anonymization, audit/backup retention, legal holds and external
  provider revocation responsibilities.
- Identity unlink: required reauthentication, last-login-method protection,
  provider grant revocation and associated session invalidation.

Production provider credentials, actual mail delivery, public confirmation/reset
landing pages or app links, production storage/isolation, standard image assets,
authoritative player ingestion and Flutter changes remain disconnected.

## Migrations and configuration

00002 was applied to the existing development DB first, after backup and database
identity checks, and the original authentication flow was verified over HTTP.
New migrations are forward-only:

- 00003: nullable local email for external-only users; external identities,
  one-use actions/proofs, reset queue and audit records; scoped reset function.
- 00004: profile text/slot selections and owned media metadata, including composite
  ownership foreign keys and 10 MiB/type constraints.
- 00005: player ingestion model, favorite registrations/main history, locking and
  membership-change triggers; no sample player rows.

Development adds a media volume; no existing database volume is removed or reset.
API memory budget is increased to 512 MiB with a 128 MiB scratch tmpfs for bounded
image decoding. Only development sets PROFILE_STORAGE_ROOT. Production intentionally
has no local storage configuration. Increase resource budgets deliberately when
raising decode safety settings. No production migration/deployment was performed.

Flutter integration work: official SDK configuration and nonce handoff, existing
token secure storage/refresh serialization, optional profile text and raw-byte image
uploads, standard catalog selection, current/history favorite lists and cursor
handling, reset request/confirmation screens and login after session revocation.
