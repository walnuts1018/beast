package graph

import (
	"time"

	"github.com/99designs/gqlgen/graphql"
)

func MarshalDateTime(t time.Time) graphql.Marshaler {
	return graphql.MarshalTime(t)
}

func UnmarshalDateTime(v any) (time.Time, error) {
	return graphql.UnmarshalTime(v)
}
