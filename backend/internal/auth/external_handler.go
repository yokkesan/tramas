package auth

import (
	"github.com/labstack/echo/v5"
)

func RegisterExternal(e *echo.Echo, h *SessionHandler, s ExternalService) {
	e.POST("/api/v1/auth/reauthenticate", func(c *echo.Context) error {
		values, err := readFields(c, "password")
		if err != nil {
			return authFailure(c, err)
		}
		account, _ := CurrentAccount(c)
		proof, err := s.Reauthenticate(c.Request().Context(), account, values["password"])
		if err != nil {
			return authFailure(c, err)
		}
		return c.JSON(200, map[string]any{"reauth_token": proof, "expires_in": 300})
	}, h.Guard, h.RequireAuth())
	e.POST("/api/v1/auth/oidc/:provider/challenge", func(c *echo.Context) error {
		values, err := readFields(c, "purpose", "reauth_token")
		if err != nil {
			return authFailure(c, err)
		}
		userID := ""
		if values["purpose"] != "login" {
			if err = h.RequireAuth()(func(c *echo.Context) error { a, _ := CurrentAccount(c); userID = a.ID; return nil })(c); err != nil {
				return err
			}
			if userID == "" {
				return nil
			} // RequireAuth already wrote its error response.
		}
		v, err := s.Challenge(c.Request().Context(), c.Param("provider"), values["purpose"], userID, values["reauth_token"])
		if err != nil {
			return authFailure(c, err)
		}
		return c.JSON(200, v)
	}, h.Guard)
	for _, purpose := range []string{"login", "link", "reauth"} {
		handler := func(c *echo.Context) error {
			values, err := readFields(c, "challenge_token", "id_token")
			if err != nil {
				return authFailure(c, err)
			}
			account, _ := CurrentAccount(c)
			a, tokens, proof, err := s.Complete(c.Request().Context(), c.Param("provider"), purpose, account.ID, values["challenge_token"], values["id_token"])
			if err != nil {
				return authFailure(c, err)
			}
			switch purpose {
			case "link":
				return c.NoContent(204)
			case "reauth":
				return c.JSON(200, map[string]any{"reauth_token": proof, "expires_in": 300})
			default:
				return c.JSON(200, map[string]any{"user": a, "tokens": tokens})
			}
		}
		middleware := []echo.MiddlewareFunc{h.Guard}
		if purpose != "login" {
			middleware = append(middleware, h.RequireAuth())
		}
		e.POST("/api/v1/auth/oidc/:provider/"+purpose, handler, middleware...)
	}
}
