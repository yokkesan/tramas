# Registration Schema and Migrations

SQL migrations use goose 3.29.0, embedded in `backend/cmd/migrate`.
The command accepts only `up`. No destructive Down migration is included.
Goose applies the migration and version record transactionally, with a PostgreSQL
session advisory lock to serialize migration execution.

## Schema

`00001_create_users.sql` creates `users`:

| Column | Type | Constraint or Default |
|---|---|---|
| `id` | UUID | Primary key, `gen_random_uuid()` |
| `email` | text | Not null, unique, lowercase/trimmed, 3–254 bytes |
| `password_hash` | text | Null or Argon2id-prefixed PHC value |
| `email_verified_at` | timestamptz | Null by default |
| `created_at` | timestamptz | Not null, `now()` |
| `updated_at` | timestamptz | Not null, `now()` |

The email uniqueness constraint is named `users_email_key`. Only this specific
unique violation maps to the API's duplicate-email error. One parameterized
`INSERT ... RETURNING` is atomic; competing requests are resolved by the database.

No nickname or administrator-role columns are created. The registration service
always hashes the password. The nullable hash leaves room for future accounts
without a password credential; no external-authentication route exists yet.
Email verification is never granted by this registration path.

## Roles

- Database owner: initial role provisioning only.
- Migration role: non-superuser, no database/role creation privileges; schema USAGE
  and CREATE. It owns the users table and goose history it creates.
- Application role: schema USAGE; INSERT only on `email,password_hash`; SELECT only
  on `id,email,created_at,updated_at,email_verified_at`. No ID assignment, verification
  timestamp assignment, hash reads, UPDATE, DELETE, DDL or migration-history access.

The migration role is separate from the application role. Migration credentials
are not passed to API/worker containers. The migrator requires an expected database
name and verifies `current_database()` before applying migrations.

## Local Development

New environments use `scripts/setup-dev-env.py` to generate `.env` with separate
random owner/application/migration passwords (0600). PostgreSQL initializers provision
the application and migration roles only when the data directory is first created.

For an existing initialized local database, add `MIGRATION_DB_USER` and
`MIGRATION_DB_PASSWORD` to the ignored `.env`, preserving its existing values.
After updating the PostgreSQL container configuration, provision the new role once:

```sh
docker compose -f compose.yaml -f compose.dev.yaml up -d --no-deps --wait postgres
docker compose -f compose.yaml -f compose.dev.yaml exec -T postgres sh -c 'psql -X -v ON_ERROR_STOP=1 -U "$POSTGRES_USER" -d "$POSTGRES_DB" -f /docker-entrypoint-initdb.d/02-migrator.sql'
```

The provisioning SQL creates a new role; do not rerun it if that role already exists.
Changing environment variables does not rotate existing PostgreSQL passwords.

Apply pending migrations explicitly, before starting an API version that needs them:

```sh
docker compose -f compose.yaml -f compose.dev.yaml -f compose.migrate.yaml run --rm --build migrate
docker compose -f compose.yaml -f compose.dev.yaml up -d --build --wait
```

`migrate` is a tools-profile one-off process, not a fifth permanent service.
The API and worker never apply migrations on startup. Existing data is not reset.

The proxy subnet, dynamic allocation range and fixed Caddy address are configurable
through `TRAMAS_PROXY_SUBNET`, `TRAMAS_PROXY_DYNAMIC_RANGE` and `TRAMAS_PROXY_IP`.
They must be consistent and must not overlap existing Docker/host networks. The fixed
address must remain outside the dynamic range. Changing network IPAM requires
recreating that proxy network, without removing PostgreSQL volumes.

## Isolated Tests

```sh
python3 scripts/setup-test-env.py
docker compose --env-file .env.test -f compose.test.yaml up -d --wait postgres_test
docker compose --env-file .env.test -f compose.test.yaml run --rm --build test
docker compose --env-file .env.test -f compose.test.yaml down
```

This Compose file is standalone; do not merge it with development or production
files. It uses a separate internal network, no published ports, and a tmpfs-backed
PostgreSQL database named `tramas_test`, with distinct app/migration roles and random
credentials in ignored `.env.test`. Stopping the test database discards its tmpfs data.
Development database volumes are never attached to it.

Integration tests assert the configured host/database/role and actual database
identity before migrations or inserts. They use unique fixture emails without
deleting existing rows. Without test configuration, host-side `go test` skips the
integration test; the standalone Compose run executes it against PostgreSQL.

Test code is kept separately from implementation code:

```text
backend/tests/
├── unit/
│   ├── auth/
│   └── migrations/
├── api/
│   └── auth/
└── integration/
```

Tests import production packages through their public interfaces. No test-only
exports are added to implementation packages.

The test command is `go test -race -count=1 ./...`. Tests cover input validation,
strict JSON, password hashing/salts, rate limits, hashing capacity, proxy trust,
success/duplicate/error responses, DB permissions, concurrent inserts/HTTP calls,
unverified email state, stable UUIDs and migration re-execution.

No production database was accessed. Production credentials, backup/recovery,
email verification and deployment procedures are not configured by these commands.
