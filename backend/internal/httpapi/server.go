package httpapi

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"path"
	"strings"

	"github.com/99designs/gqlgen/graphql/handler"
	"github.com/99designs/gqlgen/graphql/playground"
	"github.com/google/uuid"
	"github.com/labstack/echo/v5"

	"github.com/walnuts1018/beast/backend/graph"
	"github.com/walnuts1018/beast/backend/internal/crypto"
	"github.com/walnuts1018/beast/backend/internal/domain"
	"github.com/walnuts1018/beast/backend/internal/encoding"
	"github.com/walnuts1018/beast/backend/internal/media"
	"github.com/walnuts1018/beast/backend/internal/store"
)

type Server struct {
	Videos store.Repository
	Media  media.ObjectStore
	Jobs   encoding.JobDispatcher
}

func (s *Server) Register(e *echo.Echo, auth Authenticator, playgroundEnabled bool) {
	e.GET("/healthz", func(c *echo.Context) error { return c.JSON(http.StatusOK, map[string]string{"status": "ok"}) })
	e.GET("/livez", func(c *echo.Context) error { return c.JSON(http.StatusOK, map[string]string{"status": "ok"}) })
	e.GET("/readyz", func(c *echo.Context) error { return c.JSON(http.StatusOK, map[string]string{"status": "ok"}) })
	if playgroundEnabled {
		e.GET("/graphql", echo.WrapHandler(playground.Handler("GraphQL Playground", "/graphql/query")))
	}
	graphqlHandler := handler.NewDefaultServer(graph.NewExecutableSchema(graph.Config{Resolvers: graph.NewResolver(s.Videos, s.Media)}))
	e.Any("/graphql/query", func(c *echo.Context) error {
		return auth.Middleware(func(c *echo.Context) error {
			graphqlHandler.ServeHTTP(c.Response(), c.Request())
			return nil
		})(c)
	})
	e.POST("/api/videos/upload", auth.Middleware(s.upload))
	e.GET("/api/videos/:id/stream", auth.Middleware(s.stream))
	e.GET("/api/videos/:id/dash/*", auth.Middleware(s.dashArtifact))
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
	if s.Jobs == nil {
		return echo.NewHTTPError(http.StatusServiceUnavailable, "video encoding is unavailable")
	}
	sharedKeyID := request.FormValue("shared_key_id")
	key, err := s.Videos.SharedKey(request.Context(), ownerID, sharedKeyID)
	if err != nil {
		return echo.NewHTTPError(http.StatusNotFound, "shared key not found")
	}
	sourceStore, ok := s.Media.(media.SourceStore)
	if !ok {
		return echo.NewHTTPError(http.StatusServiceUnavailable, "source staging requires S3-compatible object storage")
	}
	sourceKey := "staging/" + uuid.New().String()
	if err := sourceStore.SaveSource(request.Context(), sourceKey, file); err != nil {
		return echo.NewHTTPError(http.StatusUnprocessableEntity, "stage upload").Wrap(err)
	}
	video, err := domain.NewVideo(ownerID, request.FormValue("encrypted_tags"), sourceKey, domain.EncryptionMetadata{ChunkSize: crypto.ChunkSize, KeyVersion: key.Version, SharedKeyID: sharedKeyID})
	if err != nil {
		_ = sourceStore.Delete(request.Context(), sourceKey)
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}
	video.ObjectKey = "videos/" + video.ID + "/dash/manifest.mpd"
	video.SourceObjectKey = sourceKey
	video, err = s.Videos.CreateVideo(request.Context(), video)
	if err != nil {
		_ = sourceStore.Delete(request.Context(), sourceKey)
		return echo.NewHTTPError(http.StatusInternalServerError, "save video").Wrap(err)
	}
	if err := video.TransitionStatus(domain.VideoStatusEncoding); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "start encoding").Wrap(err)
	}
	if err := video.SetProgress(0); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "initialize encoding progress").Wrap(err)
	}
	video, err = s.Videos.UpdateVideo(request.Context(), video)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "start encoding").Wrap(err)
	}
	if err := s.Jobs.PublishJob(request.Context(), encoding.Job{ContractVersion: encoding.ContractVersion, VideoID: video.ID, OwnerID: ownerID, SourceObjectKey: sourceKey, OutputPrefix: "videos/" + video.ID + "/dash", PublicKey: key.PublicKey, SharedKeyID: key.ID, KeyVersion: key.Version}); err != nil {
		if transitionErr := video.TransitionStatus(domain.VideoStatusFailed); transitionErr == nil {
			video.ErrorMessage = "encoder job could not be queued"
			_, _ = s.Videos.UpdateVideo(request.Context(), video)
		}
		return echo.NewHTTPError(http.StatusServiceUnavailable, "queue encoding job").Wrap(err)
	}
	return c.JSON(http.StatusCreated, map[string]any{"id": video.ID, "status": video.Status, "progress": video.Progress})
}

func (s *Server) stream(c *echo.Context) error {
	request := c.Request()
	video, err := s.Videos.GetVideo(request.Context(), graph.OwnerID(request.Context()), c.Param("id"))
	if err != nil {
		return echo.NewHTTPError(http.StatusNotFound, "video not found")
	}
	if video.Status != domain.VideoStatusReady {
		return echo.NewHTTPError(http.StatusConflict, "video is not ready")
	}
	file, err := s.Media.Open(request.Context(), video.ObjectKey)
	if err != nil {
		return echo.NewHTTPError(http.StatusNotFound, "media object not found").Wrap(err)
	}
	defer func() { _ = file.Close() }()
	video, err = s.Videos.RecordPlayback(request.Context(), graph.OwnerID(request.Context()), c.Param("id"))
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "record playback").Wrap(err)
	}
	c.Response().Header().Set("X-Encryption-Algorithm", video.Encryption.Algorithm)
	c.Response().Header().Set("X-Encryption-Chunk-Size", fmt.Sprintf("%d", video.Encryption.ChunkSize))
	c.Response().Header().Set("X-Encryption-Key-Version", video.Encryption.KeyVersion)
	c.Response().Header().Set("X-Encryption-Nonce", video.Encryption.Nonce)
	c.Response().Header().Set("X-Encryption-Data-Key", video.Encryption.EncryptedDataKey)
	c.Response().Header().Set("X-Encryption-Shared-Key-ID", video.Encryption.SharedKeyID)
	return c.Stream(http.StatusOK, "application/octet-stream", file)
}

func (s *Server) dashArtifact(c *echo.Context) error {
	request := c.Request()
	video, err := s.Videos.GetVideo(request.Context(), graph.OwnerID(request.Context()), c.Param("id"))
	if err != nil {
		return echo.NewHTTPError(http.StatusNotFound, "video not found")
	}
	if video.Status != domain.VideoStatusReady {
		return echo.NewHTTPError(http.StatusConflict, "video is not ready")
	}
	name := path.Clean(strings.TrimPrefix(c.Param("*"), "/"))
	if name == "." || name == ".." || strings.HasPrefix(name, "../") || strings.Contains(name, "/../") {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid DASH artifact")
	}
	artifact, ok := video.DashArtifacts[name]
	if name == "manifest.mpd" {
		artifact = domain.DashArtifact{ObjectKey: video.ObjectKey, Encryption: video.Encryption}
		ok = true
	}
	if !ok || artifact.ObjectKey == "" {
		return echo.NewHTTPError(http.StatusNotFound, "DASH artifact not found")
	}
	file, err := s.Media.Open(request.Context(), artifact.ObjectKey)
	if err != nil {
		return echo.NewHTTPError(http.StatusNotFound, "DASH artifact not found").Wrap(err)
	}
	defer func() { _ = file.Close() }()
	setEncryptionHeaders(c, artifact.Encryption)
	return c.Stream(http.StatusOK, "application/octet-stream", file)
}

func setEncryptionHeaders(c *echo.Context, encryption domain.EncryptionMetadata) {
	c.Response().Header().Set("X-Encryption-Algorithm", encryption.Algorithm)
	c.Response().Header().Set("X-Encryption-Chunk-Size", fmt.Sprintf("%d", encryption.ChunkSize))
	c.Response().Header().Set("X-Encryption-Key-Version", encryption.KeyVersion)
	c.Response().Header().Set("X-Encryption-Nonce", encryption.Nonce)
	c.Response().Header().Set("X-Encryption-Data-Key", encryption.EncryptedDataKey)
	c.Response().Header().Set("X-Encryption-Shared-Key-ID", encryption.SharedKeyID)
}

func (s *Server) HandleEncodingEvent(ctx context.Context, event encoding.Event) error {
	if event.ContractVersion != encoding.ContractVersion || event.VideoID == "" || event.OwnerID == "" || event.SourceObjectKey == "" {
		return errors.New("invalid encoder event contract")
	}
	video, err := s.Videos.GetVideo(ctx, event.OwnerID, event.VideoID)
	if err != nil {
		return fmt.Errorf("load video for encoder event: %w", err)
	}
	if video.SourceObjectKey != event.SourceObjectKey {
		return errors.New("encoder event source does not match video")
	}
	switch event.Status {
	case "ENCODING":
		if video.Status == domain.VideoStatusReady {
			return nil
		}
		if video.Status == domain.VideoStatusUploaded {
			if err := video.TransitionStatus(domain.VideoStatusEncoding); err != nil {
				return err
			}
		} else if video.Status == domain.VideoStatusFailed {
			if err := video.TransitionStatus(domain.VideoStatusEncoding); err != nil {
				return err
			}
		} else if video.Status != domain.VideoStatusEncoding {
			return errors.New("encoding event has invalid video status")
		}
		if event.Progress < video.Progress {
			return errors.New("encoding progress moved backwards")
		}
		if err := video.SetProgress(event.Progress); err != nil {
			return err
		}
		_, err = s.Videos.UpdateVideo(ctx, video)
		return err
	case "FAILED":
		if video.Status == domain.VideoStatusFailed && video.ErrorMessage == event.Error {
			return nil
		}
		if video.Status != domain.VideoStatusEncoding {
			return errors.New("failed event has invalid video status")
		}
		if err := video.TransitionStatus(domain.VideoStatusFailed); err != nil {
			return err
		}
		video.ErrorMessage = event.Error
		_, err = s.Videos.UpdateVideo(ctx, video)
		return err
	case "READY":
		if video.Status == domain.VideoStatusReady && video.ObjectKey == event.Manifest.ObjectKey {
			return nil
		}
		if video.Status != domain.VideoStatusEncoding {
			return errors.New("ready event has invalid video status")
		}
		if event.Progress != 1 {
			return errors.New("ready event must have complete progress")
		}
		if err := validateArtifact(event.Manifest, event.VideoID, video.Encryption.SharedKeyID, video.Encryption.KeyVersion); err != nil {
			return fmt.Errorf("validate manifest artifact: %w", err)
		}
		artifacts := make(map[string]domain.DashArtifact, len(event.Artifacts))
		for name, artifact := range event.Artifacts {
			if err := validateArtifact(artifact, event.VideoID, video.Encryption.SharedKeyID, video.Encryption.KeyVersion); err != nil {
				return fmt.Errorf("validate artifact %q: %w", name, err)
			}
			if name == "manifest.mpd" {
				continue
			}
			artifacts[name] = domain.DashArtifact{ObjectKey: artifact.ObjectKey, Encryption: toDomainEncryption(artifact.Encryption)}
		}
		if err := video.TransitionStatus(domain.VideoStatusReady); err != nil {
			return err
		}
		video.ObjectKey = event.Manifest.ObjectKey
		video.Encryption = toDomainEncryption(event.Manifest.Encryption)
		video.DashArtifacts = artifacts
		video.ErrorMessage = ""
		if err := video.SetProgress(1); err != nil {
			return err
		}
		if _, err := s.Videos.UpdateVideo(ctx, video); err != nil {
			return err
		}
		return nil
	default:
		return errors.New("unknown encoder event status")
	}
}

func validateArtifact(artifact encoding.Artifact, videoID, sharedKeyID, keyVersion string) error {
	prefix := "videos/" + videoID + "/dash/"
	if !strings.HasPrefix(artifact.ObjectKey, prefix) || artifact.Encryption.SharedKeyID != sharedKeyID || artifact.Encryption.KeyVersion != keyVersion || artifact.Encryption.Algorithm == "" || artifact.Encryption.ChunkSize <= 0 || artifact.Encryption.Nonce == "" || artifact.Encryption.EncryptedDataKey == "" {
		return errors.New("artifact encryption metadata or object key is invalid")
	}
	return nil
}

func toDomainEncryption(metadata encoding.EncryptionMetadata) domain.EncryptionMetadata {
	return domain.EncryptionMetadata{Algorithm: metadata.Algorithm, ChunkSize: metadata.ChunkSize, KeyVersion: metadata.KeyVersion, Nonce: metadata.Nonce, EncryptedDataKey: metadata.EncryptedDataKey, SharedKeyID: metadata.SharedKeyID}
}
