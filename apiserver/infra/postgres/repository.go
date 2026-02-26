package postgres

import (
	"context"
	"embed"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/pgx/v5"
	"github.com/golang-migrate/migrate/v4/source/iofs"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/walnuts1018/beast/apiserver/domain"
	"github.com/walnuts1018/beast/apiserver/infra/postgres/sqlcgen"
)

//go:embed migrations/*.sql
var migrationsFS embed.FS

var ErrNotFound = errors.New("not found")

type Store struct {
	pool    *pgxpool.Pool
	queries *sqlcgen.Queries

	mu          sync.RWMutex
	subscribers map[string]map[chan domain.VideoEncodingProgress]struct{}
}

func NewStore(ctx context.Context, dsn string) (*Store, error) {
	return NewStoreWithOptions(ctx, dsn, StoreOptions{})
}

type StoreOptions struct {
	MaxOpenConns    int
	MaxIdleConns    int
	ConnMaxLifetime time.Duration
	ConnMaxIdleTime time.Duration
}

func NewStoreWithOptions(ctx context.Context, dsn string, options StoreOptions) (*Store, error) {
	poolConfig, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		return nil, fmt.Errorf("parse postgres config: %w", err)
	}

	if options.MaxOpenConns > 0 {
		poolConfig.MaxConns = int32(options.MaxOpenConns)
	}
	if options.ConnMaxLifetime > 0 {
		poolConfig.MaxConnLifetime = options.ConnMaxLifetime
	}
	if options.ConnMaxIdleTime > 0 {
		poolConfig.MaxConnIdleTime = options.ConnMaxIdleTime
	}

	pool, err := pgxpool.NewWithConfig(ctx, poolConfig)
	if err != nil {
		return nil, fmt.Errorf("open postgres pool: %w", err)
	}

	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("ping postgres: %w", err)
	}

	store := &Store{
		pool:        pool,
		queries:     sqlcgen.New(pool),
		subscribers: make(map[string]map[chan domain.VideoEncodingProgress]struct{}),
	}

	if err := store.runMigrations(dsn); err != nil {
		pool.Close()
		return nil, err
	}

	return store, nil
}

func (s *Store) runMigrations(dsn string) error {
	source, err := iofs.New(migrationsFS, "migrations")
	if err != nil {
		return fmt.Errorf("create migration source: %w", err)
	}

	m, err := migrate.NewWithSourceInstance("iofs", source, "pgx5://"+dsn)
	if err != nil {
		return fmt.Errorf("create migrate instance: %w", err)
	}
	defer func() {
		sourceErr, dbErr := m.Close()
		if sourceErr != nil {
			slog.Error("マイグレーションソースのクローズに失敗", slog.Any("error", sourceErr))
		}
		if dbErr != nil {
			slog.Error("マイグレーションDBのクローズに失敗", slog.Any("error", dbErr))
		}
	}()

	if err := m.Up(); err != nil && !errors.Is(err, migrate.ErrNoChange) {
		return fmt.Errorf("run migrations: %w", err)
	}

	return nil
}

func (s *Store) Close() {
	s.pool.Close()
}

func (s *Store) Ping(ctx context.Context) error {
	return s.pool.Ping(ctx)
}

// VideoRepository

type VideoRepository struct{ store *Store }

func NewVideoRepository(store *Store) *VideoRepository {
	return &VideoRepository{store: store}
}

func (r *VideoRepository) Create(ctx context.Context, video domain.Video) error {
	if err := r.store.queries.CreateVideo(ctx, toCreateVideoParams(video)); err != nil {
		return fmt.Errorf("insert video: %w", err)
	}
	return nil
}

func (r *VideoRepository) GetByID(ctx context.Context, videoID string) (*domain.Video, error) {
	row, err := r.store.queries.GetVideoByID(ctx, videoID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("get video: %w", err)
	}
	video := toDomainVideo(row)
	return &video, nil
}

func (r *VideoRepository) ListByOwner(
	ctx context.Context,
	ownerUserID string,
	status *domain.VideoStatus,
	p domain.Pagination,
) (domain.VideoConnection, error) {
	limit := p.First
	if limit <= 0 {
		limit = 20
	}

	params := sqlcgen.ListVideosByOwnerParams{
		OwnerUserID: ownerUserID,
		LimitCount:  int32(limit + 1),
	}

	if status != nil {
		params.Status = toPgText(string(*status))
	}

	if p.After != nil {
		cursorAt, err := r.store.queries.LoadVideoUploadedAt(ctx, sqlcgen.LoadVideoUploadedAtParams{
			ID:          *p.After,
			OwnerUserID: ownerUserID,
		})
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return domain.VideoConnection{}, nil
			}
			return domain.VideoConnection{}, fmt.Errorf("load cursor: %w", err)
		}
		params.CursorUploadedAt = cursorAt
		params.CursorID = toPgText(*p.After)
	}

	rows, err := r.store.queries.ListVideosByOwner(ctx, params)
	if err != nil {
		return domain.VideoConnection{}, fmt.Errorf("list videos: %w", err)
	}

	hasNext := len(rows) > limit
	if hasNext {
		rows = rows[:limit]
	}

	edges := make([]domain.VideoEdge, 0, len(rows))
	for _, row := range rows {
		video := toDomainVideo(row)
		edges = append(edges, domain.VideoEdge{Cursor: video.ID, Node: video})
	}

	var next *string
	if hasNext && len(edges) > 0 {
		cursor := edges[len(edges)-1].Cursor
		next = &cursor
	}

	return domain.VideoConnection{Edges: edges, HasNext: hasNext, NextCursor: next}, nil
}

func (r *VideoRepository) Update(ctx context.Context, video domain.Video) error {
	rowsAffected, err := r.store.queries.UpdateVideo(ctx, toUpdateVideoParams(video))
	if err != nil {
		return fmt.Errorf("update video: %w", err)
	}
	if rowsAffected == 0 {
		return ErrNotFound
	}
	return nil
}

// SharedKeyRepository

type SharedKeyRepository struct{ store *Store }

func NewSharedKeyRepository(store *Store) *SharedKeyRepository {
	return &SharedKeyRepository{store: store}
}

func (r *SharedKeyRepository) Register(ctx context.Context, userID string, publicKeyPEM string) (domain.SharedKeyVersion, error) {
	tx, err := r.store.pool.Begin(ctx)
	if err != nil {
		return domain.SharedKeyVersion{}, fmt.Errorf("begin tx: %w", err)
	}
	defer func() {
		if err := tx.Rollback(ctx); err != nil && !errors.Is(err, pgx.ErrTxClosed) {
			slog.ErrorContext(ctx, "rollback shared key tx", slog.Any("error", err))
		}
	}()

	qtx := r.store.queries.WithTx(tx)

	nextVersion, err := qtx.GetNextSharedKeyVersion(ctx, userID)
	if err != nil {
		return domain.SharedKeyVersion{}, fmt.Errorf("next shared key version: %w", err)
	}

	now := time.Now().UTC()
	if err := qtx.InsertSharedKeyVersion(ctx, sqlcgen.InsertSharedKeyVersionParams{
		UserID:       userID,
		Version:      nextVersion,
		PublicKeyPem: publicKeyPEM,
		Status:       string(domain.SharedKeyVersionStatusActive),
		CreatedAt:    toPgTimestamptz(now),
	}); err != nil {
		return domain.SharedKeyVersion{}, fmt.Errorf("insert shared key version: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return domain.SharedKeyVersion{}, fmt.Errorf("commit: %w", err)
	}

	return domain.SharedKeyVersion{
		Version:      int(nextVersion),
		PublicKeyPEM: publicKeyPEM,
		Status:       domain.SharedKeyVersionStatusActive,
		CreatedAt:    now,
	}, nil
}

func (r *SharedKeyRepository) Revoke(ctx context.Context, userID string, version int) (domain.SharedKeyVersion, error) {
	now := time.Now().UTC()
	row, err := r.store.queries.RevokeSharedKeyVersion(ctx, sqlcgen.RevokeSharedKeyVersionParams{
		Status:    string(domain.SharedKeyVersionStatusRevoked),
		RevokedAt: toPgTimestamptz(now),
		UserID:    userID,
		Version:   int32(version),
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.SharedKeyVersion{}, ErrNotFound
		}
		return domain.SharedKeyVersion{}, fmt.Errorf("revoke shared key: %w", err)
	}

	return toDomainSharedKeyVersionFromRevoke(row), nil
}

func (r *SharedKeyRepository) List(ctx context.Context, userID string) ([]domain.SharedKeyVersion, error) {
	rows, err := r.store.queries.ListSharedKeyVersions(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("list shared keys: %w", err)
	}

	items := make([]domain.SharedKeyVersion, 0, len(rows))
	for _, row := range rows {
		items = append(items, toDomainSharedKeyVersionFromList(row))
	}
	return items, nil
}

func (r *SharedKeyRepository) Get(ctx context.Context, userID string, version int) (*domain.SharedKeyVersion, error) {
	row, err := r.store.queries.GetSharedKeyVersion(ctx, sqlcgen.GetSharedKeyVersionParams{
		UserID:  userID,
		Version: int32(version),
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("get shared key: %w", err)
	}

	item := toDomainSharedKeyVersion(row)
	return &item, nil
}

// DeviceKeyRepository

type DeviceKeyRepository struct{ store *Store }

func NewDeviceKeyRepository(store *Store) *DeviceKeyRepository {
	return &DeviceKeyRepository{store: store}
}

func (r *DeviceKeyRepository) RegisterWrappedSharedKey(
	ctx context.Context,
	userID string,
	data domain.DeviceWrappedSharedKey,
) (domain.DeviceWrappedSharedKey, error) {
	if err := r.store.queries.UpsertDeviceWrappedSharedKey(ctx, sqlcgen.UpsertDeviceWrappedSharedKeyParams{
		UserID:                    userID,
		DeviceID:                  data.DeviceID,
		DevicePublicKeyPem:        data.DevicePublicKeyPEM,
		SharedKeyVersion:          int32(data.SharedKeyVersion),
		EncryptedSharedPrivateKey: data.EncryptedSharedPrivateKey,
		CreatedAt:                 toPgTimestamptz(data.CreatedAt),
	}); err != nil {
		return domain.DeviceWrappedSharedKey{}, fmt.Errorf("register device wrapped key: %w", err)
	}
	return data, nil
}

func (r *DeviceKeyRepository) ListWrappedSharedKeys(ctx context.Context, userID string) ([]domain.DeviceWrappedSharedKey, error) {
	rows, err := r.store.queries.ListDeviceWrappedSharedKeys(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("list device wrapped keys: %w", err)
	}

	items := make([]domain.DeviceWrappedSharedKey, 0, len(rows))
	for _, row := range rows {
		items = append(items, toDomainDeviceWrappedSharedKey(row))
	}
	return items, nil
}

// UploadSessionRepository

type UploadSessionRepository struct{ store *Store }

func NewUploadSessionRepository(store *Store) *UploadSessionRepository {
	return &UploadSessionRepository{store: store}
}

func (r *UploadSessionRepository) Create(ctx context.Context, session domain.UploadSession) error {
	if err := r.store.queries.CreateUploadSession(ctx, sqlcgen.CreateUploadSessionParams{
		ID:        session.ID,
		OwnerUser: session.OwnerUser,
		ObjectKey: session.ObjectKey,
		UploadUrl: session.UploadURL,
		ExpiresAt: toPgTimestamptz(session.ExpiresAt),
		CreatedAt: toPgTimestamptz(session.CreatedAt),
	}); err != nil {
		return fmt.Errorf("create upload session: %w", err)
	}
	return nil
}

func (r *UploadSessionRepository) GetByID(ctx context.Context, id string) (*domain.UploadSession, error) {
	row, err := r.store.queries.GetUploadSessionByID(ctx, id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("get upload session: %w", err)
	}
	session := toDomainUploadSession(row)
	return &session, nil
}

func (r *UploadSessionRepository) Delete(ctx context.Context, id string) error {
	if err := r.store.queries.DeleteUploadSession(ctx, id); err != nil {
		return fmt.Errorf("delete upload session: %w", err)
	}
	return nil
}

// EncodingProgressRepository

type EncodingProgressRepository struct{ store *Store }

func NewEncodingProgressRepository(store *Store) *EncodingProgressRepository {
	return &EncodingProgressRepository{store: store}
}

func (r *EncodingProgressRepository) Set(ctx context.Context, progress domain.VideoEncodingProgress) error {
	if err := r.store.queries.UpsertEncodingProgress(ctx, sqlcgen.UpsertEncodingProgressParams{
		VideoID:   progress.VideoID,
		OwnerUser: progress.OwnerUser,
		Status:    string(progress.Status),
		Percent:   progress.Percent,
		UpdatedAt: toPgTimestamptz(progress.UpdatedAt),
		Message:   toPgTextPtr(progress.Message),
	}); err != nil {
		return fmt.Errorf("upsert encoding progress: %w", err)
	}

	r.store.mu.RLock()
	subscribers := r.store.subscribers[progress.VideoID]
	for ch := range subscribers {
		select {
		case ch <- progress:
		default:
		}
	}
	r.store.mu.RUnlock()

	return nil
}

func (r *EncodingProgressRepository) Get(ctx context.Context, videoID string) (*domain.VideoEncodingProgress, error) {
	row, err := r.store.queries.GetEncodingProgress(ctx, videoID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("get encoding progress: %w", err)
	}
	progress := toDomainEncodingProgress(row)
	return &progress, nil
}

func (r *EncodingProgressRepository) Subscribe(
	ctx context.Context,
	videoID string,
) (<-chan domain.VideoEncodingProgress, func(), error) {
	ch := make(chan domain.VideoEncodingProgress, 8)

	r.store.mu.Lock()
	if _, ok := r.store.subscribers[videoID]; !ok {
		r.store.subscribers[videoID] = make(map[chan domain.VideoEncodingProgress]struct{})
	}
	r.store.subscribers[videoID][ch] = struct{}{}
	r.store.mu.Unlock()

	cleanupOnce := sync.Once{}
	cleanup := func() {
		cleanupOnce.Do(func() {
			r.store.mu.Lock()
			if subscribers, ok := r.store.subscribers[videoID]; ok {
				delete(subscribers, ch)
			}
			r.store.mu.Unlock()
			close(ch)
		})
	}

	go func() {
		<-ctx.Done()
		cleanup()
	}()

	return ch, cleanup, nil
}
