-- name: DeleteVideoTags :exec
DELETE FROM
    video_tags
WHERE
    video_id = @video_id;

-- name: InsertVideoTag :exec
INSERT INTO
    video_tags (video_id, tag, created_at)
VALUES
    (@video_id, @tag, @created_at);

-- name: GetVideoTags :many
SELECT
    tag
FROM
    video_tags
WHERE
    video_id = @video_id
ORDER BY
    tag;

-- name: GetVideoTagsBatch :many
SELECT
    video_id,
    tag
FROM
    video_tags
WHERE
    video_id = ANY(@video_ids :: text [])
ORDER BY
    video_id,
    tag;
