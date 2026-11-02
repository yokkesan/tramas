package favorites

import (
	"context"
	"errors"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type PostgresRepository struct{ Pool *pgxpool.Pool }

func (r PostgresRepository) Players(ctx context.Context) ([]Player, error) {
	rows, err := r.Pool.Query(ctx, `SELECT id::text,display_name,team_code,career_status FROM players WHERE career_status='active' AND team_code='hanshin_tigers' ORDER BY display_name,id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Player{}
	for rows.Next() {
		var p Player
		if err = rows.Scan(&p.ID, &p.Name, &p.Team, &p.Status); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

const favoriteColumns = `f.id::text,p.id::text,p.display_name,p.team_code,p.career_status,
 EXISTS(SELECT 1 FROM user_favorite_main_history m WHERE m.favorite_id=f.id AND m.ended_at IS NULL),f.registered_at,f.ended_at,f.end_reason`

func scanFavorites(rows pgx.Rows) ([]Favorite, error) {
	defer rows.Close()
	out := []Favorite{}
	for rows.Next() {
		var f Favorite
		if err := rows.Scan(&f.ID, &f.Player.ID, &f.Player.Name, &f.Player.Team, &f.Player.Status, &f.Main, &f.RegisteredAt, &f.EndedAt, &f.EndReason); err != nil {
			return nil, err
		}
		out = append(out, f)
	}
	return out, rows.Err()
}
func (r PostgresRepository) List(ctx context.Context, user string, public bool) ([]Favorite, error) {
	if public {
		var exists bool
		if err := r.Pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM users WHERE id=$1::uuid AND profile_completed_at IS NOT NULL)`, user).Scan(&exists); err != nil {
			return nil, err
		}
		if !exists {
			return nil, ErrMissing
		}
	}
	rows, err := r.Pool.Query(ctx, `SELECT `+favoriteColumns+` FROM user_favorites f JOIN players p ON p.id=f.player_id
      WHERE f.user_id=$1::uuid AND f.ended_at IS NULL AND p.career_status='active' AND p.team_code='hanshin_tigers' ORDER BY f.registered_at,f.id`, user)
	if err != nil {
		return nil, err
	}
	return scanFavorites(rows)
}
func lockUser(ctx context.Context, tx pgx.Tx, user string) error {
	var id string
	return tx.QueryRow(ctx, `SELECT id::text FROM users WHERE id=$1::uuid FOR UPDATE`, user).Scan(&id)
}
func (r PostgresRepository) Add(ctx context.Context, user, player string) error {
	tx, err := r.Pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	var eligible bool
	err = tx.QueryRow(ctx, `SELECT eligible FROM lock_player_membership($1::uuid)`, player).Scan(&eligible)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrMissing
	}
	if err != nil {
		return err
	}
	if !eligible {
		return ErrIneligible
	}
	if err = lockUser(ctx, tx, user); err != nil {
		return err
	}
	var exists bool
	var count int
	if err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM user_favorites WHERE user_id=$1::uuid AND player_id=$2::uuid AND ended_at IS NULL),
      (SELECT count(*) FROM user_favorites WHERE user_id=$1::uuid AND ended_at IS NULL)`, user, player).Scan(&exists, &count); err != nil {
		return err
	}
	if exists {
		return tx.Commit(ctx)
	}
	if count >= 3 {
		return ErrLimit
	}
	if _, err = tx.Exec(ctx, `INSERT INTO user_favorites(user_id,player_id) VALUES($1::uuid,$2::uuid)`, user, player); err != nil {
		return err
	}
	return tx.Commit(ctx)
}
func (r PostgresRepository) Remove(ctx context.Context, user, player string) error {
	tx, err := r.Pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if err = lockUser(ctx, tx, user); err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, `UPDATE user_favorite_main_history SET ended_at=now() WHERE user_id=$1::uuid AND ended_at IS NULL
      AND favorite_id IN (SELECT id FROM user_favorites WHERE user_id=$1::uuid AND player_id=$2::uuid AND ended_at IS NULL)`, user, player); err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, `UPDATE user_favorites SET ended_at=now(),end_reason='removed' WHERE user_id=$1::uuid AND player_id=$2::uuid AND ended_at IS NULL`, user, player); err != nil {
		return err
	}
	return tx.Commit(ctx)
}
func (r PostgresRepository) SetMain(ctx context.Context, user, player string) error {
	tx, err := r.Pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	// Player-before-user lock order prevents a cycle with membership updates.
	if player != "" {
		var eligible bool
		err = tx.QueryRow(ctx, `SELECT eligible FROM lock_player_membership($1::uuid)`, player).Scan(&eligible)
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrMissing
		}
		if err != nil {
			return err
		}
		if !eligible {
			return ErrIneligible
		}
	}
	if err = lockUser(ctx, tx, user); err != nil {
		return err
	}
	var favorite string
	if player != "" {
		err = tx.QueryRow(ctx, `SELECT id::text FROM user_favorites WHERE user_id=$1::uuid AND player_id=$2::uuid AND ended_at IS NULL`, user, player).Scan(&favorite)
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrMissing
		}
		if err != nil {
			return err
		}
	}
	var same bool
	if err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM user_favorite_main_history WHERE user_id=$1::uuid AND favorite_id=nullif($2,'')::uuid AND ended_at IS NULL)`, user, favorite).Scan(&same); err != nil {
		return err
	}
	if same {
		return tx.Commit(ctx)
	}
	if _, err = tx.Exec(ctx, `UPDATE user_favorite_main_history SET ended_at=now() WHERE user_id=$1::uuid AND ended_at IS NULL`, user); err != nil {
		return err
	}
	if favorite != "" {
		if _, err = tx.Exec(ctx, `INSERT INTO user_favorite_main_history(favorite_id,user_id) VALUES($1::uuid,$2::uuid)`, favorite, user); err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}
func (r PostgresRepository) History(ctx context.Context, user string, cursor *Cursor) ([]Favorite, error) {
	var at any
	var id any
	if cursor != nil {
		at = cursor.At
		id = cursor.ID
	}
	rows, err := r.Pool.Query(ctx, `SELECT `+favoriteColumns+` FROM user_favorites f JOIN players p ON p.id=f.player_id WHERE f.user_id=$1::uuid
      AND ($2::timestamptz IS NULL OR (f.registered_at,f.id)<($2::timestamptz,$3::uuid)) ORDER BY f.registered_at DESC,f.id DESC LIMIT 51`, user, at, id)
	if err != nil {
		return nil, err
	}
	return scanFavorites(rows)
}
func (r PostgresRepository) MainHistory(ctx context.Context, user string, cursor *Cursor) ([]MainHistory, error) {
	var at any
	var id any
	if cursor != nil {
		at = cursor.At
		id = cursor.ID
	}
	rows, err := r.Pool.Query(ctx, `SELECT m.id::text,m.favorite_id::text,f.player_id::text,m.started_at,m.ended_at FROM user_favorite_main_history m JOIN user_favorites f ON f.id=m.favorite_id
      WHERE m.user_id=$1::uuid AND ($2::timestamptz IS NULL OR (m.started_at,m.id)<($2::timestamptz,$3::uuid)) ORDER BY m.started_at DESC,m.id DESC LIMIT 51`, user, at, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []MainHistory{}
	for rows.Next() {
		var m MainHistory
		if err = rows.Scan(&m.ID, &m.FavoriteID, &m.PlayerID, &m.StartedAt, &m.EndedAt); err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

// For a separately authorized ingestion process, never the normal app DB role.
func (r PostgresRepository) UpsertPlayer(ctx context.Context, p PlayerRecord) error {
	if p.Source == "" || p.SourceID == "" || p.Name == "" {
		return ErrIneligible
	}
	if p.Status != "active" && p.Status != "transferred" && p.Status != "retired" && p.Status != "departed" {
		return ErrIneligible
	}
	_, err := r.Pool.Exec(ctx, `INSERT INTO players(source,source_id,display_name,team_code,career_status) VALUES($1,$2,$3,nullif($4,''),$5)
       ON CONFLICT(source,source_id) DO UPDATE SET display_name=excluded.display_name,team_code=excluded.team_code,career_status=excluded.career_status,updated_at=now()`, p.Source, p.SourceID, p.Name, p.Team, p.Status)
	return err
}
