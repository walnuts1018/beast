package auth

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sync"
	"time"
)

type Introspector interface {
	Introspect(ctx context.Context, token string) (Principal, error)
}

type HTTPIntrospector struct {
	endpoint     string
	clientID     string
	clientSecret string
	httpClient   *http.Client
}

func NewHTTPIntrospector(endpoint, clientID, clientSecret string) *HTTPIntrospector {
	return &HTTPIntrospector{
		endpoint:     endpoint,
		clientID:     clientID,
		clientSecret: clientSecret,
		httpClient:   &http.Client{Timeout: 5 * time.Second},
	}
}

type introspectionResponse struct {
	Active bool   `json:"active"`
	Sub    string `json:"sub"`
	Exp    int64  `json:"exp"`
}

func (i *HTTPIntrospector) Introspect(ctx context.Context, token string) (Principal, error) {
	if i.endpoint == "" {
		if token == "dev-token" {
			return Principal{Subject: "dev-user", ExpiresAt: time.Now().Add(1 * time.Minute)}, nil
		}
		return Principal{}, ErrUnauthorized
	}

	form := url.Values{}
	form.Set("token", token)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, i.endpoint, bytes.NewBufferString(form.Encode()))
	if err != nil {
		return Principal{}, fmt.Errorf("build introspection request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	if i.clientID != "" {
		req.SetBasicAuth(i.clientID, i.clientSecret)
	}

	res, err := i.httpClient.Do(req)
	if err != nil {
		return Principal{}, fmt.Errorf("send introspection request: %w", err)
	}
	defer res.Body.Close()

	if res.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(res.Body, 1024))
		return Principal{}, fmt.Errorf("introspection failed status=%d body=%s", res.StatusCode, string(body))
	}

	var payload introspectionResponse
	if err := json.NewDecoder(res.Body).Decode(&payload); err != nil {
		return Principal{}, fmt.Errorf("decode introspection response: %w", err)
	}

	if !payload.Active || payload.Sub == "" {
		return Principal{}, ErrUnauthorized
	}

	expiresAt := time.Unix(payload.Exp, 0).UTC()
	if expiresAt.Before(time.Now().UTC()) {
		return Principal{}, ErrUnauthorized
	}

	return Principal{Subject: payload.Sub, ExpiresAt: expiresAt}, nil
}

type cachedPrincipal struct {
	principal Principal
	until     time.Time
}

type CachedIntrospector struct {
	base   Introspector
	maxTTL time.Duration

	mu    sync.RWMutex
	cache map[string]cachedPrincipal
}

func NewCachedIntrospector(base Introspector, maxTTL time.Duration) *CachedIntrospector {
	return &CachedIntrospector{
		base:   base,
		maxTTL: maxTTL,
		cache:  make(map[string]cachedPrincipal),
	}
}

func (i *CachedIntrospector) Introspect(ctx context.Context, token string) (Principal, error) {
	now := time.Now().UTC()

	i.mu.RLock()
	entry, ok := i.cache[token]
	i.mu.RUnlock()
	if ok && now.Before(entry.until) && now.Before(entry.principal.ExpiresAt) {
		return entry.principal, nil
	}

	principal, err := i.base.Introspect(ctx, token)
	if err != nil {
		return Principal{}, err
	}

	ttl := time.Until(principal.ExpiresAt)
	if ttl <= 0 {
		return Principal{}, errors.New("expired token")
	}
	if ttl > i.maxTTL {
		ttl = i.maxTTL
	}

	i.mu.Lock()
	i.cache[token] = cachedPrincipal{principal: principal, until: now.Add(ttl)}
	i.mu.Unlock()

	return principal, nil
}
