package favorites

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"github.com/labstack/echo/v5"
	"log/slog"
	"tramas/backend/internal/auth"
	"tramas/backend/internal/profile"
)

func favoriteError(c *echo.Context, err error) error {
	switch {
	case errors.Is(err, ErrMissing):
		return auth.ErrorResponse(c, 404, "not_found")
	case errors.Is(err, ErrLimit):
		return auth.ErrorResponse(c, 409, "favorite_limit")
	case errors.Is(err, ErrIneligible):
		return auth.ErrorResponse(c, 422, "ineligible_player")
	default:
		return auth.WriteFailure(c, err)
	}
}
func Register(e *echo.Echo, h *auth.SessionHandler, s Service) {
	m := []echo.MiddlewareFunc{h.Guard, h.RequireAuth(), auth.RequireProfile}
	e.GET("/api/v1/players", func(c *echo.Context) error {
		rows, err := s.Repository.Players(c.Request().Context())
		if err != nil {
			return favoriteError(c, err)
		}
		return c.JSON(200, map[string]any{"players": rows})
	}, m...)
	e.GET("/api/v1/users/me/favorites", func(c *echo.Context) error {
		a, _ := auth.CurrentAccount(c)
		rows, err := s.Repository.List(c.Request().Context(), a.ID, false)
		if err != nil {
			return favoriteError(c, err)
		}
		return c.JSON(200, map[string]any{"favorites": rows})
	}, m...)
	e.GET("/api/v1/users/:user_id/favorites", func(c *echo.Context) error {
		id := c.Param("user_id")
		if !profile.ValidID(id) {
			return auth.ErrorResponse(c, 404, "not_found")
		}
		rows, err := s.Repository.List(c.Request().Context(), id, true)
		if err != nil {
			return favoriteError(c, err)
		}
		return c.JSON(200, map[string]any{"favorites": rows})
	}, m...)
	e.POST("/api/v1/users/me/favorites", func(c *echo.Context) error {
		v, err := auth.ReadStringFields(c, 8<<10, "player_id")
		if err != nil {
			return auth.WriteFailure(c, err)
		}
		if !profile.ValidID(v["player_id"]) {
			return auth.ErrorResponse(c, 422, "invalid_player")
		}
		a, _ := auth.CurrentAccount(c)
		if err = s.Add(c.Request().Context(), a.ID, v["player_id"]); err != nil {
			return favoriteError(c, err)
		}
		slog.Info("favorite added", "category", "audit", "user_id", a.ID)
		return c.NoContent(204)
	}, m...)
	e.DELETE("/api/v1/users/me/favorites/:player_id", func(c *echo.Context) error {
		id := c.Param("player_id")
		if !profile.ValidID(id) {
			return auth.ErrorResponse(c, 422, "invalid_player")
		}
		a, _ := auth.CurrentAccount(c)
		if err := s.Remove(c.Request().Context(), a.ID, id); err != nil {
			return favoriteError(c, err)
		}
		slog.Info("favorite removed", "category", "audit", "user_id", a.ID)
		return c.NoContent(204)
	}, m...)
	e.PUT("/api/v1/users/me/favorites/:player_id/main", func(c *echo.Context) error {
		id := c.Param("player_id")
		if !profile.ValidID(id) {
			return auth.ErrorResponse(c, 422, "invalid_player")
		}
		a, _ := auth.CurrentAccount(c)
		if err := s.Main(c.Request().Context(), a.ID, id); err != nil {
			return favoriteError(c, err)
		}
		slog.Info("main favorite changed", "category", "audit", "user_id", a.ID)
		return c.NoContent(204)
	}, m...)
	e.DELETE("/api/v1/users/me/favorites/main", func(c *echo.Context) error {
		a, _ := auth.CurrentAccount(c)
		if err := s.Main(c.Request().Context(), a.ID, ""); err != nil {
			return favoriteError(c, err)
		}
		return c.NoContent(204)
	}, m...)
	e.GET("/api/v1/users/me/favorites/history", func(c *echo.Context) error {
		cursor, err := decodeCursor(c.QueryParam("cursor"))
		if err != nil {
			return auth.ErrorResponse(c, 400, "invalid_cursor")
		}
		a, _ := auth.CurrentAccount(c)
		rows, err := s.Repository.History(c.Request().Context(), a.ID, cursor)
		if err != nil {
			return favoriteError(c, err)
		}
		next := ""
		if len(rows) > 50 {
			rows = rows[:50]
			last := rows[len(rows)-1]
			next = encodeCursor(Cursor{last.RegisteredAt, last.ID})
		}
		return c.JSON(200, map[string]any{"history": rows, "next_cursor": next})
	}, m...)
	e.GET("/api/v1/users/me/favorites/main-history", func(c *echo.Context) error {
		cursor, err := decodeCursor(c.QueryParam("cursor"))
		if err != nil {
			return auth.ErrorResponse(c, 400, "invalid_cursor")
		}
		a, _ := auth.CurrentAccount(c)
		rows, err := s.Repository.MainHistory(c.Request().Context(), a.ID, cursor)
		if err != nil {
			return favoriteError(c, err)
		}
		next := ""
		if len(rows) > 50 {
			rows = rows[:50]
			last := rows[len(rows)-1]
			next = encodeCursor(Cursor{last.StartedAt, last.ID})
		}
		return c.JSON(200, map[string]any{"history": rows, "next_cursor": next})
	}, m...)
}
func encodeCursor(c Cursor) string {
	b, _ := json.Marshal(c)
	return base64.RawURLEncoding.EncodeToString(b)
}
func decodeCursor(s string) (*Cursor, error) {
	if s == "" {
		return nil, nil
	}
	if len(s) > 512 {
		return nil, ErrMissing
	}
	b, err := base64.RawURLEncoding.Strict().DecodeString(s)
	if err != nil {
		return nil, err
	}
	var c Cursor
	if err = json.Unmarshal(b, &c); err != nil {
		return nil, err
	}
	if !profile.ValidID(c.ID) || c.At.IsZero() {
		return nil, ErrMissing
	}
	return &c, nil
}
