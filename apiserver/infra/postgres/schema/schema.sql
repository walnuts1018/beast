CREATE TABLE IF NOT EXISTS shared_key_versions (
    user_id TEXT NOT NULL,
    version INTEGER NOT NULL,
    public_key_pem TEXT NOT NULL,
    status TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL,
    revoked_at TIMESTAMPTZ,
    PRIMARY KEY (user_id, version)
);

CREATE TABLE IF NOT EXISTS device_wrapped_shared_keys (
    user_id TEXT NOT NULL,
    device_id TEXT NOT NULL,
    device_public_key_pem TEXT NOT NULL,
    shared_key_version INTEGER NOT NULL,
    encrypted_shared_private_key BYTEA NOT NULL,
    created_at TIMESTAMPTZ NOT NULL,
    PRIMARY KEY (user_id, device_id)
);

CREATE TABLE IF NOT EXISTS upload_sessions (
    id TEXT PRIMARY KEY,
    owner_user TEXT NOT NULL,
    object_key TEXT NOT NULL,
    upload_url TEXT NOT NULL,
    expires_at TIMESTAMPTZ NOT NULL,
    created_at TIMESTAMPTZ NOT NULL
);

CREATE TABLE IF NOT EXISTS videos (
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
);

CREATE INDEX IF NOT EXISTS videos_owner_uploaded_idx ON videos(owner_user_id, uploaded_at DESC, id DESC);

CREATE TABLE IF NOT EXISTS encoding_progress (
    video_id TEXT PRIMARY KEY,
    owner_user TEXT NOT NULL,
    status TEXT NOT NULL,
    percent DOUBLE PRECISION NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL,
    message TEXT
);
