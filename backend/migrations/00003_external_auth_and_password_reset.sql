-- +goose Up
-- External-only accounts have no local email/password credential. Provider email
-- is metadata on the identity and never an automatic account-link key.
ALTER TABLE users ALTER COLUMN email DROP NOT NULL;
CREATE TABLE auth_identities (
    provider text NOT NULL CHECK(provider IN ('apple','google')),
    subject text NOT NULL CHECK(octet_length(subject) BETWEEN 1 AND 1024),
    user_id uuid NOT NULL REFERENCES users(id),
    provider_email text,
    created_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY(provider,subject),
    UNIQUE(user_id,provider)
);
CREATE TABLE auth_actions (
    token_hash bytea PRIMARY KEY CHECK(octet_length(token_hash)=32),
    purpose text NOT NULL CHECK(purpose IN ('oidc_login','oidc_link','oidc_reauth','reauth_proof')),
    provider text CHECK(provider IN ('apple','google')),
    user_id uuid REFERENCES users(id),
    nonce_hash bytea CHECK(octet_length(nonce_hash)=32),
    expires_at timestamptz NOT NULL,
    consumed_at timestamptz
);
CREATE INDEX auth_actions_expiry ON auth_actions(expires_at);
CREATE TABLE auth_password_resets (
    user_id uuid PRIMARY KEY REFERENCES users(id),
    token_hash bytea UNIQUE CHECK(octet_length(token_hash)=32),
    expires_at timestamptz,
    consumed_at timestamptz,
    pending boolean NOT NULL DEFAULT true,
    attempts integer NOT NULL DEFAULT 0,
    next_attempt_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX auth_reset_pending ON auth_password_resets(next_attempt_at) WHERE pending;
CREATE TABLE auth_audit_events (
    id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    user_id uuid REFERENCES users(id),
    event text NOT NULL CHECK(event IN ('password_reset_requested','password_reset_completed','password_reset_rejected','identity_linked')),
    created_at timestamptz NOT NULL DEFAULT now()
);

-- Password updates remain denied to the app role. Only a live one-use reset token
-- can invoke the scoped update, session revocation and audit transaction.
-- +goose StatementBegin
CREATE FUNCTION complete_password_reset(proof bytea, new_hash text) RETURNS text
LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog,public AS $$
DECLARE target uuid;
BEGIN
    IF new_hash NOT LIKE '$argon2id$%' THEN RETURN NULL; END IF;
    SELECT user_id INTO target FROM public.auth_password_resets WHERE token_hash=proof;
    IF target IS NULL THEN RETURN NULL; END IF;
    PERFORM 1 FROM public.users WHERE id=target FOR UPDATE;
    UPDATE public.auth_password_resets SET consumed_at=now(),pending=false
      WHERE user_id=target AND token_hash=proof AND consumed_at IS NULL AND expires_at>now();
    IF NOT FOUND THEN RETURN NULL; END IF;
    UPDATE public.users SET password_hash=new_hash,updated_at=now() WHERE id=target AND password_hash IS NOT NULL;
    IF NOT FOUND THEN RAISE EXCEPTION 'reset credential unavailable'; END IF;
    UPDATE public.auth_sessions SET revoked_at=coalesce(revoked_at,now()) WHERE user_id=target;
    UPDATE public.auth_actions SET consumed_at=coalesce(consumed_at,now()) WHERE user_id=target;
    INSERT INTO public.auth_audit_events(user_id,event) VALUES(target,'password_reset_completed');
    RETURN target::text;
END $$;
-- +goose StatementEnd
REVOKE ALL ON FUNCTION complete_password_reset(bytea,text) FROM PUBLIC;
-- +goose StatementBegin
DO $$
BEGIN
    EXECUTE format('GRANT SELECT,INSERT ON auth_identities TO %I',current_setting('tramas.app_role'));
    EXECUTE format('GRANT SELECT,INSERT,UPDATE,DELETE ON auth_actions,auth_password_resets TO %I',current_setting('tramas.app_role'));
    EXECUTE format('GRANT INSERT(user_id,event) ON auth_audit_events TO %I',current_setting('tramas.app_role'));
    EXECUTE format('GRANT EXECUTE ON FUNCTION complete_password_reset(bytea,text) TO %I',current_setting('tramas.app_role'));
END $$;
-- +goose StatementEnd
