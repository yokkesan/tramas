-- +goose Up
CREATE TABLE users (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    email text NOT NULL,
    password_hash text,
    email_verified_at timestamptz DEFAULT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT users_email_key UNIQUE (email),
    CONSTRAINT users_email_normalized CHECK (email = lower(btrim(email)) AND octet_length(email) BETWEEN 3 AND 254),
    CONSTRAINT users_password_hash_format CHECK (password_hash IS NULL OR password_hash LIKE '$argon2id$%')
);

-- Only the database may assign an account's ID. No UPDATE privilege is granted.
-- +goose StatementBegin
DO $$
BEGIN
    EXECUTE format('GRANT INSERT (email, password_hash) ON users TO %I', current_setting('tramas.app_role'));
    EXECUTE format('GRANT SELECT (id, email, created_at, updated_at, email_verified_at) ON users TO %I', current_setting('tramas.app_role'));
END $$;
-- +goose StatementEnd
