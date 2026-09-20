package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"path"
	"strconv"
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
	e.GET("/api/auth/login", auth.Login)
	e.GET("/api/auth/mobile/login", auth.NativeLogin)
	e.GET("/api/auth/callback", auth.Callback)
	e.GET("/api/auth/session", auth.Session)
	e.POST("/api/auth/logout", auth.Logout)
	e.GET("/api/auth/logout", auth.Logout)
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
	e.GET("/api/videos/:id/hls/*", auth.Middleware(s.hlsArtifact))
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
	sourceStore, ok := s.Media.(media.SourceStore)
	if !ok {
		return echo.NewHTTPError(http.StatusServiceUnavailable, "source staging requires S3-compatible object storage")
	}
	sourceKey := "staging/" + uuid.New().String()
	if err := sourceStore.SaveSource(request.Context(), sourceKey, file); err != nil {
		return echo.NewHTTPError(http.StatusUnprocessableEntity, "stage upload").Wrap(err)
	}
	var tags []string
	if value := request.FormValue("tags"); value != "" {
		if err := json.Unmarshal([]byte(value), &tags); err != nil {
			_ = sourceStore.Delete(request.Context(), sourceKey)
			return echo.NewHTTPError(http.StatusBadRequest, "tags must be a JSON array").Wrap(err)
		}
	}
	video, err := domain.NewServerVideo(ownerID, tags, sourceKey)
	if err != nil {
		_ = sourceStore.Delete(request.Context(), sourceKey)
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}
	video.ObjectKey = "videos/" + video.ID + "/hls/manifest.m3u8"
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
	if err := s.Jobs.PublishJob(request.Context(), encoding.Job{ContractVersion: encoding.ContractVersion, VideoID: video.ID, OwnerID: ownerID, SourceObjectKey: sourceKey, OutputPrefix: "videos/" + video.ID + "/hls"}); err != nil {
		if transitionErr := video.TransitionStatus(domain.VideoStatusFailed); transitionErr == nil {
			video.ErrorMessage = "encoder job could not be queued"
			_, _ = s.Videos.UpdateVideo(request.Context(), video)
		}
		return echo.NewHTTPError(http.StatusServiceUnavailable, "queue encoding job").Wrap(err)
	}
	return c.JSON(http.StatusCreated, map[string]any{"id": video.ID, "status": video.Status, "progress": video.Progress})
}

func (s *Server) hlsArtifact(c *echo.Context) error {
	video, err := s.Videos.GetVideo(c.Request().Context(), graph.OwnerID(c.Request().Context()), c.Param("id"))
	if err != nil {
		return echo.NewHTTPError(http.StatusNotFound, "video not found")
	}
	if video.Status != domain.VideoStatusReady {
		return echo.NewHTTPError(http.StatusConflict, "video is not ready")
	}
	name := path.Clean(strings.TrimPrefix(c.Param("*"), "/"))
	if name == "." || name == ".." || strings.HasPrefix(name, "../") || strings.Contains(name, "/../") {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid HLS artifact")
	}
	artifact, ok := video.HLSArtifacts[name]
	if name == "manifest.m3u8" {
		artifact = domain.HLSArtifact{ObjectKey: video.ObjectKey, Encryption: video.Encryption}
		ok = true
	}
	if !ok || artifact.ObjectKey == "" {
		return echo.NewHTTPError(http.StatusNotFound, "HLS artifact not found")
	}
	return s.serveArtifact(c, artifact.ObjectKey, artifact.Encryption, artifactContentType(name))
}

func (s *Server) serveArtifact(c *echo.Context, objectKey string, encryption domain.EncryptionMetadata, contentType string) error {
	if encryption.Algorithm != crypto.AtRestAlgorithm || encryption.PlaintextSize < 0 {
		return echo.NewHTTPError(http.StatusInternalServerError, "media encryption metadata is invalid")
	}
	start, end, partial, err := requestedRange(c.Request(), encryption.PlaintextSize)
	if err != nil {
		c.Response().Header().Set("Content-Range", fmt.Sprintf("bytes */%d", encryption.PlaintextSize))
		return echo.NewHTTPError(http.StatusRequestedRangeNotSatisfiable, "invalid media range")
	}
	metadata := crypto.AtRestResult{Algorithm: encryption.Algorithm, ChunkSize: encryption.ChunkSize, Nonce: encryption.Nonce, EncryptedDataKey: encryption.EncryptedDataKey, PlaintextSize: encryption.PlaintextSize}
	body, err := s.Media.OpenDecrypted(c.Request().Context(), objectKey, metadata, start, end)
	if err != nil {
		return echo.NewHTTPError(http.StatusNotFound, "media object not found").Wrap(err)
	}
	defer func() { _ = body.Close() }()
	response := c.Response()
	response.Header().Set("Accept-Ranges", "bytes")
	response.Header().Set("Content-Length", strconv.FormatInt(end-start, 10))
	if partial {
		response.Header().Set("Content-Range", fmt.Sprintf("bytes %d-%d/%d", start, end-1, encryption.PlaintextSize))
	}
	status := http.StatusOK
	if partial {
		status = http.StatusPartialContent
	}
	return c.Stream(status, contentType, body)
}

func requestedRange(request *http.Request, size int64) (start, end int64, partial bool, err error) {
	if size < 0 {
		return 0, 0, false, errors.New("negative media size")
	}
	value := request.Header.Get("Range")
	if value == "" {
		return 0, size, false, nil
	}
	if !strings.HasPrefix(value, "bytes=") || strings.Contains(value, ",") {
		return 0, 0, false, errors.New("only one byte range is supported")
	}
	value = strings.TrimPrefix(value, "bytes=")
	left, right, ok := strings.Cut(value, "-")
	if !ok {
		return 0, 0, false, errors.New("invalid byte range")
	}
	if left == "" {
		count, parseErr := strconv.ParseInt(right, 10, 64)
		if parseErr != nil || count <= 0 {
			return 0, 0, false, errors.New("invalid suffix range")
		}
		if count > size {
			count = size
		}
		return size - count, size, true, nil
	}
	start, err = strconv.ParseInt(left, 10, 64)
	if err != nil || start < 0 || start >= size {
		return 0, 0, false, errors.New("invalid range start")
	}
	end = size
	if right != "" {
		last, parseErr := strconv.ParseInt(right, 10, 64)
		if parseErr != nil || last < start {
			return 0, 0, false, errors.New("invalid range end")
		}
		if last+1 < end {
			end = last + 1
		}
	}
	return start, end, true, nil
}

func artifactContentType(name string) string {
	switch {
	case strings.HasSuffix(name, ".m3u8"):
		return "application/vnd.apple.mpegurl"
	case strings.HasSuffix(name, ".m4s"):
		return "video/iso.segment"
	case strings.HasSuffix(name, ".mp4"):
		return "video/mp4"
	default:
		return "application/octet-stream"
	}
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
		if err := validateArtifact(event.Manifest, event.VideoID); err != nil {
			return fmt.Errorf("validate manifest artifact: %w", err)
		}
		artifacts := make(map[string]domain.HLSArtifact, len(event.Artifacts))
		for name, artifact := range event.Artifacts {
			if err := validateArtifact(artifact, event.VideoID); err != nil {
				return fmt.Errorf("validate artifact %q: %w", name, err)
			}
			artifacts[name] = domain.HLSArtifact{ObjectKey: artifact.ObjectKey, Encryption: toDomainEncryption(artifact.Encryption)}
		}
		if err := video.TransitionStatus(domain.VideoStatusReady); err != nil {
			return err
		}
		video.ObjectKey = event.Manifest.ObjectKey
		video.Encryption = toDomainEncryption(event.Manifest.Encryption)
		video.HLSArtifacts = artifacts
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

func validateArtifact(artifact encoding.Artifact, videoID string) error {
	prefix := "videos/" + videoID + "/hls/"
	if !strings.HasPrefix(artifact.ObjectKey, prefix) || artifact.Encryption.Algorithm != crypto.AtRestAlgorithm || artifact.Encryption.ChunkSize != crypto.AtRestChunkSize || artifact.Encryption.Nonce == "" || artifact.Encryption.EncryptedDataKey == "" || artifact.Encryption.PlaintextSize <= 0 {
		return errors.New("artifact encryption metadata or object key is invalid")
	}
	return nil
}

func toDomainEncryption(metadata encoding.EncryptionMetadata) domain.EncryptionMetadata {
	return domain.EncryptionMetadata{Algorithm: metadata.Algorithm, ChunkSize: metadata.ChunkSize, Nonce: metadata.Nonce, EncryptedDataKey: metadata.EncryptedDataKey, PlaintextSize: metadata.PlaintextSize}
}
