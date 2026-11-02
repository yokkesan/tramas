package profile

import (
	"errors"
	"github.com/labstack/echo/v5"
	"io"
	"log/slog"
	"mime"
	"net/http"
	"regexp"
	"tramas/backend/internal/auth"
)

var uuidPattern = regexp.MustCompile(`^[a-fA-F0-9]{8}-[a-fA-F0-9]{4}-[a-fA-F0-9]{4}-[a-fA-F0-9]{4}-[a-fA-F0-9]{12}$`)

func ValidID(v string) bool { return uuidPattern.MatchString(v) }

type Handler struct{ Service Service }

func profileError(c *echo.Context, err error) error {
	switch {
	case errors.Is(err, ErrNotFound):
		return auth.ErrorResponse(c, 404, "not_found")
	case errors.Is(err, ErrInput):
		return auth.ErrorResponse(c, 422, "invalid_profile")
	case errors.Is(err, ErrImage):
		return auth.ErrorResponse(c, 422, "invalid_image")
	case errors.Is(err, ErrTooLarge):
		return auth.ErrorResponse(c, 413, "image_too_large")
	case errors.Is(err, ErrStorage):
		return auth.ErrorResponse(c, 503, "storage_unavailable")
	case errors.Is(err, ErrImageBusy):
		c.Response().Header().Set("Retry-After", "1")
		return auth.ErrorResponse(c, 503, "image_busy")
	default:
		return auth.WriteFailure(c, err)
	}
}
func Register(e *echo.Echo, authHandler *auth.SessionHandler, s Service) {
	h := Handler{s}
	self := []echo.MiddlewareFunc{authHandler.Guard, authHandler.RequireAuth()}
	ordinary := []echo.MiddlewareFunc{authHandler.Guard, authHandler.RequireAuth(), auth.RequireProfile}
	e.GET("/api/v1/users/me/profile", h.Self, self...)
	e.PUT("/api/v1/users/me/profile", h.Update, self...)
	e.GET("/api/v1/users/:user_id/profile", h.Public, ordinary...)
	e.PUT("/api/v1/users/me/profile/images/:kind", h.Upload, self...)
	e.DELETE("/api/v1/users/me/profile/images/:kind", h.Remove, self...)
	e.PUT("/api/v1/users/me/profile/images/:kind/standard", h.Standard, self...)
	e.GET("/api/v1/profile/standard-images", func(c *echo.Context) error {
		images := s.Standards
		if images == nil {
			images = []StandardImage{}
		}
		return c.JSON(200, map[string]any{"images": images})
	}, self...)
	e.GET("/api/v1/profile/images/:image_id", h.Image, self...)
}
func (h Handler) Self(c *echo.Context) error {
	a, _ := auth.CurrentAccount(c)
	p, err := h.Service.Repository.Get(c.Request().Context(), a.ID, false)
	if err != nil {
		return profileError(c, err)
	}
	return c.JSON(200, map[string]any{"profile": p})
}
func (h Handler) Public(c *echo.Context) error {
	id := c.Param("user_id")
	if !ValidID(id) {
		return auth.ErrorResponse(c, 404, "not_found")
	}
	p, err := h.Service.Repository.Get(c.Request().Context(), id, true)
	if err != nil {
		return profileError(c, err)
	}
	return c.JSON(200, map[string]any{"profile": p})
}
func (h Handler) Update(c *echo.Context) error {
	values, err := auth.ReadStringFields(c, 1<<20, "nickname", "biography")
	if err != nil {
		return auth.WriteFailure(c, err)
	}
	a, _ := auth.CurrentAccount(c)
	p, err := h.Service.Update(c.Request().Context(), a.ID, values)
	if err != nil {
		return profileError(c, err)
	}
	slog.Info("profile updated", "category", "audit", "user_id", a.ID)
	return c.JSON(200, map[string]any{"profile": p})
}
func (h Handler) Upload(c *echo.Context) error {
	mediaType, _, err := mime.ParseMediaType(c.Request().Header.Get("Content-Type"))
	if err != nil || mediaType != "image/jpeg" && mediaType != "image/png" && mediaType != "image/heic" {
		return auth.ErrorResponse(c, 415, "unsupported_media_type")
	}
	if c.Request().ContentLength > MaxImageBytes {
		return auth.ErrorResponse(c, 413, "image_too_large")
	}
	a, _ := auth.CurrentAccount(c)
	image, err := h.Service.Upload(c.Request().Context(), a.ID, c.Param("kind"), mediaType, c.Request().Body)
	if err != nil {
		return profileError(c, err)
	}
	slog.Info("profile image updated", "category", "audit", "user_id", a.ID)
	return c.JSON(200, map[string]any{"image": image})
}
func (h Handler) Remove(c *echo.Context) error {
	a, _ := auth.CurrentAccount(c)
	if err := h.Service.RemoveImage(c.Request().Context(), a.ID, c.Param("kind")); err != nil {
		return profileError(c, err)
	}
	slog.Info("profile image removed", "category", "audit", "user_id", a.ID)
	return c.NoContent(204)
}
func (h Handler) Standard(c *echo.Context) error {
	v, err := auth.ReadStringFields(c, 8<<10, "image_id")
	if err != nil {
		return auth.WriteFailure(c, err)
	}
	a, _ := auth.CurrentAccount(c)
	if err = h.Service.SelectStandard(c.Request().Context(), a.ID, c.Param("kind"), v["image_id"]); err != nil {
		return profileError(c, err)
	}
	return c.NoContent(204)
}
func (h Handler) Image(c *echo.Context) error {
	id := c.Param("image_id")
	if !ValidID(id) {
		return auth.ErrorResponse(c, 404, "not_found")
	}
	if h.Service.Storage == nil {
		return profileError(c, ErrStorage)
	}
	a, _ := auth.CurrentAccount(c)
	key, contentType, err := h.Service.Repository.Media(c.Request().Context(), id, a.ID, a.ProfileCompleted)
	if err != nil {
		return profileError(c, err)
	}
	r, err := h.Service.Storage.Open(c.Request().Context(), key)
	if err != nil {
		return profileError(c, ErrNotFound)
	}
	defer r.Close()
	c.Response().Header().Set("X-Content-Type-Options", "nosniff")
	c.Response().Header().Set("Content-Type", contentType)
	c.Response().Header().Set("Content-Disposition", `inline; filename="profile-image"`)
	if err = c.NoContent(http.StatusOK); err != nil {
		return err
	}
	_, err = io.Copy(c.Response(), r)
	return err
}
