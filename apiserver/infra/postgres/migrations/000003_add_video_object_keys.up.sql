ALTER TABLE
    videos
ADD
    COLUMN source_object_key TEXT NOT NULL DEFAULT '';

ALTER TABLE
    videos
ADD
    COLUMN encoded_object_key TEXT;
