DROP INDEX IF EXISTS idx_playback_histories_owner;

DROP INDEX IF EXISTS idx_playback_histories_video_id;

DROP TABLE IF EXISTS playback_histories;

ALTER TABLE
    videos DROP COLUMN IF EXISTS last_played_at;

ALTER TABLE
    videos DROP COLUMN IF EXISTS play_count;

ALTER TABLE
    videos DROP COLUMN IF EXISTS rating;
