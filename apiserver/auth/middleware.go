package auth

import (
	"net/http"
	"strings"

	"github.com/labstack/echo/v5"
)

func Middleware(introspector Introspector) echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c *echo.Context) error {
			header := c.Request().Header.Get("Authorization")
			if !strings.HasPrefix(header, "Bearer ") {
				return echo.NewHTTPError(http.StatusUnauthorized, "missing bearer token")
			}

			token := strings.TrimPrefix(header, "Bearer ")
			principal, err := introspector.Introspect(c.Request().Context(), token)
			if err != nil {
				return echo.NewHTTPError(http.StatusUnauthorized, "invalid token")
			}

			ctx := WithPrincipal(c.Request().Context(), principal)
			c.SetRequest(c.Request().WithContext(ctx))

			return next(c)
		}
	}
}
