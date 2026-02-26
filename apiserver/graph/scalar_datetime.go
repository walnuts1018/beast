package graph

import (
	"github.com/99designs/gqlgen/graphql"
	"github.com/Code-Hex/synchro"
	"github.com/Code-Hex/synchro/tz"
	"github.com/walnuts1018/beast/apiserver/graph/scalar"
)

func MarshalDateTime(t scalar.DateTime) graphql.Marshaler {
	return graphql.MarshalTime(t.StdTime())
}

func UnmarshalDateTime(v any) (scalar.DateTime, error) {
	parsed, err := graphql.UnmarshalTime(v)
	if err != nil {
		return scalar.DateTime{}, err
	}

	return synchro.In[tz.UTC](parsed), nil
}
