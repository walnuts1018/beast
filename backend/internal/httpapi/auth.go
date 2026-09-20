package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/labstack/echo/v5"
	"github.com/walnuts1018/beast/backend/graph"
)

type Authenticator struct {
	Environment   string
	Introspection string
	ClientID      string
	ClientSecret  string
	HTTPClient    *http.Client
}

func (a Authenticator) Middleware(next echo.HandlerFunc) echo.HandlerFunc {
	return func(c *echo.Context) error {
		subject, err := a.Subject(c.Request().Context(), c.Request())
		if err != nil {
			return echo.NewHTTPError(http.StatusUnauthorized, "authentication required")
		}
		c.SetRequest(c.Request().WithContext(graph.WithOwnerID(c.Request().Context(), subject)))
		return next(c)
	}
}

func (a Authenticator) Subject(ctx context.Context, request *http.Request) (string, error) {
	authorization := request.Header.Get("Authorization")
	if token, ok := strings.CutPrefix(authorization, "Bearer "); ok && token != "" {
		return a.introspect(ctx, token)
	}
	if a.Environment != "production" {
		if subject := request.Header.Get("X-User-ID"); subject != "" && !strings.ContainsAny(subject, "\r\n") {
			return subject, nil
		}
	}
	return "", errors.New("no supported credentials")
}

func (a Authenticator) introspect(ctx context.Context, token string) (string, error) {
	if a.Introspection == "" || a.ClientID == "" || a.ClientSecret == "" {
		return "", errors.New("token introspection is not configured")
	}
	form := url.Values{"token": {token}}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, a.Introspection, strings.NewReader(form.Encode()))
	if err != nil {
		return "", err
	}
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	request.SetBasicAuth(a.ClientID, a.ClientSecret)
	client := a.HTTPClient
	if client == nil {
		client = http.DefaultClient
	}
	response, err := client.Do(request)
	if err != nil {
		return "", err
	}
	defer func() { _ = response.Body.Close() }()
	if response.StatusCode != http.StatusOK {
		return "", errors.New("token introspection rejected")
	}
	var result struct {
		Active bool   `json:"active"`
		Sub    string `json:"sub"`
		Exp    int64  `json:"exp"`
	}
	if err := json.NewDecoder(response.Body).Decode(&result); err != nil {
		return "", err
	}
	if !result.Active || result.Sub == "" || (result.Exp != 0 && !time.Unix(result.Exp, 0).After(time.Now())) {
		return "", errors.New("token is inactive or expired")
	}
	return result.Sub, nil
}
