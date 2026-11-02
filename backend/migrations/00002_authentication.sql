-- +goose Up
ALTER TABLE users ADD COLUMN nickname text;
ALTER TABLE users ADD COLUMN profile_completed_at timestamptz;
ALTER TABLE users ADD CONSTRAINT users_profile CHECK (
    (nickname IS NULL AND profile_completed_at IS NULL) OR
    (nickname IS NOT NULL AND char_length(nickname) BETWEEN 2 AND 20 AND profile_completed_at IS NOT NULL)
);

-- One row per account combines verification state and a durable delivery job.
CREATE TABLE auth_email_verifications (
    user_id uuid PRIMARY KEY REFERENCES users(id),
    token_hash bytea UNIQUE CHECK (octet_length(token_hash) = 32),
    expires_at timestamptz,
    consumed_at timestamptz,
    pending boolean NOT NULL DEFAULT true,
    attempts integer NOT NULL DEFAULT 0 CHECK (attempts >= 0),
    next_attempt_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX auth_email_pending ON auth_email_verifications(next_attempt_at) WHERE pending;
INSERT INTO auth_email_verifications(user_id) SELECT id FROM users WHERE email_verified_at IS NULL;

CREATE TABLE auth_sessions (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id uuid NOT NULL REFERENCES users(id),
    access_hash bytea NOT NULL UNIQUE CHECK (octet_length(access_hash) = 32),
    refresh_hash bytea NOT NULL UNIQUE CHECK (octet_length(refresh_hash) = 32),
    previous_refresh_hash bytea UNIQUE CHECK (octet_length(previous_refresh_hash) = 32),
    refresh_request_hash bytea CHECK (octet_length(refresh_request_hash) = 32),
    retry_until timestamptz,
    access_expires_at timestamptz NOT NULL,
    refresh_expires_at timestamptz NOT NULL,
    absolute_expires_at timestamptz NOT NULL,
    revoked_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX auth_sessions_user ON auth_sessions(user_id);

CREATE TABLE auth_rate_limits (
    key bytea PRIMARY KEY CHECK (octet_length(key) = 32),
    hits integer NOT NULL,
    expires_at timestamptz NOT NULL
);
CREATE INDEX auth_rate_expiry ON auth_rate_limits(expires_at);

-- +goose StatementBegin
DO $$
BEGIN
    EXECUTE format('GRANT SELECT (password_hash,nickname,profile_completed_at) ON users TO %I', current_setting('tramas.app_role'));
    EXECUTE format('GRANT UPDATE (email_verified_at,nickname,profile_completed_at,updated_at) ON users TO %I', current_setting('tramas.app_role'));
    EXECUTE format('GRANT SELECT,INSERT,UPDATE,DELETE ON auth_email_verifications,auth_sessions,auth_rate_limits TO %I', current_setting('tramas.app_role'));
END $$;
-- +goose StatementEnd
