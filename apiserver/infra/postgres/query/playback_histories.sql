-- name: InsertPlaybackHistory :exec
INSERT INTO
    playback_histories (id, video_id, owner_user_id, played_at)
VALUES
    (@id, @video_id, @owner_user_id, @played_at);

-- name: ListPlaybackHistoriesByVideo :many
SELECT
    id,
    video_id,
    owner_user_id,
    played_at
FROM
    playback_histories
WHERE
    video_id = @video_id
ORDER BY
    played_at DESC
LIMIT
    @limit_count;
