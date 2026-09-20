package httpapi

import (
	"encoding/base64"
	"net/http"

	"github.com/99designs/gqlgen/graphql/handler"
	"github.com/99designs/gqlgen/graphql/playground"
	"github.com/google/uuid"
	"github.com/labstack/echo/v5"

	"github.com/walnuts1018/beast/backend/graph"
	"github.com/walnuts1018/beast/backend/internal/domain"
	"github.com/walnuts1018/beast/backend/internal/media"
	"github.com/walnuts1018/beast/backend/internal/store"
)

type Server struct {
	Videos store.Repository
	Media  media.ObjectStore
}

func (s *Server) Register(e *echo.Echo, auth Authenticator) {
	e.GET("/healthz", func(c *echo.Context) error { return c.JSON(http.StatusOK, map[string]string{"status": "ok"}) })
	e.GET("/livez", func(c *echo.Context) error { return c.JSON(http.StatusOK, map[string]string{"status": "ok"}) })
	e.GET("/readyz", func(c *echo.Context) error { return c.JSON(http.StatusOK, map[string]string{"status": "ok"}) })
	e.GET("/graphql", echo.WrapHandler(playground.Handler("GraphQL Playground", "/graphql/query")))
	graphqlHandler := handler.NewDefaultServer(graph.NewExecutableSchema(graph.Config{Resolvers: graph.NewResolver(s.Videos, s.Media)}))
	e.Any("/graphql/query", func(c *echo.Context) error {
		return auth.Middleware(func(c *echo.Context) error {
			graphqlHandler.ServeHTTP(c.Response(), c.Request())
			return nil
		})(c)
	})
	e.POST("/api/videos/upload", auth.Middleware(s.upload))
	e.GET("/api/videos/:id/stream", auth.Middleware(s.stream))
}

func (s *Server) upload(c *echo.Context) error {
	request := c.Request()
	if err := request.ParseMultipartForm(32 << 20); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid multipart upload")
	}
	file, _, err := request.FormFile("file")
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "file is required")
	}
	defer func() { _ = file.Close() }()
	ownerID := graph.OwnerID(request.Context())
	sharedKeyID := request.FormValue("shared_key_id")
	key, err := s.Videos.SharedKey(request.Context(), ownerID, sharedKeyID)
	if err != nil {
		return echo.NewHTTPError(http.StatusNotFound, "shared key not found")
	}
	objectKey := uuid.New().String()
	encrypted, err := s.Media.Save(request.Context(), objectKey, file, key.PublicKey)
	if err != nil {
		return echo.NewHTTPError(http.StatusUnprocessableEntity, "encrypt upload").Wrap(err)
	}
	keyVersion := request.FormValue("key_version")
	if keyVersion == "" {
		keyVersion = key.Version
	}
	video, err := domain.NewVideo(ownerID, request.FormValue("encrypted_tags"), objectKey, domain.EncryptionMetadata{
		Algorithm: encrypted.Algorithm, ChunkSize: encrypted.ChunkSize, KeyVersion: keyVersion, Nonce: base64.RawStdEncoding.EncodeToString(encrypted.Nonce),
		EncryptedDataKey: base64.RawStdEncoding.EncodeToString(encrypted.EncryptedDataKey), SharedKeyID: sharedKeyID,
	})
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}
	video, err = s.Videos.CreateVideo(request.Context(), video)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "save video").Wrap(err)
	}
	return c.JSON(http.StatusCreated, video)
}

func (s *Server) stream(c *echo.Context) error {
	request := c.Request()
	video, err := s.Videos.GetVideo(request.Context(), graph.OwnerID(request.Context()), c.Param("id"))
	if err != nil {
		return echo.NewHTTPError(http.StatusNotFound, "video not found")
	}
	file, err := s.Media.Open(request.Context(), video.ObjectKey)
	if err != nil {
		return echo.NewHTTPError(http.StatusNotFound, "media object not found").Wrap(err)
	}
	defer func() { _ = file.Close() }()
	c.Response().Header().Set("X-Encryption-Algorithm", video.Encryption.Algorithm)
	c.Response().Header().Set("X-Encryption-Key-Version", video.Encryption.KeyVersion)
	c.Response().Header().Set("X-Encryption-Nonce", video.Encryption.Nonce)
	c.Response().Header().Set("X-Encryption-Data-Key", video.Encryption.EncryptedDataKey)
	return c.Stream(http.StatusOK, "application/octet-stream", file)
}
