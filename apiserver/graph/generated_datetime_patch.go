package graph

import (
	"context"
	"time"

	"github.com/99designs/gqlgen/graphql"
	"github.com/vektah/gqlparser/v2/ast"
)

func (ec *executionContext) unmarshalInputDateTime(ctx context.Context, obj any) (time.Time, error) {
	res, err := UnmarshalDateTime(obj)
	return res, graphql.ErrorOnPath(ctx, err)
}

func (ec *executionContext) _DateTime(ctx context.Context, sel ast.SelectionSet, obj *time.Time) graphql.Marshaler {
	_ = sel
	if obj == nil {
		return graphql.Null
	}

	return MarshalDateTime(*obj)
}
