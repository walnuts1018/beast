-- レーティングカラムをvideosテーブルに追加（NULL=未評価、1-5=星の数）
ALTER TABLE
    videos
ADD
    COLUMN rating INTEGER CHECK (
        rating >= 1
        AND rating <= 5
    );

-- 再生回数と最終再生日時をvideosテーブルに追加（おすすめ機能用の非正規化フィールド）
ALTER TABLE
    videos
ADD
    COLUMN play_count INTEGER NOT NULL DEFAULT 0;

ALTER TABLE
    videos
ADD
    COLUMN last_played_at TIMESTAMPTZ;

-- 再生履歴テーブル
CREATE TABLE IF NOT EXISTS playback_histories (
    id TEXT PRIMARY KEY,
    video_id TEXT NOT NULL REFERENCES videos(id) ON DELETE CASCADE,
    owner_user_id TEXT NOT NULL,
    played_at TIMESTAMPTZ NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_playback_histories_video_id ON playback_histories(video_id, played_at DESC);

CREATE INDEX IF NOT EXISTS idx_playback_histories_owner ON playback_histories(owner_user_id, played_at DESC);
