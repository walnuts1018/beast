package main

import (
	"context"
	"encoding/json/v2"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/labstack/echo/v5"
	"github.com/labstack/echo/v5/middleware"

	"github.com/walnuts1018/beast/backend/internal/encoding"
	"github.com/walnuts1018/beast/backend/internal/httpapi"
	"github.com/walnuts1018/beast/backend/internal/media"
	"github.com/walnuts1018/beast/backend/internal/store"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	slog.SetDefault(logger)
	environment := envOr("APP_ENV", "development")
	authMode := strings.ToLower(os.Getenv("AUTH_MODE"))
	if authMode == "" {
		if environment == "production" {
			authMode = "introspection"
		} else {
			authMode = "development"
		}
	}
	if authMode != "development" && authMode != "static" && authMode != "introspection" {
		logger.Error("AUTH_MODE must be static, development, or introspection")
		os.Exit(1)
	}
	if environment == "production" && authMode != "introspection" {
		logger.Error("production requires AUTH_MODE=introspection")
		os.Exit(1)
	}
	if authMode == "introspection" && (os.Getenv("OIDC_INTROSPECTION_URL") == "" || os.Getenv("OIDC_CLIENT_ID") == "" || os.Getenv("OIDC_CLIENT_SECRET") == "") {
		logger.Error("introspection authentication requires OIDC_INTROSPECTION_URL, OIDC_CLIENT_ID, and OIDC_CLIENT_SECRET")
		os.Exit(1)
	}
	if authMode == "static" && os.Getenv("AUTH_DEV_STATIC_TOKEN") == "" {
		logger.Error("static authentication requires AUTH_DEV_STATIC_TOKEN")
		os.Exit(1)
	}
	if environment == "production" && os.Getenv("DATABASE_URL") == "" {
		logger.Error("DATABASE_URL is required in production")
		os.Exit(1)
	}

	var videoStore store.Repository = store.NewMemory()
	var database *store.Postgres
	if databaseURL := os.Getenv("DATABASE_URL"); databaseURL != "" {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		var err error
		database, err = store.NewPostgres(ctx, databaseURL)
		if err == nil {
			err = database.Migrate(ctx, store.Schema)
		}
		cancel()
		if err != nil {
			if database != nil {
				database.Close()
			}
			logger.Error("database initialization failed", "error", err)
			os.Exit(1)
		}
		defer database.Close()
		videoStore = database
		logger.Info("database connection established and schema applied")
	} else {
		logger.Warn("DATABASE_URL is not set; using in-memory metadata repository")
	}

	var mediaStore media.ObjectStore
	var err error
	if environment == "production" || os.Getenv("OBJECT_STORAGE") == "s3" {
		mediaStore, err = media.NewS3Store(context.Background(), os.Getenv("S3_ENDPOINT"), envOr("S3_REGION", "us-east-1"), os.Getenv("S3_BUCKET"), firstNonEmptyEnv("S3_ACCESS_KEY_ID", "AWS_ACCESS_KEY_ID", "S3_ACCESS_KEY"), firstNonEmptyEnv("S3_SECRET_ACCESS_KEY", "AWS_SECRET_ACCESS_KEY", "S3_SECRET_KEY"), os.Getenv("STAGING_ENCRYPTION_KEY"))
		if err == nil {
			err = mediaStore.(*media.S3Store).Check(context.Background())
		}
	} else {
		mediaStore, err = media.NewLocalStore(envOr("MEDIA_DIR", ".beast-data/media"), os.Getenv("STAGING_ENCRYPTION_KEY"))
	}
	if err != nil {
		logger.Error("media storage initialization failed", "error", err)
		os.Exit(1)
	}
	var rabbit *encoding.RabbitMQ
	var deliveries <-chan encoding.Delivery
	if rabbitURL := os.Getenv("RABBITMQ_URL"); rabbitURL != "" {
		rabbit, deliveries, err = encoding.NewRabbitMQ(rabbitURL, envOr("RABBITMQ_ENCODE_JOB_QUEUE", "beast.encoder.jobs"), envOr("RABBITMQ_ENCODE_EVENT_QUEUE", "beast.encoder.events"), envOr("RABBITMQ_CONSUMER_TAG", "beast-api"))
		if err != nil {
			logger.Error("encoder queue initialization failed", "error", err)
			os.Exit(1)
		}
		defer func() {
			if closeErr := rabbit.Close(); closeErr != nil {
				logger.Error("encoder queue close failed", "error", closeErr)
			}
		}()
	} else if environment == "production" {
		logger.Error("RABBITMQ_URL is required in production")
		os.Exit(1)
	} else {
		logger.Warn("RABBITMQ_URL is not set; video encoding is unavailable")
	}
	auth := httpapi.Authenticator{
		Mode:          authMode,
		Environment:   environment,
		StaticToken:   os.Getenv("AUTH_DEV_STATIC_TOKEN"),
		Introspection: os.Getenv("OIDC_INTROSPECTION_URL"),
		ClientID:      os.Getenv("OIDC_CLIENT_ID"),
		ClientSecret:  os.Getenv("OIDC_CLIENT_SECRET"),
		HTTPClient:    &http.Client{Timeout: 5 * time.Second},
	}
	e := echo.New()
	e.Use(middleware.RequestLogger())
	e.Use(middleware.Recover())
	server := &httpapi.Server{Videos: videoStore, Media: mediaStore, Jobs: rabbit}
	server.Register(e, auth, environment != "production")
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	if rabbit != nil {
		go consumeEncodingEvents(ctx, server, deliveries, logger)
	}

	serverErrors := make(chan error, 1)
	httpServer := &http.Server{Addr: envOr("HTTP_ADDR", ":8080"), Handler: e}
	go func() { serverErrors <- httpServer.ListenAndServe() }()

	select {
	case err := <-serverErrors:
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			logger.Error("http server stopped", "error", err)
			os.Exit(1)
		}
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := httpServer.Shutdown(shutdownCtx); err != nil {
			logger.Error("http server shutdown failed", "error", err)
			os.Exit(1)
		}
	}
}

func consumeEncodingEvents(ctx context.Context, server *httpapi.Server, deliveries <-chan encoding.Delivery, logger *slog.Logger) {
	for {
		select {
		case <-ctx.Done():
			return
		case delivery, ok := <-deliveries:
			if !ok {
				logger.Error("encoder event queue closed")
				return
			}
			var event encoding.Event
			if err := json.Unmarshal(delivery.Body(), &event); err != nil {
				logger.Error("decode encoder event failed", "error", err)
				_ = delivery.Nack(false)
				continue
			}
			if err := server.HandleEncodingEvent(ctx, event); err != nil {
				logger.Error("apply encoder event failed", "video_id", event.VideoID, "error", err)
				_ = delivery.Nack(true)
				continue
			}
			if err := delivery.Ack(); err != nil {
				logger.Error("ack encoder event failed", "error", err)
				return
			}
		}
	}
}

func envOr(name, fallback string) string {
	if value := os.Getenv(name); value != "" {
		return value
	}
	return fallback
}

func firstNonEmptyEnv(names ...string) string {
	for _, name := range names {
		if value := os.Getenv(name); value != "" {
			return value
		}
	}
	return ""
}
