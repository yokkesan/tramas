-- +goose Up
-- Authoritative ingestion is not configured. No production sample players.
CREATE TABLE players (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    source text NOT NULL,
    source_id text NOT NULL,
    display_name text NOT NULL,
    team_code text,
    career_status text NOT NULL CHECK(career_status IN ('active','transferred','retired','departed')),
    updated_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE(source,source_id)
);
CREATE TABLE user_favorites (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id uuid NOT NULL REFERENCES users(id),
    player_id uuid NOT NULL REFERENCES players(id),
    registered_at timestamptz NOT NULL DEFAULT now(),
    ended_at timestamptz,
    end_reason text CHECK(end_reason IN ('removed','ineligible')),
    CHECK((ended_at IS NULL)=(end_reason IS NULL))
);
CREATE UNIQUE INDEX user_favorites_current ON user_favorites(user_id,player_id) WHERE ended_at IS NULL;
CREATE TABLE user_favorite_main_history (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    favorite_id uuid NOT NULL REFERENCES user_favorites(id),
    user_id uuid NOT NULL REFERENCES users(id),
    started_at timestamptz NOT NULL DEFAULT now(),
    ended_at timestamptz
);
CREATE UNIQUE INDEX user_favorite_one_main ON user_favorite_main_history(user_id) WHERE ended_at IS NULL;

-- Enforce max three / eligibility even for competing direct writes. All mutations
-- serialize per user. Player status changes close registrations and main history.
-- +goose StatementBegin
CREATE FUNCTION lock_player_membership(target uuid) RETURNS TABLE(eligible boolean)
LANGUAGE sql SECURITY DEFINER SET search_path=pg_catalog,public AS $$
    SELECT coalesce(career_status='active' AND team_code='hanshin_tigers',false)
    FROM public.players WHERE id=target FOR SHARE;
$$;
REVOKE ALL ON FUNCTION lock_player_membership(uuid) FROM PUBLIC;
CREATE FUNCTION enforce_favorite() RETURNS trigger LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog,public AS $$
BEGIN
    IF NEW.ended_at IS NULL THEN
        PERFORM 1 FROM public.users WHERE id=NEW.user_id FOR UPDATE;
        PERFORM 1 FROM public.players WHERE id=NEW.player_id AND career_status='active' AND team_code='hanshin_tigers' FOR SHARE;
        IF NOT FOUND THEN RAISE EXCEPTION 'ineligible player' USING ERRCODE='23514'; END IF;
        IF (SELECT count(*) FROM public.user_favorites WHERE user_id=NEW.user_id AND ended_at IS NULL AND id<>NEW.id)>=3 THEN
            RAISE EXCEPTION 'favorite limit' USING ERRCODE='23514';
        END IF;
    END IF;
    RETURN NEW;
END $$;
CREATE FUNCTION enforce_main_favorite() RETURNS trigger LANGUAGE plpgsql SET search_path=pg_catalog,public AS $$
BEGIN
    IF NEW.ended_at IS NULL THEN
        PERFORM 1 FROM public.users WHERE id=NEW.user_id FOR UPDATE;
        PERFORM 1 FROM public.user_favorites WHERE id=NEW.favorite_id AND user_id=NEW.user_id AND ended_at IS NULL;
        IF NOT FOUND THEN RAISE EXCEPTION 'inactive favorite' USING ERRCODE='23514'; END IF;
    END IF;
    RETURN NEW;
END $$;
CREATE FUNCTION retire_favorites() RETURNS trigger LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog,public AS $$
BEGIN
    IF NEW.career_status<>'active' OR NEW.team_code IS DISTINCT FROM 'hanshin_tigers' THEN
        UPDATE public.user_favorite_main_history SET ended_at=now() WHERE ended_at IS NULL AND favorite_id IN
          (SELECT id FROM public.user_favorites WHERE player_id=NEW.id AND ended_at IS NULL);
        UPDATE public.user_favorites SET ended_at=now(),end_reason='ineligible' WHERE player_id=NEW.id AND ended_at IS NULL;
    END IF;
    RETURN NEW;
END $$;
CREATE TRIGGER favorite_guard BEFORE INSERT OR UPDATE ON user_favorites FOR EACH ROW EXECUTE FUNCTION enforce_favorite();
CREATE TRIGGER favorite_main_guard BEFORE INSERT OR UPDATE ON user_favorite_main_history FOR EACH ROW EXECUTE FUNCTION enforce_main_favorite();
CREATE TRIGGER player_retirement AFTER UPDATE OF career_status,team_code ON players FOR EACH ROW EXECUTE FUNCTION retire_favorites();
DO $$
BEGIN
    EXECUTE format('GRANT SELECT ON players TO %I',current_setting('tramas.app_role'));
    EXECUTE format('GRANT EXECUTE ON FUNCTION lock_player_membership(uuid) TO %I',current_setting('tramas.app_role'));
    EXECUTE format('GRANT SELECT,INSERT,UPDATE ON user_favorites,user_favorite_main_history TO %I',current_setting('tramas.app_role'));
END $$;
-- +goose StatementEnd
