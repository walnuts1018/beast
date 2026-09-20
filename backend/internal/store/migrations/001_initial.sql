CREATE TABLE IF NOT EXISTS shared_keys (
    id uuid PRIMARY KEY,
    owner_id text NOT NULL,
    version text NOT NULL,
    public_key text NOT NULL,
    status text NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'retired', 'revoked')),
    created_at timestamptz NOT NULL DEFAULT now(),
    retired_at timestamptz,
    UNIQUE (owner_id, version)
);

CREATE TABLE IF NOT EXISTS device_keys (
    id uuid PRIMARY KEY,
    owner_id text NOT NULL,
    device_id text NOT NULL,
    shared_key_id uuid NOT NULL REFERENCES shared_keys(id),
    encrypted_shared_private_key bytea NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    revoked_at timestamptz,
    UNIQUE (owner_id, device_id, shared_key_id)
);

CREATE TABLE IF NOT EXISTS videos (
    id uuid PRIMARY KEY,
    owner_id text NOT NULL,
    status text NOT NULL CHECK (status IN ('UPLOADED', 'ENCODING', 'READY', 'FAILED')),
    object_key text NOT NULL,
    tags_ciphertext bytea NOT NULL,
    tags_nonce bytea NOT NULL,
    encrypted_data_key bytea NOT NULL,
    encryption_algorithm text NOT NULL,
    chunk_size integer NOT NULL DEFAULT 1048576 CHECK (chunk_size > 0),
    encryption_key_version text NOT NULL,
    shared_key_id uuid NOT NULL REFERENCES shared_keys(id),
    play_count bigint NOT NULL DEFAULT 0 CHECK (play_count >= 0),
    rating smallint CHECK (rating IS NULL OR rating BETWEEN 1 AND 5),
    last_played_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);

ALTER TABLE videos ADD COLUMN IF NOT EXISTS chunk_size integer NOT NULL DEFAULT 1048576;

CREATE INDEX IF NOT EXISTS videos_owner_created_at_idx ON videos (owner_id, created_at DESC);
CREATE INDEX IF NOT EXISTS videos_owner_rating_idx ON videos (owner_id, rating DESC NULLS LAST);
