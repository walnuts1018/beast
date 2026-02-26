package auth

import (
	"context"
	"crypto/subtle"
	"time"

	"github.com/Code-Hex/synchro"
	"github.com/Code-Hex/synchro/tz"
)

type StaticTokenIntrospector struct {
	token   string
	subject string
}

func NewStaticTokenIntrospector(token, subject string) *StaticTokenIntrospector {
	return &StaticTokenIntrospector{token: token, subject: subject}
}

func (i *StaticTokenIntrospector) Introspect(_ context.Context, token string) (Principal, error) {
	if subtle.ConstantTimeCompare([]byte(i.token), []byte(token)) != 1 {
		return Principal{}, ErrUnauthorized
	}

	return Principal{
		Subject:   i.subject,
		ExpiresAt: synchro.Now[tz.UTC]().Add(24 * time.Hour),
	}, nil
}
