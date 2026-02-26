package graph

// This file will not be regenerated automatically.
//
// It serves as dependency injection for your app, add any dependencies you require here.

import "github.com/walnuts1018/beast/apiserver/usecase"

//go:generate go tool gqlgen generate

type Resolver struct {
	Service *usecase.Service
}
