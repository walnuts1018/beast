-- name: GetNextSharedKeyVersion :one
SELECT
    COALESCE(MAX(version), 0) + 1 AS next_version
FROM
    shared_key_versions
WHERE
    user_id = @user_id;

-- name: InsertSharedKeyVersion :exec
INSERT INTO
    shared_key_versions (
        user_id,
        version,
        public_key_pem,
        status,
        created_at,
        revoked_at
    )
VALUES
    (
        @user_id,
        @version,
        @public_key_pem,
        @status,
        @created_at,
        @revoked_at
    );

-- name: RevokeSharedKeyVersion :one
UPDATE
    shared_key_versions
SET
    status = @status,
    revoked_at = @revoked_at
WHERE
    user_id = @user_id
    AND version = @version RETURNING version,
    public_key_pem,
    status,
    created_at,
    revoked_at;

-- name: ListSharedKeyVersions :many
SELECT
    version,
    public_key_pem,
    status,
    created_at,
    revoked_at
FROM
    shared_key_versions
WHERE
    user_id = @user_id
ORDER BY
    version DESC;

-- name: GetSharedKeyVersion :one
SELECT
    version,
    public_key_pem,
    status,
    created_at,
    revoked_at
FROM
    shared_key_versions
WHERE
    user_id = @user_id
    AND version = @version;
