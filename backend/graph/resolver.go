package graph

import (
	"context"

	"github.com/walnuts1018/beast/backend/internal/media"
	"github.com/walnuts1018/beast/backend/internal/store"
)

type Resolver struct {
	Store store.Repository
	Media media.ObjectStore
}

type ownerContextKey struct{}

func WithOwnerID(ctx context.Context, ownerID string) context.Context {
	return context.WithValue(ctx, ownerContextKey{}, ownerID)
}

func OwnerID(ctx context.Context) string {
	ownerID, _ := ctx.Value(ownerContextKey{}).(string)
	return ownerID
}

func NewResolver(videos store.Repository, mediaStore media.ObjectStore) *Resolver {
	return &Resolver{Store: videos, Media: mediaStore}
}
