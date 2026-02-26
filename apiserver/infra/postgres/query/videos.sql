-- name: CreateVideo :exec
INSERT INTO
    videos (
        id,
        owner_user_id,
        status,
        uploaded_at,
        ready_at,
        failed_reason,
        duration_millis,
        width,
        height,
        playback_manifest_url,
        playback_expires_at,
        playback_enc_algorithm,
        playback_enc_key_version,
        playback_enc_nonce,
        playback_enc_encrypted_data_key,
        content_enc_algorithm,
        content_enc_key_version,
        content_enc_nonce,
        content_enc_encrypted_data_key,
        created_at,
        updated_at
    )
VALUES
    (
        @id,
        @owner_user_id,
        @status,
        @uploaded_at,
        @ready_at,
        @failed_reason,
        @duration_millis,
        @width,
        @height,
        @playback_manifest_url,
        @playback_expires_at,
        @playback_enc_algorithm,
        @playback_enc_key_version,
        @playback_enc_nonce,
        @playback_enc_encrypted_data_key,
        @content_enc_algorithm,
        @content_enc_key_version,
        @content_enc_nonce,
        @content_enc_encrypted_data_key,
        @created_at,
        @updated_at
    );

-- name: GetVideoByID :one
SELECT
    id,
    owner_user_id,
    status,
    uploaded_at,
    ready_at,
    failed_reason,
    duration_millis,
    width,
    height,
    playback_manifest_url,
    playback_expires_at,
    playback_enc_algorithm,
    playback_enc_key_version,
    playback_enc_nonce,
    playback_enc_encrypted_data_key,
    content_enc_algorithm,
    content_enc_key_version,
    content_enc_nonce,
    content_enc_encrypted_data_key,
    created_at,
    updated_at
FROM
    videos
WHERE
    id = @id;

-- name: ListVideosByOwner :many
SELECT
    id,
    owner_user_id,
    status,
    uploaded_at,
    ready_at,
    failed_reason,
    duration_millis,
    width,
    height,
    playback_manifest_url,
    playback_expires_at,
    playback_enc_algorithm,
    playback_enc_key_version,
    playback_enc_nonce,
    playback_enc_encrypted_data_key,
    content_enc_algorithm,
    content_enc_key_version,
    content_enc_nonce,
    content_enc_encrypted_data_key,
    created_at,
    updated_at
FROM
    videos
WHERE
    owner_user_id = @owner_user_id
    AND (
        sqlc.narg('status') :: text IS NULL
        OR status = sqlc.narg('status')
    )
    AND (
        sqlc.narg('cursor_uploaded_at') :: timestamptz IS NULL
        OR (uploaded_at, id) < (
            sqlc.narg('cursor_uploaded_at'),
            sqlc.narg('cursor_id')::text
        )
    )
ORDER BY
    uploaded_at DESC,
    id DESC
LIMIT
    @limit_count;

-- name: ListVideosByOwnerAndTag :many
SELECT
    v.id,
    v.owner_user_id,
    v.status,
    v.uploaded_at,
    v.ready_at,
    v.failed_reason,
    v.duration_millis,
    v.width,
    v.height,
    v.playback_manifest_url,
    v.playback_expires_at,
    v.playback_enc_algorithm,
    v.playback_enc_key_version,
    v.playback_enc_nonce,
    v.playback_enc_encrypted_data_key,
    v.content_enc_algorithm,
    v.content_enc_key_version,
    v.content_enc_nonce,
    v.content_enc_encrypted_data_key,
    v.created_at,
    v.updated_at
FROM
    videos v
    INNER JOIN video_tags vt ON v.id = vt.video_id
WHERE
    v.owner_user_id = @owner_user_id
    AND vt.tag = @tag
    AND (
        sqlc.narg('status') :: text IS NULL
        OR v.status = sqlc.narg('status')
    )
    AND (
        sqlc.narg('cursor_uploaded_at') :: timestamptz IS NULL
        OR (v.uploaded_at, v.id) < (
            sqlc.narg('cursor_uploaded_at'),
            sqlc.narg('cursor_id')::text
        )
    )
ORDER BY
    v.uploaded_at DESC,
    v.id DESC
LIMIT
    @limit_count;

-- name: UpdateVideo :execrows
UPDATE
    videos
SET
    owner_user_id = @owner_user_id,
    status = @status,
    uploaded_at = @uploaded_at,
    ready_at = @ready_at,
    failed_reason = @failed_reason,
    duration_millis = @duration_millis,
    width = @width,
    height = @height,
    playback_manifest_url = @playback_manifest_url,
    playback_expires_at = @playback_expires_at,
    playback_enc_algorithm = @playback_enc_algorithm,
    playback_enc_key_version = @playback_enc_key_version,
    playback_enc_nonce = @playback_enc_nonce,
    playback_enc_encrypted_data_key = @playback_enc_encrypted_data_key,
    content_enc_algorithm = @content_enc_algorithm,
    content_enc_key_version = @content_enc_key_version,
    content_enc_nonce = @content_enc_nonce,
    content_enc_encrypted_data_key = @content_enc_encrypted_data_key,
    created_at = @created_at,
    updated_at = @updated_at
WHERE
    id = @id;

-- name: LoadVideoUploadedAt :one
SELECT
    uploaded_at
FROM
    videos
WHERE
    id = @id
    AND owner_user_id = @owner_user_id;
