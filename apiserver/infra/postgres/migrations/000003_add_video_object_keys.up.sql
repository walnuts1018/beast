-- このマイグレーションはフレッシュデータベースを前提としています。
-- 既存の動画データがある場合、source_object_keyがNULLとなります。
-- 既存環境で実行する場合は、事前に既存動画のsource_object_keyを
-- 手動で設定してください。
ALTER TABLE
    videos
ADD
    COLUMN source_object_key TEXT;

ALTER TABLE
    videos
ADD
    COLUMN encoded_object_key TEXT;
