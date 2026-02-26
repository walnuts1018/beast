package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"

	"github.com/walnuts1018/beast/apiserver/domain"
)

var ErrNotFound = errors.New("not found")

type Store struct {
	db *sql.DB

	mu          sync.RWMutex
	subscribers map[string]map[chan domain.VideoEncodingProgress]struct{}
}

func NewStore(ctx context.Context, dsn string) (*Store, error) {
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		return nil, fmt.Errorf("open postgres: %w", err)
	}

	if err := db.PingContext(ctx); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("ping postgres: %w", err)
	}

	store := &Store{
		db:          db,
		subscribers: make(map[string]map[chan domain.VideoEncodingProgress]struct{}),
	}

	if err := store.migrate(ctx); err != nil {
		_ = db.Close()
		return nil, err
	}

	return store, nil
}

func (s *Store) Close() error {
	return s.db.Close()
}

func (s *Store) migrate(ctx context.Context) error {
	queries := []string{
		`CREATE TABLE IF NOT EXISTS shared_key_versions (
			user_id TEXT NOT NULL,
			version INTEGER NOT NULL,
			public_key_pem TEXT NOT NULL,
			status TEXT NOT NULL,
			created_at TIMESTAMPTZ NOT NULL,
			revoked_at TIMESTAMPTZ,
			PRIMARY KEY (user_id, version)
		)`,
		`CREATE TABLE IF NOT EXISTS device_wrapped_shared_keys (
			user_id TEXT NOT NULL,
			device_id TEXT NOT NULL,
			shared_key_version INTEGER NOT NULL,
			encrypted_shared_private_key BYTEA NOT NULL,
			created_at TIMESTAMPTZ NOT NULL,
			PRIMARY KEY (user_id, device_id)
		)`,
		`CREATE TABLE IF NOT EXISTS upload_sessions (
			id TEXT PRIMARY KEY,
			owner_user TEXT NOT NULL,
			object_key TEXT NOT NULL,
			upload_url TEXT NOT NULL,
			expires_at TIMESTAMPTZ NOT NULL,
			created_at TIMESTAMPTZ NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS videos (
			id TEXT PRIMARY KEY,
			owner_user_id TEXT NOT NULL,
			status TEXT NOT NULL,
			uploaded_at TIMESTAMPTZ NOT NULL,
			ready_at TIMESTAMPTZ,
			failed_reason TEXT,
			duration_millis INTEGER,
			width INTEGER,
			height INTEGER,
			playback_manifest_url TEXT,
			playback_expires_at TIMESTAMPTZ,
			playback_enc_algorithm TEXT,
			playback_enc_key_version INTEGER,
			playback_enc_nonce BYTEA,
			playback_enc_encrypted_data_key BYTEA,
			encrypted_tags BYTEA NOT NULL,
			tag_enc_algorithm TEXT NOT NULL,
			tag_enc_key_version INTEGER NOT NULL,
			tag_enc_nonce BYTEA NOT NULL,
			tag_enc_encrypted_data_key BYTEA NOT NULL,
			content_enc_algorithm TEXT,
			content_enc_key_version INTEGER,
			content_enc_nonce BYTEA,
			content_enc_encrypted_data_key BYTEA,
			created_at TIMESTAMPTZ NOT NULL,
			updated_at TIMESTAMPTZ NOT NULL
		)`,
		`CREATE INDEX IF NOT EXISTS videos_owner_uploaded_idx
		 ON videos(owner_user_id, uploaded_at DESC, id DESC)`,
		`CREATE TABLE IF NOT EXISTS encoding_progress (
			video_id TEXT PRIMARY KEY,
			owner_user TEXT NOT NULL,
			status TEXT NOT NULL,
			percent DOUBLE PRECISION NOT NULL,
			updated_at TIMESTAMPTZ NOT NULL,
			message TEXT
		)`,
	}

	for _, query := range queries {
		if _, err := s.db.ExecContext(ctx, query); err != nil {
			return fmt.Errorf("migrate postgres: %w", err)
		}
	}

	return nil
}

type VideoRepository struct{ store *Store }

func NewVideoRepository(store *Store) *VideoRepository {
	return &VideoRepository{store: store}
}

func (r *VideoRepository) Create(ctx context.Context, video domain.Video) error {
	const query = `INSERT INTO videos (
		id, owner_user_id, status, uploaded_at, ready_at, failed_reason,
		duration_millis, width, height,
		playback_manifest_url, playback_expires_at,
		playback_enc_algorithm, playback_enc_key_version, playback_enc_nonce, playback_enc_encrypted_data_key,
		encrypted_tags, tag_enc_algorithm, tag_enc_key_version, tag_enc_nonce, tag_enc_encrypted_data_key,
		content_enc_algorithm, content_enc_key_version, content_enc_nonce, content_enc_encrypted_data_key,
		created_at, updated_at
	) VALUES (
		$1,$2,$3,$4,$5,$6,
		$7,$8,$9,
		$10,$11,
		$12,$13,$14,$15,
		$16,$17,$18,$19,$20,
		$21,$22,$23,$24,
		$25,$26
	)`

	playback := nullablePlayback(video.Playback)
	content := nullableEncryption(video.ContentEncryption)

	_, err := r.store.db.ExecContext(ctx, query,
		video.ID,
		video.OwnerUserID,
		string(video.Status),
		video.UploadedAt,
		nullTimePtr(video.ReadyAt),
		nullStringPtr(video.FailedReason),
		nullIntPtr(video.DurationMillis),
		nullIntPtr(video.Width),
		nullIntPtr(video.Height),
		playback.ManifestURL,
		playback.ExpiresAt,
		playback.Algorithm,
		playback.KeyVersion,
		playback.Nonce,
		playback.EncryptedDataKey,
		video.EncryptedTags,
		string(video.TagEncryption.Algorithm),
		video.TagEncryption.KeyVersion,
		video.TagEncryption.Nonce,
		video.TagEncryption.EncryptedDataKey,
		content.Algorithm,
		content.KeyVersion,
		content.Nonce,
		content.EncryptedDataKey,
		video.CreatedAt,
		video.UpdatedAt,
	)
	if err != nil {
		return fmt.Errorf("insert video: %w", err)
	}

	return nil
}

func (r *VideoRepository) GetByID(ctx context.Context, videoID string) (*domain.Video, error) {
	const query = `SELECT
		id, owner_user_id, status, uploaded_at, ready_at, failed_reason,
		duration_millis, width, height,
		playback_manifest_url, playback_expires_at,
		playback_enc_algorithm, playback_enc_key_version, playback_enc_nonce, playback_enc_encrypted_data_key,
		encrypted_tags, tag_enc_algorithm, tag_enc_key_version, tag_enc_nonce, tag_enc_encrypted_data_key,
		content_enc_algorithm, content_enc_key_version, content_enc_nonce, content_enc_encrypted_data_key,
		created_at, updated_at
	FROM videos WHERE id = $1`

	video, err := scanVideo(r.store.db.QueryRowContext(ctx, query, videoID))
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("get video: %w", err)
	}

	return video, nil
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

	args := []any{ownerUserID}
	conditions := []string{"owner_user_id = $1"}

	if status != nil {
		args = append(args, string(*status))
		conditions = append(conditions, fmt.Sprintf("status = $%d", len(args)))
	}

	if p.After != nil {
		cursorAt, err := r.loadUploadedAtByID(ctx, ownerUserID, *p.After)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return domain.VideoConnection{}, nil
			}
			return domain.VideoConnection{}, fmt.Errorf("load cursor: %w", err)
		}

		args = append(args, cursorAt, *p.After)
		conditions = append(conditions, fmt.Sprintf("(uploaded_at, id) < ($%d, $%d)", len(args)-1, len(args)))
	}

	args = append(args, limit+1)
	query := `SELECT
		id, owner_user_id, status, uploaded_at, ready_at, failed_reason,
		duration_millis, width, height,
		playback_manifest_url, playback_expires_at,
		playback_enc_algorithm, playback_enc_key_version, playback_enc_nonce, playback_enc_encrypted_data_key,
		encrypted_tags, tag_enc_algorithm, tag_enc_key_version, tag_enc_nonce, tag_enc_encrypted_data_key,
		content_enc_algorithm, content_enc_key_version, content_enc_nonce, content_enc_encrypted_data_key,
		created_at, updated_at
	FROM videos
	WHERE ` + strings.Join(conditions, " AND ") + `
	ORDER BY uploaded_at DESC, id DESC
	LIMIT $` + fmt.Sprint(len(args))

	rows, err := r.store.db.QueryContext(ctx, query, args...)
	if err != nil {
		return domain.VideoConnection{}, fmt.Errorf("list videos: %w", err)
	}
	defer rows.Close()

	videos := make([]domain.Video, 0, limit+1)
	for rows.Next() {
		video, scanErr := scanVideo(rows)
		if scanErr != nil {
			return domain.VideoConnection{}, fmt.Errorf("scan video row: %w", scanErr)
		}
		videos = append(videos, *video)
	}
	if err := rows.Err(); err != nil {
		return domain.VideoConnection{}, fmt.Errorf("iterate videos: %w", err)
	}

	hasNext := len(videos) > limit
	if hasNext {
		videos = videos[:limit]
	}

	edges := make([]domain.VideoEdge, 0, len(videos))
	for _, item := range videos {
		edges = append(edges, domain.VideoEdge{Cursor: item.ID, Node: item})
	}

	var next *string
	if hasNext && len(edges) > 0 {
		cursor := edges[len(edges)-1].Cursor
		next = &cursor
	}

	return domain.VideoConnection{Edges: edges, HasNext: hasNext, NextCursor: next}, nil
}

func (r *VideoRepository) loadUploadedAtByID(ctx context.Context, ownerUserID string, videoID string) (time.Time, error) {
	const query = `SELECT uploaded_at FROM videos WHERE id = $1 AND owner_user_id = $2`

	var uploadedAt time.Time
	if err := r.store.db.QueryRowContext(ctx, query, videoID, ownerUserID).Scan(&uploadedAt); err != nil {
		return time.Time{}, err
	}

	return uploadedAt, nil
}

func (r *VideoRepository) Update(ctx context.Context, video domain.Video) error {
	const query = `UPDATE videos SET
		owner_user_id = $2,
		status = $3,
		uploaded_at = $4,
		ready_at = $5,
		failed_reason = $6,
		duration_millis = $7,
		width = $8,
		height = $9,
		playback_manifest_url = $10,
		playback_expires_at = $11,
		playback_enc_algorithm = $12,
		playback_enc_key_version = $13,
		playback_enc_nonce = $14,
		playback_enc_encrypted_data_key = $15,
		encrypted_tags = $16,
		tag_enc_algorithm = $17,
		tag_enc_key_version = $18,
		tag_enc_nonce = $19,
		tag_enc_encrypted_data_key = $20,
		content_enc_algorithm = $21,
		content_enc_key_version = $22,
		content_enc_nonce = $23,
		content_enc_encrypted_data_key = $24,
		created_at = $25,
		updated_at = $26
	WHERE id = $1`

	playback := nullablePlayback(video.Playback)
	content := nullableEncryption(video.ContentEncryption)

	result, err := r.store.db.ExecContext(ctx, query,
		video.ID,
		video.OwnerUserID,
		string(video.Status),
		video.UploadedAt,
		nullTimePtr(video.ReadyAt),
		nullStringPtr(video.FailedReason),
		nullIntPtr(video.DurationMillis),
		nullIntPtr(video.Width),
		nullIntPtr(video.Height),
		playback.ManifestURL,
		playback.ExpiresAt,
		playback.Algorithm,
		playback.KeyVersion,
		playback.Nonce,
		playback.EncryptedDataKey,
		video.EncryptedTags,
		string(video.TagEncryption.Algorithm),
		video.TagEncryption.KeyVersion,
		video.TagEncryption.Nonce,
		video.TagEncryption.EncryptedDataKey,
		content.Algorithm,
		content.KeyVersion,
		content.Nonce,
		content.EncryptedDataKey,
		video.CreatedAt,
		video.UpdatedAt,
	)
	if err != nil {
		return fmt.Errorf("update video: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("rows affected update video: %w", err)
	}
	if rowsAffected == 0 {
		return ErrNotFound
	}

	return nil
}

type SharedKeyRepository struct{ store *Store }

func NewSharedKeyRepository(store *Store) *SharedKeyRepository {
	return &SharedKeyRepository{store: store}
}

func (r *SharedKeyRepository) Register(ctx context.Context, userID string, publicKeyPEM string) (domain.SharedKeyVersion, error) {
	const nextVersionQuery = `SELECT COALESCE(MAX(version), 0) + 1 FROM shared_key_versions WHERE user_id = $1`

	var version int
	if err := r.store.db.QueryRowContext(ctx, nextVersionQuery, userID).Scan(&version); err != nil {
		return domain.SharedKeyVersion{}, fmt.Errorf("next shared key version: %w", err)
	}

	item := domain.SharedKeyVersion{
		Version:      version,
		PublicKeyPEM: publicKeyPEM,
		Status:       domain.SharedKeyVersionStatusActive,
		CreatedAt:    time.Now().UTC(),
	}

	const insertQuery = `INSERT INTO shared_key_versions (
		user_id, version, public_key_pem, status, created_at, revoked_at
	) VALUES ($1, $2, $3, $4, $5, $6)`

	_, err := r.store.db.ExecContext(ctx, insertQuery,
		userID,
		item.Version,
		item.PublicKeyPEM,
		string(item.Status),
		item.CreatedAt,
		nullTimePtr(item.RevokedAt),
	)
	if err != nil {
		return domain.SharedKeyVersion{}, fmt.Errorf("insert shared key version: %w", err)
	}

	return item, nil
}

func (r *SharedKeyRepository) Revoke(ctx context.Context, userID string, version int) (domain.SharedKeyVersion, error) {
	now := time.Now().UTC()
	const query = `UPDATE shared_key_versions
	SET status = $1, revoked_at = $2
	WHERE user_id = $3 AND version = $4
	RETURNING version, public_key_pem, status, created_at, revoked_at`

	var item domain.SharedKeyVersion
	var status string
	var revokedAt sql.NullTime

	err := r.store.db.QueryRowContext(ctx, query,
		string(domain.SharedKeyVersionStatusRevoked),
		now,
		userID,
		version,
	).Scan(
		&item.Version,
		&item.PublicKeyPEM,
		&status,
		&item.CreatedAt,
		&revokedAt,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return domain.SharedKeyVersion{}, ErrNotFound
		}
		return domain.SharedKeyVersion{}, fmt.Errorf("revoke shared key: %w", err)
	}

	item.Status = domain.SharedKeyVersionStatus(status)
	if revokedAt.Valid {
		item.RevokedAt = &revokedAt.Time
	}

	return item, nil
}

func (r *SharedKeyRepository) List(ctx context.Context, userID string) ([]domain.SharedKeyVersion, error) {
	const query = `SELECT version, public_key_pem, status, created_at, revoked_at
	FROM shared_key_versions
	WHERE user_id = $1
	ORDER BY version DESC`

	rows, err := r.store.db.QueryContext(ctx, query, userID)
	if err != nil {
		return nil, fmt.Errorf("list shared keys: %w", err)
	}
	defer rows.Close()

	items := make([]domain.SharedKeyVersion, 0)
	for rows.Next() {
		var item domain.SharedKeyVersion
		var status string
		var revokedAt sql.NullTime
		if scanErr := rows.Scan(&item.Version, &item.PublicKeyPEM, &status, &item.CreatedAt, &revokedAt); scanErr != nil {
			return nil, fmt.Errorf("scan shared key: %w", scanErr)
		}
		item.Status = domain.SharedKeyVersionStatus(status)
		if revokedAt.Valid {
			item.RevokedAt = &revokedAt.Time
		}
		items = append(items, item)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate shared keys: %w", err)
	}

	return items, nil
}

func (r *SharedKeyRepository) Get(ctx context.Context, userID string, version int) (*domain.SharedKeyVersion, error) {
	const query = `SELECT version, public_key_pem, status, created_at, revoked_at
	FROM shared_key_versions
	WHERE user_id = $1 AND version = $2`

	var item domain.SharedKeyVersion
	var status string
	var revokedAt sql.NullTime

	err := r.store.db.QueryRowContext(ctx, query, userID, version).Scan(
		&item.Version,
		&item.PublicKeyPEM,
		&status,
		&item.CreatedAt,
		&revokedAt,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("get shared key: %w", err)
	}

	item.Status = domain.SharedKeyVersionStatus(status)
	if revokedAt.Valid {
		item.RevokedAt = &revokedAt.Time
	}

	return &item, nil
}

type DeviceKeyRepository struct{ store *Store }

func NewDeviceKeyRepository(store *Store) *DeviceKeyRepository {
	return &DeviceKeyRepository{store: store}
}

func (r *DeviceKeyRepository) RegisterWrappedSharedKey(
	ctx context.Context,
	userID string,
	data domain.DeviceWrappedSharedKey,
) (domain.DeviceWrappedSharedKey, error) {
	const query = `INSERT INTO device_wrapped_shared_keys (
		user_id, device_id, shared_key_version, encrypted_shared_private_key, created_at
	) VALUES ($1, $2, $3, $4, $5)
	ON CONFLICT (user_id, device_id)
	DO UPDATE SET
		shared_key_version = EXCLUDED.shared_key_version,
		encrypted_shared_private_key = EXCLUDED.encrypted_shared_private_key,
		created_at = EXCLUDED.created_at`

	_, err := r.store.db.ExecContext(ctx, query,
		userID,
		data.DeviceID,
		data.SharedKeyVersion,
		data.EncryptedSharedPrivateKey,
		data.CreatedAt,
	)
	if err != nil {
		return domain.DeviceWrappedSharedKey{}, fmt.Errorf("register device wrapped key: %w", err)
	}

	return data, nil
}

func (r *DeviceKeyRepository) ListWrappedSharedKeys(ctx context.Context, userID string) ([]domain.DeviceWrappedSharedKey, error) {
	const query = `SELECT device_id, shared_key_version, encrypted_shared_private_key, created_at
	FROM device_wrapped_shared_keys
	WHERE user_id = $1
	ORDER BY created_at DESC`

	rows, err := r.store.db.QueryContext(ctx, query, userID)
	if err != nil {
		return nil, fmt.Errorf("list device wrapped keys: %w", err)
	}
	defer rows.Close()

	items := make([]domain.DeviceWrappedSharedKey, 0)
	for rows.Next() {
		var item domain.DeviceWrappedSharedKey
		if scanErr := rows.Scan(
			&item.DeviceID,
			&item.SharedKeyVersion,
			&item.EncryptedSharedPrivateKey,
			&item.CreatedAt,
		); scanErr != nil {
			return nil, fmt.Errorf("scan device wrapped key: %w", scanErr)
		}
		items = append(items, item)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate device wrapped keys: %w", err)
	}

	return items, nil
}

type UploadSessionRepository struct{ store *Store }

func NewUploadSessionRepository(store *Store) *UploadSessionRepository {
	return &UploadSessionRepository{store: store}
}

func (r *UploadSessionRepository) Create(ctx context.Context, session domain.UploadSession) error {
	const query = `INSERT INTO upload_sessions (
		id, owner_user, object_key, upload_url, expires_at, created_at
	) VALUES ($1, $2, $3, $4, $5, $6)`

	_, err := r.store.db.ExecContext(ctx, query,
		session.ID,
		session.OwnerUser,
		session.ObjectKey,
		session.UploadURL,
		session.ExpiresAt,
		session.CreatedAt,
	)
	if err != nil {
		return fmt.Errorf("create upload session: %w", err)
	}

	return nil
}

func (r *UploadSessionRepository) GetByID(ctx context.Context, id string) (*domain.UploadSession, error) {
	const query = `SELECT id, owner_user, object_key, upload_url, expires_at, created_at
	FROM upload_sessions
	WHERE id = $1`

	var item domain.UploadSession
	err := r.store.db.QueryRowContext(ctx, query, id).Scan(
		&item.ID,
		&item.OwnerUser,
		&item.ObjectKey,
		&item.UploadURL,
		&item.ExpiresAt,
		&item.CreatedAt,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("get upload session: %w", err)
	}

	return &item, nil
}

func (r *UploadSessionRepository) Delete(ctx context.Context, id string) error {
	const query = `DELETE FROM upload_sessions WHERE id = $1`

	if _, err := r.store.db.ExecContext(ctx, query, id); err != nil {
		return fmt.Errorf("delete upload session: %w", err)
	}

	return nil
}

type EncodingProgressRepository struct{ store *Store }

func NewEncodingProgressRepository(store *Store) *EncodingProgressRepository {
	return &EncodingProgressRepository{store: store}
}

func (r *EncodingProgressRepository) Set(ctx context.Context, progress domain.VideoEncodingProgress) error {
	const query = `INSERT INTO encoding_progress (
		video_id, owner_user, status, percent, updated_at, message
	) VALUES ($1, $2, $3, $4, $5, $6)
	ON CONFLICT (video_id)
	DO UPDATE SET
		owner_user = EXCLUDED.owner_user,
		status = EXCLUDED.status,
		percent = EXCLUDED.percent,
		updated_at = EXCLUDED.updated_at,
		message = EXCLUDED.message`

	_, err := r.store.db.ExecContext(ctx, query,
		progress.VideoID,
		progress.OwnerUser,
		string(progress.Status),
		progress.Percent,
		progress.UpdatedAt,
		nullStringPtr(progress.Message),
	)
	if err != nil {
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
	const query = `SELECT video_id, owner_user, status, percent, updated_at, message
	FROM encoding_progress
	WHERE video_id = $1`

	var item domain.VideoEncodingProgress
	var status string
	var message sql.NullString

	err := r.store.db.QueryRowContext(ctx, query, videoID).Scan(
		&item.VideoID,
		&item.OwnerUser,
		&status,
		&item.Percent,
		&item.UpdatedAt,
		&message,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("get encoding progress: %w", err)
	}

	item.Status = domain.VideoStatus(status)
	if message.Valid {
		item.Message = &message.String
	}

	return &item, nil
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

type nullablePlaybackValue struct {
	ManifestURL      sql.NullString
	ExpiresAt        sql.NullTime
	Algorithm        sql.NullString
	KeyVersion       sql.NullInt64
	Nonce            []byte
	EncryptedDataKey []byte
}

func nullablePlayback(playback *domain.PlaybackGrant) nullablePlaybackValue {
	if playback == nil {
		return nullablePlaybackValue{}
	}

	return nullablePlaybackValue{
		ManifestURL:      sql.NullString{String: playback.ManifestURL, Valid: true},
		ExpiresAt:        sql.NullTime{Time: playback.ExpiresAt, Valid: true},
		Algorithm:        sql.NullString{String: string(playback.Encryption.Algorithm), Valid: true},
		KeyVersion:       sql.NullInt64{Int64: int64(playback.Encryption.KeyVersion), Valid: true},
		Nonce:            playback.Encryption.Nonce,
		EncryptedDataKey: playback.Encryption.EncryptedDataKey,
	}
}

type nullableEncryptionValue struct {
	Algorithm        sql.NullString
	KeyVersion       sql.NullInt64
	Nonce            []byte
	EncryptedDataKey []byte
}

func nullableEncryption(value *domain.EncryptionMetadata) nullableEncryptionValue {
	if value == nil {
		return nullableEncryptionValue{}
	}

	return nullableEncryptionValue{
		Algorithm:        sql.NullString{String: string(value.Algorithm), Valid: true},
		KeyVersion:       sql.NullInt64{Int64: int64(value.KeyVersion), Valid: true},
		Nonce:            value.Nonce,
		EncryptedDataKey: value.EncryptedDataKey,
	}
}

type scanner interface {
	Scan(dest ...any) error
}

func scanVideo(row scanner) (*domain.Video, error) {
	var item domain.Video

	var status string
	var readyAt sql.NullTime
	var failedReason sql.NullString
	var duration sql.NullInt64
	var width sql.NullInt64
	var height sql.NullInt64

	var playbackManifest sql.NullString
	var playbackExpiresAt sql.NullTime
	var playbackAlg sql.NullString
	var playbackKeyVersion sql.NullInt64
	var playbackNonce []byte
	var playbackEncryptedDataKey []byte

	var tagAlgorithm string
	var tagKeyVersion int
	var tagNonce []byte
	var tagEncryptedDataKey []byte

	var contentAlg sql.NullString
	var contentKeyVersion sql.NullInt64
	var contentNonce []byte
	var contentEncryptedDataKey []byte

	err := row.Scan(
		&item.ID,
		&item.OwnerUserID,
		&status,
		&item.UploadedAt,
		&readyAt,
		&failedReason,
		&duration,
		&width,
		&height,
		&playbackManifest,
		&playbackExpiresAt,
		&playbackAlg,
		&playbackKeyVersion,
		&playbackNonce,
		&playbackEncryptedDataKey,
		&item.EncryptedTags,
		&tagAlgorithm,
		&tagKeyVersion,
		&tagNonce,
		&tagEncryptedDataKey,
		&contentAlg,
		&contentKeyVersion,
		&contentNonce,
		&contentEncryptedDataKey,
		&item.CreatedAt,
		&item.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}

	item.Status = domain.VideoStatus(status)
	if readyAt.Valid {
		item.ReadyAt = &readyAt.Time
	}
	if failedReason.Valid {
		item.FailedReason = &failedReason.String
	}
	if duration.Valid {
		value := int(duration.Int64)
		item.DurationMillis = &value
	}
	if width.Valid {
		value := int(width.Int64)
		item.Width = &value
	}
	if height.Valid {
		value := int(height.Int64)
		item.Height = &value
	}

	item.TagEncryption = domain.EncryptionMetadata{
		Algorithm:        domain.EncryptionAlgorithm(tagAlgorithm),
		KeyVersion:       tagKeyVersion,
		Nonce:            tagNonce,
		EncryptedDataKey: tagEncryptedDataKey,
	}

	if playbackManifest.Valid && playbackExpiresAt.Valid && playbackAlg.Valid && playbackKeyVersion.Valid {
		item.Playback = &domain.PlaybackGrant{
			VideoID:     item.ID,
			ManifestURL: playbackManifest.String,
			ExpiresAt:   playbackExpiresAt.Time,
			Encryption: domain.EncryptionMetadata{
				Algorithm:        domain.EncryptionAlgorithm(playbackAlg.String),
				KeyVersion:       int(playbackKeyVersion.Int64),
				Nonce:            playbackNonce,
				EncryptedDataKey: playbackEncryptedDataKey,
			},
		}
	}

	if contentAlg.Valid && contentKeyVersion.Valid {
		item.ContentEncryption = &domain.EncryptionMetadata{
			Algorithm:        domain.EncryptionAlgorithm(contentAlg.String),
			KeyVersion:       int(contentKeyVersion.Int64),
			Nonce:            contentNonce,
			EncryptedDataKey: contentEncryptedDataKey,
		}
	}

	return &item, nil
}

func nullStringPtr(value *string) sql.NullString {
	if value == nil {
		return sql.NullString{}
	}

	return sql.NullString{String: *value, Valid: true}
}

func nullIntPtr(value *int) sql.NullInt64 {
	if value == nil {
		return sql.NullInt64{}
	}

	return sql.NullInt64{Int64: int64(*value), Valid: true}
}

func nullTimePtr(value *time.Time) sql.NullTime {
	if value == nil {
		return sql.NullTime{}
	}

	return sql.NullTime{Time: *value, Valid: true}
}
