CREATE TABLE IF NOT EXISTS videos (
    id uuid PRIMARY KEY,
    owner_id text NOT NULL,
	status text NOT NULL CHECK (status IN ('UPLOADED', 'ENCODING', 'READY', 'FAILED')),
	object_key text NOT NULL,
	source_object_key text NOT NULL DEFAULT '',
    tags_ciphertext bytea NOT NULL,
    tags_nonce bytea NOT NULL,
    encrypted_data_key bytea NOT NULL,
    encryption_algorithm text NOT NULL,
    chunk_size integer NOT NULL DEFAULT 1048576 CHECK (chunk_size > 0),
    encryption_key_version text NOT NULL,
    play_count bigint NOT NULL DEFAULT 0 CHECK (play_count >= 0),
    rating smallint CHECK (rating IS NULL OR rating BETWEEN 1 AND 5),
    last_played_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT now(),
	updated_at timestamptz NOT NULL DEFAULT now(),
	progress double precision NOT NULL DEFAULT 0 CHECK (progress BETWEEN 0 AND 1),
	error_message text NOT NULL DEFAULT '',
	hls_artifacts jsonb NOT NULL DEFAULT '{}'::jsonb
);

ALTER TABLE videos ADD COLUMN IF NOT EXISTS chunk_size integer NOT NULL DEFAULT 1048576;
ALTER TABLE videos ADD COLUMN IF NOT EXISTS source_object_key text NOT NULL DEFAULT '';
ALTER TABLE videos ADD COLUMN IF NOT EXISTS progress double precision NOT NULL DEFAULT 0;
ALTER TABLE videos ADD COLUMN IF NOT EXISTS error_message text NOT NULL DEFAULT '';
ALTER TABLE videos ADD COLUMN IF NOT EXISTS hls_artifacts jsonb NOT NULL DEFAULT '{}'::jsonb;

DO $$
BEGIN
    IF EXISTS (
        SELECT 1
        FROM information_schema.columns
        WHERE table_schema = current_schema()
          AND table_name = 'videos'
          AND column_name = 'dash_artifacts'
    ) AND NOT EXISTS (
        SELECT 1
        FROM information_schema.columns
        WHERE table_schema = current_schema()
          AND table_name = 'videos'
          AND column_name = 'hls_artifacts'
    ) THEN
        ALTER TABLE videos RENAME COLUMN dash_artifacts TO hls_artifacts;
    ELSIF EXISTS (
        SELECT 1
        FROM information_schema.columns
        WHERE table_schema = current_schema()
          AND table_name = 'videos'
          AND column_name = 'dash_artifacts'
    ) THEN
        UPDATE videos SET hls_artifacts = dash_artifacts WHERE hls_artifacts = '{}'::jsonb;
        ALTER TABLE videos DROP COLUMN dash_artifacts;
    END IF;
END
$$;

ALTER TABLE videos DROP COLUMN IF EXISTS shared_key_id;

CREATE INDEX IF NOT EXISTS videos_owner_created_at_idx ON videos (owner_id, created_at DESC);
CREATE INDEX IF NOT EXISTS videos_owner_rating_idx ON videos (owner_id, rating DESC NULLS LAST);
