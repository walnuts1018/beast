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
        encrypted_tags,
        tag_enc_algorithm,
        tag_enc_key_version,
        tag_enc_nonce,
        tag_enc_encrypted_data_key,
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
        @encrypted_tags,
        @tag_enc_algorithm,
        @tag_enc_key_version,
        @tag_enc_nonce,
        @tag_enc_encrypted_data_key,
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
    encrypted_tags,
    tag_enc_algorithm,
    tag_enc_key_version,
    tag_enc_nonce,
    tag_enc_encrypted_data_key,
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
    encrypted_tags,
    tag_enc_algorithm,
    tag_enc_key_version,
    tag_enc_nonce,
    tag_enc_encrypted_data_key,
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
    encrypted_tags = @encrypted_tags,
    tag_enc_algorithm = @tag_enc_algorithm,
    tag_enc_key_version = @tag_enc_key_version,
    tag_enc_nonce = @tag_enc_nonce,
    tag_enc_encrypted_data_key = @tag_enc_encrypted_data_key,
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
