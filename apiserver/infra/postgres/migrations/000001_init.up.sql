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
    content_enc_algorithm TEXT,
    content_enc_key_version INTEGER,
    content_enc_nonce BYTEA,
    content_enc_encrypted_data_key BYTEA,
    rating INTEGER CHECK (
        rating >= 1
        AND rating <= 5
    ),
    play_count INTEGER NOT NULL DEFAULT 0,
    last_played_at TIMESTAMPTZ,
    source_object_key TEXT NOT NULL,
    encoded_object_key TEXT,
    created_at TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL
);

CREATE INDEX IF NOT EXISTS videos_owner_uploaded_idx ON videos(owner_user_id, uploaded_at DESC, id DESC);

-- TODO: 将来的にサーバー側管理の対称鍵による決定性暗号化を導入し、タグを暗号化して保存する
CREATE TABLE IF NOT EXISTS video_tags (
    video_id TEXT NOT NULL REFERENCES videos(id) ON DELETE CASCADE,
    tag TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL,
    PRIMARY KEY (video_id, tag)
);

CREATE INDEX IF NOT EXISTS idx_video_tags_tag ON video_tags(tag);

CREATE TABLE IF NOT EXISTS encoding_progress (
    video_id TEXT PRIMARY KEY,
    owner_user TEXT NOT NULL,
    status TEXT NOT NULL,
    percent DOUBLE PRECISION NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL,
    message TEXT
);

CREATE TABLE IF NOT EXISTS playback_histories (
    id TEXT PRIMARY KEY,
    video_id TEXT NOT NULL REFERENCES videos(id) ON DELETE CASCADE,
    owner_user_id TEXT NOT NULL,
    played_at TIMESTAMPTZ NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_playback_histories_video_id ON playback_histories(video_id, played_at DESC);

CREATE INDEX IF NOT EXISTS idx_playback_histories_owner ON playback_histories(owner_user_id, played_at DESC);
