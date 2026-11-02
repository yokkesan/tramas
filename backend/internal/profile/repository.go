package profile

import (
	"context"
	"errors"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type PostgresRepository struct{ Pool *pgxpool.Pool }

const profileSQL = `SELECT u.id::text,u.nickname,coalesce(p.biography,''),p.avatar_media_id::text,p.header_media_id::text,p.avatar_standard_id,p.header_standard_id,
 a.content_type,a.byte_size,h.content_type,h.byte_size FROM users u LEFT JOIN user_profiles p ON p.user_id=u.id
 LEFT JOIN profile_media a ON a.id=p.avatar_media_id LEFT JOIN profile_media h ON h.id=p.header_media_id WHERE u.id=$1::uuid`

func scanProfile(row pgx.Row) (Profile, error) {
	var p Profile
	var avatarID, headerID, ac, hc *string
	var az, hz *int64
	err := row.Scan(&p.ID, &p.Nickname, &p.Biography, &avatarID, &headerID, &p.AvatarStandard, &p.HeaderStandard, &ac, &az, &hc, &hz)
	if errors.Is(err, pgx.ErrNoRows) {
		return p, ErrNotFound
	}
	if err != nil {
		return p, err
	}
	if avatarID != nil && ac != nil && az != nil {
		p.Avatar = &Image{*avatarID, *ac, *az}
	}
	if headerID != nil && hc != nil && hz != nil {
		p.Header = &Image{*headerID, *hc, *hz}
	}
	return p, nil
}
func (r PostgresRepository) Get(ctx context.Context, id string, public bool) (Profile, error) {
	query := profileSQL
	if public {
		query += ` AND u.profile_completed_at IS NOT NULL`
	}
	return scanProfile(r.Pool.QueryRow(ctx, query, id))
}
func (r PostgresRepository) Update(ctx context.Context, id string, nickname, biography *string) (Profile, error) {
	tx, err := r.Pool.Begin(ctx)
	if err != nil {
		return Profile{}, err
	}
	defer tx.Rollback(ctx)
	var locked string
	if err = tx.QueryRow(ctx, `SELECT id::text FROM users WHERE id=$1::uuid FOR UPDATE`, id).Scan(&locked); err != nil {
		return Profile{}, err
	}
	if nickname != nil {
		if _, err = tx.Exec(ctx, `UPDATE users SET nickname=$2,profile_completed_at=coalesce(profile_completed_at,now()),updated_at=now() WHERE id=$1::uuid`, id, *nickname); err != nil {
			return Profile{}, err
		}
	}
	if biography != nil {
		if _, err = tx.Exec(ctx, `INSERT INTO user_profiles(user_id,biography) VALUES($1::uuid,$2) ON CONFLICT(user_id) DO UPDATE SET biography=excluded.biography`, id, *biography); err != nil {
			return Profile{}, err
		}
	}
	p, err := scanProfile(tx.QueryRow(ctx, profileSQL, id))
	if err != nil {
		return p, err
	}
	if err = tx.Commit(ctx); err != nil {
		return Profile{}, err
	}
	return p, nil
}
func (r PostgresRepository) SetImage(ctx context.Context, id, kind, key, contentType string, size int64) (Image, error) {
	if !validKind(kind) {
		return Image{}, ErrInput
	}
	tx, err := r.Pool.Begin(ctx)
	if err != nil {
		return Image{}, err
	}
	defer tx.Rollback(ctx)
	var locked string
	if err = tx.QueryRow(ctx, `SELECT id::text FROM users WHERE id=$1::uuid FOR UPDATE`, id).Scan(&locked); err != nil {
		return Image{}, err
	}
	if _, err = tx.Exec(ctx, `INSERT INTO user_profiles(user_id) VALUES($1::uuid) ON CONFLICT DO NOTHING`, id); err != nil {
		return Image{}, err
	}
	image := Image{ContentType: contentType, Size: size}
	if err = tx.QueryRow(ctx, `INSERT INTO profile_media(user_id,storage_key,content_type,byte_size) VALUES($1::uuid,$2,$3,$4) RETURNING id::text`, id, key, contentType, size).Scan(&image.ID); err != nil {
		return Image{}, err
	}
	// kind is selected from a closed server-side allowlist, never interpolated input.
	query := `UPDATE user_profiles SET avatar_media_id=$2::uuid,avatar_standard_id=NULL WHERE user_id=$1::uuid`
	if kind == "header" {
		query = `UPDATE user_profiles SET header_media_id=$2::uuid,header_standard_id=NULL WHERE user_id=$1::uuid`
	}
	if _, err = tx.Exec(ctx, query, id, image.ID); err != nil {
		return Image{}, err
	}
	if err = tx.Commit(ctx); err != nil {
		return Image{}, err
	}
	return image, nil
}
func (r PostgresRepository) SelectStandard(ctx context.Context, id, kind string, standard *string) error {
	if !validKind(kind) {
		return ErrInput
	}
	query := `INSERT INTO user_profiles(user_id,avatar_standard_id) VALUES($1::uuid,$2) ON CONFLICT(user_id) DO UPDATE SET avatar_media_id=NULL,avatar_standard_id=excluded.avatar_standard_id`
	if kind == "header" {
		query = `INSERT INTO user_profiles(user_id,header_standard_id) VALUES($1::uuid,$2) ON CONFLICT(user_id) DO UPDATE SET header_media_id=NULL,header_standard_id=excluded.header_standard_id`
	}
	tx, err := r.Pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	var locked string
	if err = tx.QueryRow(ctx, `SELECT id::text FROM users WHERE id=$1::uuid FOR UPDATE`, id).Scan(&locked); err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, query, id, standard); err != nil {
		return err
	}
	return tx.Commit(ctx)
}
func (r PostgresRepository) Media(ctx context.Context, mediaID, viewerID string, allowOthers bool) (string, string, error) {
	var key, contentType string
	// Only currently selected images are readable; replaced/deleted images are not.
	err := r.Pool.QueryRow(ctx, `SELECT m.storage_key,m.content_type FROM profile_media m JOIN user_profiles p ON p.user_id=m.user_id JOIN users u ON u.id=m.user_id
      WHERE m.id=$1::uuid AND (p.avatar_media_id=m.id OR p.header_media_id=m.id)
      AND (m.user_id=$2::uuid OR ($3 AND u.profile_completed_at IS NOT NULL))`, mediaID, viewerID, allowOthers).Scan(&key, &contentType)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", "", ErrNotFound
	}
	return key, contentType, err
}
func (r PostgresRepository) Unused(ctx context.Context, userID string) ([]StoredMedia, error) {
	rows, err := r.Pool.Query(ctx, `SELECT m.id::text,m.storage_key FROM profile_media m WHERE ($1='' OR m.user_id=nullif($1,'')::uuid)
      AND NOT EXISTS(SELECT 1 FROM user_profiles p WHERE p.avatar_media_id=m.id OR p.header_media_id=m.id) LIMIT 100`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []StoredMedia{}
	for rows.Next() {
		var v StoredMedia
		if err = rows.Scan(&v.ID, &v.Key); err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, rows.Err()
}
func (r PostgresRepository) DeleteUnused(ctx context.Context, id string) error {
	_, err := r.Pool.Exec(ctx, `DELETE FROM profile_media m WHERE id=$1::uuid AND NOT EXISTS(SELECT 1 FROM user_profiles p WHERE p.avatar_media_id=m.id OR p.header_media_id=m.id)`, id)
	return err
}
