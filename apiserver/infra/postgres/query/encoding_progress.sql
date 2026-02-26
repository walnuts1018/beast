-- name: UpsertEncodingProgress :exec
INSERT INTO
    encoding_progress (
        video_id,
        owner_user,
        status,
        percent,
        updated_at,
        message
    )
VALUES
    (
        @video_id,
        @owner_user,
        @status,
        @percent,
        @updated_at,
        @message
    ) ON CONFLICT (video_id) DO
UPDATE
SET
    owner_user = EXCLUDED.owner_user,
    status = EXCLUDED.status,
    percent = EXCLUDED.percent,
    updated_at = EXCLUDED.updated_at,
    message = EXCLUDED.message;

-- name: GetEncodingProgress :one
SELECT
    video_id,
    owner_user,
    status,
    percent,
    updated_at,
    message
FROM
    encoding_progress
WHERE
    video_id = @video_id;
