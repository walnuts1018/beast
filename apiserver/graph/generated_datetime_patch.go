package graph

import (
	"context"

	"github.com/99designs/gqlgen/graphql"
	"github.com/vektah/gqlparser/v2/ast"
	"github.com/walnuts1018/beast/apiserver/graph/scalar"
)

func (ec *executionContext) unmarshalInputDateTime(ctx context.Context, obj any) (scalar.DateTime, error) {
	res, err := UnmarshalDateTime(obj)
	return res, graphql.ErrorOnPath(ctx, err)
}

func (ec *executionContext) _DateTime(ctx context.Context, sel ast.SelectionSet, obj *scalar.DateTime) graphql.Marshaler {
	_ = sel
	if obj == nil {
		return graphql.Null
	}

	return MarshalDateTime(*obj)
}
