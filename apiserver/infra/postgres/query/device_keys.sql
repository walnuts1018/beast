-- name: UpsertDeviceWrappedSharedKey :exec
INSERT INTO
    device_wrapped_shared_keys (
        user_id,
        device_id,
        device_public_key_pem,
        shared_key_version,
        encrypted_shared_private_key,
        created_at
    )
VALUES
    (
        @user_id,
        @device_id,
        @device_public_key_pem,
        @shared_key_version,
        @encrypted_shared_private_key,
        @created_at
    ) ON CONFLICT (user_id, device_id) DO
UPDATE
SET
    device_public_key_pem = EXCLUDED.device_public_key_pem,
    shared_key_version = EXCLUDED.shared_key_version,
    encrypted_shared_private_key = EXCLUDED.encrypted_shared_private_key,
    created_at = EXCLUDED.created_at;

-- name: ListDeviceWrappedSharedKeys :many
SELECT
    device_id,
    device_public_key_pem,
    shared_key_version,
    encrypted_shared_private_key,
    created_at
FROM
    device_wrapped_shared_keys
WHERE
    user_id = @user_id
ORDER BY
    created_at DESC;
