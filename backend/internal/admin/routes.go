package admin

import (
	"net/http"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/labstack/echo/v5"
	"tramas/backend/internal/platform"
	"tramas/backend/web/pages"
)

// Register is called only in development until authentication is implemented.
func Register(e *echo.Echo, pool *pgxpool.Pool) {
	e.GET("/admin", func(c *echo.Context) error {
		c.Response().Header().Set("Content-Type", "text/html; charset=utf-8")
		return pages.Admin().Render(c.Request().Context(), c.Response())
	})
	e.GET("/admin/status", func(c *echo.Context) error {
		healthy := platform.Ping(c.Request().Context(), pool) == nil
		c.Response().Header().Set("Content-Type", "text/html; charset=utf-8")
		if !healthy {
			c.Response().WriteHeader(http.StatusServiceUnavailable)
		}
		return pages.Status(healthy).Render(c.Request().Context(), c.Response())
	})
}
