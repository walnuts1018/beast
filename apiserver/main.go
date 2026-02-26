package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/99designs/gqlgen/graphql"
	"github.com/99designs/gqlgen/graphql/handler"
	"github.com/99designs/gqlgen/graphql/handler/extension"
	"github.com/99designs/gqlgen/graphql/handler/lru"
	"github.com/99designs/gqlgen/graphql/handler/transport"
	"github.com/99designs/gqlgen/graphql/playground"
	"github.com/labstack/echo/v5"
	"github.com/labstack/echo/v5/middleware"
	"github.com/vektah/gqlparser/v2/ast"
	"github.com/vektah/gqlparser/v2/gqlerror"

	"github.com/walnuts1018/beast/apiserver/auth"
	"github.com/walnuts1018/beast/apiserver/config"
	"github.com/walnuts1018/beast/apiserver/graph"
	"github.com/walnuts1018/beast/apiserver/infra/objectstorage"
	"github.com/walnuts1018/beast/apiserver/infra/postgres"
	"github.com/walnuts1018/beast/apiserver/logger"
	"github.com/walnuts1018/beast/apiserver/usecase"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGTERM, syscall.SIGINT)
	defer stop()

	cfg, err := config.Load()
	if err != nil {
		slog.ErrorContext(ctx, "Failed to load config", slog.Any("error", err))
		os.Exit(1)
	}

	logger := logger.CreateLogger(cfg.LogLevel, cfg.LogType)
	slog.SetDefault(logger)

	store, err := postgres.NewStoreWithOptions(ctx, cfg.DB.DSN(), postgres.StoreOptions{
		MaxOpenConns:    cfg.DB.MaxOpenConns,
		MaxIdleConns:    cfg.DB.MaxIdleConns,
		ConnMaxLifetime: cfg.DB.ConnMaxLifetime,
		ConnMaxIdleTime: cfg.DB.ConnMaxIdleTime,
	})
	if err != nil {
		slog.ErrorContext(ctx, "postgres initialization error", slog.Any("error", err))
		os.Exit(1)
	}
	defer func() {
		if closeErr := store.Close(); closeErr != nil {
			slog.ErrorContext(ctx, "postgres close error", slog.Any("error", closeErr))
		}
	}()

	objectStore, err := objectstorage.NewS3Storage(ctx, objectstorage.Config{
		Region:          cfg.S3.Region,
		Endpoint:        cfg.S3.Endpoint,
		Bucket:          cfg.S3.Bucket,
		AccessKeyID:     cfg.S3.AccessKeyID,
		SecretAccessKey: cfg.S3.SecretAccessKey,
		UsePathStyle:    cfg.S3.UsePathStyle,
		UploadTTL:       cfg.S3.UploadURLTTL,
	})
	if err != nil {
		slog.ErrorContext(ctx, "object storage initialization error", slog.Any("error", err))
		os.Exit(1)
	}

	service := usecase.NewService(
		postgres.NewVideoRepository(store),
		postgres.NewSharedKeyRepository(store),
		postgres.NewDeviceKeyRepository(store),
		postgres.NewUploadSessionRepository(store),
		postgres.NewEncodingProgressRepository(store),
		objectStore,
	)

	resolvers := &graph.Resolver{Service: service}
	gqlHandler := handler.New(graph.NewExecutableSchema(graph.Config{Resolvers: resolvers}))
	gqlHandler.SetQueryCache(lru.New[*ast.QueryDocument](1000))
	gqlHandler.Use(extension.Introspection{})
	gqlHandler.SetRecoverFunc(func(ctx context.Context, panicErr any) error {
		slog.ErrorContext(ctx, "graphql panic recovered", slog.Any("panic", panicErr))
		return gqlerror.Errorf("internal server error")
	})
	gqlHandler.SetErrorPresenter(func(ctx context.Context, err error) *gqlerror.Error {
		presented := graphql.DefaultErrorPresenter(ctx, err)

		switch {
		case errors.Is(err, auth.ErrUnauthorized), errors.Is(err, usecase.ErrUnauthorized):
			presented.Message = "unauthorized"
			presented.Extensions = map[string]any{"code": "UNAUTHORIZED"}
		case errors.Is(err, usecase.ErrNotFound):
			presented.Message = "not found"
			presented.Extensions = map[string]any{"code": "NOT_FOUND"}
		case errors.Is(err, usecase.ErrInvalidInput):
			presented.Message = "invalid input"
			presented.Extensions = map[string]any{"code": "INVALID_INPUT"}
		}

		return presented
	})
	gqlHandler.AddTransport(transport.Options{})
	gqlHandler.AddTransport(transport.GET{})
	gqlHandler.AddTransport(transport.POST{})
	gqlHandler.AddTransport(transport.Websocket{
		KeepAlivePingInterval: 10 * time.Second,
	})

	e := echo.New()
	e.Use(middleware.RequestID())
	e.Use(middleware.Recover())
	e.Use(middleware.CORS())

	e.GET("/livez", func(c *echo.Context) error {
		return c.NoContent(http.StatusOK)
	})
	e.GET("/readyz", func(c *echo.Context) error {
		healthCtx, cancel := context.WithTimeout(c.Request().Context(), 3*time.Second)
		defer cancel()

		if err := store.Ping(healthCtx); err != nil {
			slog.WarnContext(healthCtx, "postgres not ready", slog.Any("error", err))
			return c.NoContent(http.StatusServiceUnavailable)
		}

		if err := objectStore.HealthCheck(healthCtx); err != nil {
			slog.WarnContext(healthCtx, "object storage not ready", slog.Any("error", err))
			return c.NoContent(http.StatusServiceUnavailable)
		}

		return c.NoContent(http.StatusOK)
	})
	e.GET("/playground", echo.WrapHandler(playground.Handler("GraphQL playground", "/query")))

	introspector := newIntrospector(cfg)

	secured := e.Group("", auth.Middleware(introspector))
	secured.Any("/query", echo.WrapHandler(gqlHandler))

	server := &http.Server{
		Addr:              fmt.Sprintf(":%d", cfg.Server.Port),
		Handler:           e,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	go func() {
		slog.InfoContext(ctx, "starting api server", slog.Int("port", cfg.Server.Port), slog.String("auth_mode", string(cfg.Auth.Mode)))

		if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			slog.ErrorContext(ctx, "server start error", slog.Any("error", err))
		}
	}()

	<-ctx.Done()
	stop()
	slog.InfoContext(ctx, "shutting down server")

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := server.Shutdown(ctx); err != nil {
		slog.ErrorContext(ctx, "graceful shutdown error", slog.Any("error", err))
		os.Exit(1)
	}

	slog.InfoContext(ctx, "server stopped")
}

func newIntrospector(cfg *config.Config) auth.Introspector {
	switch cfg.Auth.Mode {
	case config.AuthModeStatic:
		return auth.NewStaticTokenIntrospector(cfg.Auth.DevStaticToken, cfg.Auth.DevStaticSubject)
	case config.AuthModeIntrospection:
		fallthrough
	default:
		return auth.NewCachedIntrospector(
			auth.NewHTTPIntrospector(
				cfg.OIDC.IntrospectionURL,
				cfg.OIDC.ClientID,
				cfg.OIDC.ClientSecret,
			),
			cfg.Auth.IntrospectionCacheTTL,
		)
	}
}
