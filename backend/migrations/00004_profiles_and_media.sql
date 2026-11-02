-- +goose Up
CREATE TABLE profile_media (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id uuid NOT NULL REFERENCES users(id),
    storage_key text NOT NULL UNIQUE CHECK(storage_key ~ '^[a-f0-9]{64}$'),
    content_type text NOT NULL CHECK(content_type IN ('image/jpeg','image/png','image/heic')),
    byte_size bigint NOT NULL CHECK(byte_size BETWEEN 1 AND 10485760),
    created_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE(id,user_id)
);
CREATE TABLE user_profiles (
    user_id uuid PRIMARY KEY REFERENCES users(id),
    biography text NOT NULL DEFAULT '',
    avatar_media_id uuid,
    header_media_id uuid,
    avatar_standard_id text,
    header_standard_id text,
    CHECK(avatar_media_id IS NULL OR avatar_standard_id IS NULL),
    CHECK(header_media_id IS NULL OR header_standard_id IS NULL),
    FOREIGN KEY(avatar_media_id,user_id) REFERENCES profile_media(id,user_id),
    FOREIGN KEY(header_media_id,user_id) REFERENCES profile_media(id,user_id)
);
-- +goose StatementBegin
DO $$
BEGIN
    EXECUTE format('GRANT SELECT,INSERT,UPDATE,DELETE ON user_profiles,profile_media TO %I',current_setting('tramas.app_role'));
END $$;
-- +goose StatementEnd
