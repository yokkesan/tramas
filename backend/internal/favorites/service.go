package favorites

import (
	"context"
	"errors"
	"time"
)

var ErrMissing = errors.New("favorite not found")
var ErrIneligible = errors.New("ineligible player")
var ErrLimit = errors.New("favorite limit reached")

type Player struct {
	ID     string  `json:"id"`
	Name   string  `json:"name"`
	Team   *string `json:"team"`
	Status string  `json:"status"`
}
type Favorite struct {
	ID           string     `json:"id"`
	Player       Player     `json:"player"`
	Main         bool       `json:"main"`
	RegisteredAt time.Time  `json:"registered_at"`
	EndedAt      *time.Time `json:"ended_at,omitempty"`
	EndReason    *string    `json:"end_reason,omitempty"`
}
type MainHistory struct {
	ID         string     `json:"id"`
	FavoriteID string     `json:"favorite_id"`
	PlayerID   string     `json:"player_id"`
	StartedAt  time.Time  `json:"started_at"`
	EndedAt    *time.Time `json:"ended_at"`
}
type Cursor struct {
	At time.Time `json:"at"`
	ID string    `json:"id"`
}
type Repository interface {
	Players(context.Context) ([]Player, error)
	List(context.Context, string, bool) ([]Favorite, error)
	Add(context.Context, string, string) error
	Remove(context.Context, string, string) error
	SetMain(context.Context, string, string) error
	History(context.Context, string, *Cursor) ([]Favorite, error)
	MainHistory(context.Context, string, *Cursor) ([]MainHistory, error)
}
type Service struct{ Repository Repository }

func (s Service) Add(ctx context.Context, user, player string) error {
	return s.Repository.Add(ctx, user, player)
}
func (s Service) Remove(ctx context.Context, user, player string) error {
	return s.Repository.Remove(ctx, user, player)
}
func (s Service) Main(ctx context.Context, user, player string) error {
	return s.Repository.SetMain(ctx, user, player)
}

// A future authoritative data adapter supplies stable source IDs and current
// membership. No mock snapshots are seeded in application startup or migrations.
type PlayerSource interface {
	Snapshot(context.Context) ([]PlayerRecord, error)
}
type PlayerRecord struct{ Source, SourceID, Name, Team, Status string }
