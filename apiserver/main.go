package main

import (
	"context"
	"errors"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/99designs/gqlgen/graphql/handler"
	"github.com/99designs/gqlgen/graphql/handler/extension"
	"github.com/99designs/gqlgen/graphql/handler/lru"
	"github.com/99designs/gqlgen/graphql/handler/transport"
	"github.com/99designs/gqlgen/graphql/playground"
	"github.com/labstack/echo/v5"
	"github.com/vektah/gqlparser/v2/ast"

	"github.com/walnuts1018/beast/apiserver/auth"
	"github.com/walnuts1018/beast/apiserver/graph"
	"github.com/walnuts1018/beast/apiserver/infra/memory"
	"github.com/walnuts1018/beast/apiserver/usecase"
)

func main() {
	// TODO: PostgreSQL/sqlc実装へ切り替えるまでの暫定としてメモリ実装を利用する。
	// ローカル開発速度を優先しつつ、usecase/domainの依存方向を固定するため。
	store := memory.NewStore()
	service := usecase.NewService(
		memory.NewVideoRepository(store),
		memory.NewSharedKeyRepository(store),
		memory.NewDeviceKeyRepository(store),
		memory.NewUploadSessionRepository(store),
		memory.NewEncodingProgressRepository(store),
	)

	resolvers := &graph.Resolver{Service: service}
	gqlHandler := handler.New(graph.NewExecutableSchema(graph.Config{Resolvers: resolvers}))
	gqlHandler.SetQueryCache(lru.New[*ast.QueryDocument](1000))
	gqlHandler.Use(extension.Introspection{})
	gqlHandler.AddTransport(transport.Options{})
	gqlHandler.AddTransport(transport.GET{})
	gqlHandler.AddTransport(transport.POST{})
	gqlHandler.AddTransport(transport.Websocket{
		KeepAlivePingInterval: 10 * time.Second,
	})

	e := echo.New()

	e.GET("/livez", func(c *echo.Context) error {
		return c.NoContent(http.StatusOK)
	})
	e.GET("/readyz", func(c *echo.Context) error {
		return c.NoContent(http.StatusOK)
	})
	e.GET("/playground", echo.WrapHandler(playground.Handler("GraphQL playground", "/query")))

	introspector := auth.NewCachedIntrospector(
		auth.NewHTTPIntrospector(
			os.Getenv("OIDC_INTROSPECTION_URL"),
			os.Getenv("OIDC_CLIENT_ID"),
			os.Getenv("OIDC_CLIENT_SECRET"),
		),
		30*time.Second,
	)

	secured := e.Group("", auth.Middleware(introspector))
	secured.Any("/query", echo.WrapHandler(gqlHandler))

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	server := &http.Server{
		Addr:              ":" + port,
		Handler:           e,
		ReadHeaderTimeout: 5 * time.Second,
	}

	go func() {
		if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Printf("server start error: %v", err)
		}
	}()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	<-stop

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := server.Shutdown(ctx); err != nil {
		log.Printf("graceful shutdown error: %v", err)
	}
}
