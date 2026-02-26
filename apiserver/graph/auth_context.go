package graph

import (
	"context"

	"github.com/walnuts1018/beast/apiserver/auth"
)

func userIDFromContext(ctx context.Context) (string, error) {
	principal, ok := auth.PrincipalFromContext(ctx)
	if !ok || principal.Subject == "" {
		return "", auth.ErrUnauthorized
	}

	return principal.Subject, nil
}
