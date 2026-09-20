package httpapi

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/labstack/echo/v5"
	"github.com/walnuts1018/beast/backend/graph"
)

type Authenticator struct {
	Mode                 string
	Environment          string
	StaticToken          string
	Introspection        string
	ClientID             string
	ClientSecret         string
	AuthorizationURL     string
	TokenURL             string
	RedirectURL          string
	FrontendURL          string
	SessionCookieName    string
	LoginStateCookieName string
	SessionCookieMaxAge  int
	SecureCookies        bool
	Scopes               []string
	HTTPClient           *http.Client
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
	if a.Mode == "introspection" {
		token, ok := strings.CutPrefix(authorization, "Bearer ")
		if !ok || token == "" {
			var err error
			token, err = a.sessionToken(request)
			if err != nil {
				return "", errors.New("bearer token is required")
			}
		}
		return a.introspect(ctx, token)
	}
	if a.Mode == "static" {
		token, ok := strings.CutPrefix(authorization, "Bearer ")
		if !ok || token == "" || a.StaticToken == "" || subtle.ConstantTimeCompare([]byte(token), []byte(a.StaticToken)) != 1 {
			return "", errors.New("static bearer token is invalid")
		}
		subject := request.Header.Get("X-User-ID")
		if subject == "" {
			subject = "static-development-user"
		}
		if strings.ContainsAny(subject, "\r\n") {
			return "", errors.New("subject is invalid")
		}
		return subject, nil
	}
	if token, err := a.sessionToken(request); err == nil {
		return a.introspect(ctx, token)
	}
	if a.Mode == "development" && a.Environment != "production" {
		if subject := request.Header.Get("X-User-ID"); subject != "" && !strings.ContainsAny(subject, "\r\n") {
			return subject, nil
		}
	}
	return "", errors.New("no supported credentials")
}

func (a Authenticator) Login(c *echo.Context) error {
	if a.AuthorizationURL == "" || a.ClientID == "" || a.RedirectURL == "" {
		return echo.NewHTTPError(http.StatusServiceUnavailable, "OIDC login is not configured")
	}
	state, err := randomToken(32)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "create login state").Wrap(err)
	}
	verifier, err := randomToken(32)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "create PKCE verifier").Wrap(err)
	}
	stateCookieValue := state + "." + verifier
	c.SetCookie(a.newCookie(a.loginStateCookieName(), stateCookieValue, 600))

	authorizationURL, err := url.Parse(a.AuthorizationURL)
	if err != nil || authorizationURL.Scheme == "" || authorizationURL.Host == "" {
		return echo.NewHTTPError(http.StatusServiceUnavailable, "OIDC authorization URL is invalid")
	}
	query := authorizationURL.Query()
	query.Set("client_id", a.ClientID)
	query.Set("redirect_uri", a.RedirectURL)
	query.Set("response_type", "code")
	query.Set("scope", strings.Join(a.scopes(), " "))
	query.Set("state", state)
	query.Set("code_challenge", codeChallenge(verifier))
	query.Set("code_challenge_method", "S256")
	authorizationURL.RawQuery = query.Encode()
	return c.Redirect(http.StatusFound, authorizationURL.String())
}

func (a Authenticator) Callback(c *echo.Context) error {
	cookie, err := c.Cookie(a.loginStateCookieName())
	if err != nil || cookie.Value == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "login state is missing")
	}
	c.SetCookie(a.newCookie(a.loginStateCookieName(), "", -1))
	state, verifier, ok := strings.Cut(cookie.Value, ".")
	returnedState := c.QueryParam("state")
	code := c.QueryParam("code")
	if !ok || state == "" || verifier == "" || returnedState == "" || subtle.ConstantTimeCompare([]byte(state), []byte(returnedState)) != 1 {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid login state")
	}
	if providerError := c.QueryParam("error"); providerError != "" {
		return echo.NewHTTPError(http.StatusBadRequest, "OIDC authorization failed")
	}
	if code == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "authorization code is missing")
	}
	token, err := a.exchangeCode(c.Request().Context(), code, verifier)
	if err != nil {
		return echo.NewHTTPError(http.StatusUnauthorized, "authorization code exchange failed").Wrap(err)
	}
	if _, err := a.introspect(c.Request().Context(), token.AccessToken); err != nil {
		return echo.NewHTTPError(http.StatusUnauthorized, "access token is invalid").Wrap(err)
	}
	maxAge := a.sessionCookieMaxAge()
	if token.ExpiresIn > 0 && token.ExpiresIn < maxAge {
		maxAge = token.ExpiresIn
	}
	c.SetCookie(a.newCookie(a.sessionCookieName(), token.AccessToken, maxAge))
	return c.Redirect(http.StatusFound, a.frontendURL())
}

func (a Authenticator) Session(c *echo.Context) error {
	subject, err := a.Subject(c.Request().Context(), c.Request())
	if err != nil {
		return echo.NewHTTPError(http.StatusUnauthorized, "authentication required")
	}
	return c.JSON(http.StatusOK, map[string]any{"authenticated": true, "subject": subject})
}

func (a Authenticator) Logout(c *echo.Context) error {
	c.SetCookie(a.newCookie(a.sessionCookieName(), "", -1))
	return c.NoContent(http.StatusNoContent)
}

type tokenResponse struct {
	AccessToken string `json:"access_token"`
	TokenType   string `json:"token_type"`
	ExpiresIn   int    `json:"expires_in"`
}

func (a Authenticator) exchangeCode(ctx context.Context, code, verifier string) (tokenResponse, error) {
	if a.TokenURL == "" || a.ClientID == "" || a.ClientSecret == "" || a.RedirectURL == "" {
		return tokenResponse{}, errors.New("OIDC token exchange is not configured")
	}
	form := url.Values{
		"grant_type":    {"authorization_code"},
		"code":          {code},
		"code_verifier": {verifier},
		"redirect_uri":  {a.RedirectURL},
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, a.TokenURL, strings.NewReader(form.Encode()))
	if err != nil {
		return tokenResponse{}, err
	}
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	request.SetBasicAuth(a.ClientID, a.ClientSecret)
	response, err := a.httpClient().Do(request)
	if err != nil {
		return tokenResponse{}, err
	}
	defer func() { _ = response.Body.Close() }()
	if response.StatusCode != http.StatusOK {
		return tokenResponse{}, fmt.Errorf("OIDC token endpoint returned status %d", response.StatusCode)
	}
	var token tokenResponse
	if err := json.NewDecoder(response.Body).Decode(&token); err != nil {
		return tokenResponse{}, err
	}
	if token.AccessToken == "" || !strings.EqualFold(token.TokenType, "bearer") {
		return tokenResponse{}, errors.New("OIDC token response did not contain a bearer access token")
	}
	return token, nil
}

func (a Authenticator) sessionToken(request *http.Request) (string, error) {
	cookie, err := request.Cookie(a.sessionCookieName())
	if err != nil || cookie.Value == "" {
		return "", errors.New("session cookie is missing")
	}
	return cookie.Value, nil
}

func (a Authenticator) newCookie(name, value string, maxAge int) *http.Cookie {
	return &http.Cookie{Name: name, Value: value, Path: "/", MaxAge: maxAge, HttpOnly: true, Secure: a.SecureCookies, SameSite: http.SameSiteLaxMode}
}

func (a Authenticator) sessionCookieName() string {
	if a.SessionCookieName != "" {
		return a.SessionCookieName
	}
	return "beast_session"
}

func (a Authenticator) loginStateCookieName() string {
	if a.LoginStateCookieName != "" {
		return a.LoginStateCookieName
	}
	return "beast_oidc_state"
}

func (a Authenticator) sessionCookieMaxAge() int {
	if a.SessionCookieMaxAge > 0 {
		return a.SessionCookieMaxAge
	}
	return int((8 * time.Hour).Seconds())
}

func (a Authenticator) frontendURL() string {
	if a.FrontendURL != "" {
		return a.FrontendURL
	}
	return "/"
}

func (a Authenticator) scopes() []string {
	if len(a.Scopes) > 0 {
		return a.Scopes
	}
	return []string{"openid", "profile", "email"}
}

func (a Authenticator) httpClient() *http.Client {
	if a.HTTPClient != nil {
		return a.HTTPClient
	}
	return http.DefaultClient
}

func codeChallenge(verifier string) string {
	digest := sha256.Sum256([]byte(verifier))
	return base64.RawURLEncoding.EncodeToString(digest[:])
}

func randomToken(size int) (string, error) {
	value := make([]byte, size)
	if _, err := rand.Read(value); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(value), nil
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
	response, err := a.httpClient().Do(request)
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
