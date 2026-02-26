-- このマイグレーションはフレッシュデータベースを前提としています。
-- 既存の動画データがある場合、source_object_keyには空文字列が設定されます。
-- 既存環境で実行する場合で、別の値を設定したい場合は、
-- マイグレーション実行後に適切な値へ更新してください。
ALTER TABLE
    videos
ADD
    COLUMN source_object_key TEXT NOT NULL DEFAULT '';

ALTER TABLE
    videos
ADD
    COLUMN encoded_object_key TEXT;
