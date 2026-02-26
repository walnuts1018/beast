-- name: CreateUploadSession :exec
INSERT INTO
    upload_sessions (
        id,
        owner_user,
        object_key,
        upload_url,
        expires_at,
        created_at
    )
VALUES
    (
        @id,
        @owner_user,
        @object_key,
        @upload_url,
        @expires_at,
        @created_at
    );

-- name: GetUploadSessionByID :one
SELECT
    id,
    owner_user,
    object_key,
    upload_url,
    expires_at,
    created_at
FROM
    upload_sessions
WHERE
    id = @id;

-- name: DeleteUploadSession :exec
DELETE FROM
    upload_sessions
WHERE
    id = @id;
