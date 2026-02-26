package auth

import (
	"context"
	"errors"
	"time"
)

type Principal struct {
	Subject   string
	ExpiresAt time.Time
}

type contextKey string

const principalKey contextKey = "principal"

var ErrUnauthorized = errors.New("unauthorized")

func WithPrincipal(ctx context.Context, principal Principal) context.Context {
	return context.WithValue(ctx, principalKey, principal)
}

func PrincipalFromContext(ctx context.Context) (Principal, bool) {
	principal, ok := ctx.Value(principalKey).(Principal)
	if !ok {
		return Principal{}, false
	}

	return principal, true
}
