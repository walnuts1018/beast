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
	"time"

	"github.com/Code-Hex/synchro"
	"github.com/Code-Hex/synchro/tz"
	lru "github.com/hashicorp/golang-lru/v2"
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
		return Principal{}, errors.New("introspection endpoint is empty")
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

	expiresAt := synchro.In[tz.UTC](time.Unix(payload.Exp, 0))
	if expiresAt.Before(synchro.Now[tz.UTC]()) {
		return Principal{}, ErrUnauthorized
	}

	return Principal{Subject: payload.Sub, ExpiresAt: expiresAt}, nil
}

type cachedPrincipal struct {
	principal Principal
	until     synchro.Time[tz.UTC]
}

// introspectionCacheSize はキャッシュエントリの最大数です。
// 超過した場合は最も最近使われていないエントリが自動的に削除されます。
const introspectionCacheSize = 1024

type CachedIntrospector struct {
	base   Introspector
	maxTTL time.Duration
	cache  *lru.Cache[string, cachedPrincipal]
}

func NewCachedIntrospector(base Introspector, maxTTL time.Duration) *CachedIntrospector {
	cache, err := lru.New[string, cachedPrincipal](introspectionCacheSize)
	if err != nil {
		panic(err)
	}
	return &CachedIntrospector{
		base:   base,
		maxTTL: maxTTL,
		cache:  cache,
	}
}

func (i *CachedIntrospector) Introspect(ctx context.Context, token string) (Principal, error) {
	now := synchro.Now[tz.UTC]()

	if entry, ok := i.cache.Get(token); ok && now.Before(entry.until) && now.Before(entry.principal.ExpiresAt) {
		return entry.principal, nil
	}

	principal, err := i.base.Introspect(ctx, token)
	if err != nil {
		return Principal{}, err
	}

	ttl := time.Until(principal.ExpiresAt.StdTime())
	if ttl <= 0 {
		return Principal{}, errors.New("expired token")
	}
	if ttl > i.maxTTL {
		ttl = i.maxTTL
	}

	i.cache.Add(token, cachedPrincipal{principal: principal, until: now.Add(ttl)})

	return principal, nil
}
