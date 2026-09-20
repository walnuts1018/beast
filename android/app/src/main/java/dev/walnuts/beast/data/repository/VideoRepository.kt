package dev.walnuts.beast.data.repository

import dev.walnuts.beast.data.api.GraphQlOperation
import dev.walnuts.beast.data.api.GraphQlTransport
import dev.walnuts.beast.data.api.EncryptedTagsDecoder
import dev.walnuts.beast.data.api.NoopEncryptedTagsDecoder
import dev.walnuts.beast.data.api.VideoGraphQlOperations
import dev.walnuts.beast.data.api.toVideo
import dev.walnuts.beast.domain.model.EncryptionMetadata
import dev.walnuts.beast.domain.model.Video
import dev.walnuts.beast.domain.model.VideoStatus
import kotlinx.coroutines.delay
import kotlinx.serialization.json.buildJsonObject
import kotlinx.serialization.json.JsonNull
import kotlinx.serialization.json.jsonArray
import kotlinx.serialization.json.jsonObject
import kotlinx.serialization.json.put

interface VideoRepository {
    suspend fun listVideos(): List<Video>
    suspend fun setRating(videoId: String, rating: Int?): Video
    suspend fun recordPlayback(videoId: String): Video
}

class GraphQlVideoRepository(
    private val transport: GraphQlTransport,
    private val accessTokenProvider: suspend () -> String?,
    private val encryptedTagsDecoder: EncryptedTagsDecoder = NoopEncryptedTagsDecoder,
) : VideoRepository {
    override suspend fun listVideos(): List<Video> {
        val token = requireToken()
        return transport.execute(GraphQlOperation("ListVideos", VideoGraphQlOperations.listVideos), token)
            .getOrThrow().jsonObject["videos"]!!.jsonArray.map { it.toVideo() }.map { video ->
                video.copy(tags = encryptedTagsDecoder.decode(video))
            }
    }

    override suspend fun setRating(videoId: String, rating: Int?): Video {
        val variables = buildJsonObject { put("id", videoId); if (rating == null) put("rating", JsonNull) else put("rating", rating) }
        return executeVideoMutation("RateVideo", "rateVideo", VideoGraphQlOperations.rateVideo, variables, videoId)
    }

    override suspend fun recordPlayback(videoId: String): Video = executeVideoMutation(
        "RecordPlayback", "recordPlayback", VideoGraphQlOperations.recordPlayback, buildJsonObject { put("id", videoId) }, videoId,
    )

    private suspend fun executeVideoMutation(name: String, fieldName: String, query: String, variables: kotlinx.serialization.json.JsonObject, videoId: String): Video {
        val result = transport.execute(GraphQlOperation(name, query, variables), requireToken()).getOrThrow().jsonObject
        return result[fieldName]?.toVideo() ?: error("$name did not return video $videoId")
    }

    private suspend fun requireToken(): String = accessTokenProvider() ?: error("ログインセッションがありません")
}

class PreviewVideoRepository : VideoRepository {
    private val previewPlaybackUrl = "https://storage.googleapis.com/gtv-videos-bucket/sample/ForBiggerEscapes.mp4"
    private var videos = listOf(
        preview("video-01", VideoStatus.READY, listOf("旅", "夕方"), 4, 5, "1710000000", "shared-01", 623),
        preview("video-02", VideoStatus.READY, listOf("散歩", "街"), 0, null, null, "shared-01", 218),
        preview("video-03", VideoStatus.READY, listOf("料理", "週末"), 9, 4, "1708000000", "shared-01", 496),
        preview("video-04", VideoStatus.ENCODING, listOf("旅行"), 0, null, null, "shared-02", 1280).copy(playbackUrl = null),
        preview("video-05", VideoStatus.READY, listOf("猫", "日常"), 2, 3, "1710200000", "shared-01", 92),
    )

    override suspend fun listVideos(): List<Video> {
        delay(180)
        return videos
    }

    override suspend fun setRating(videoId: String, rating: Int?): Video {
        delay(120)
        return update(videoId) { it.copy(rating = rating) }
    }

    override suspend fun recordPlayback(videoId: String): Video {
        delay(120)
        return update(videoId) { it.copy(playCount = it.playCount + 1, lastPlayedAt = (System.currentTimeMillis() / 1000).toString()) }
    }

    private fun update(videoId: String, transform: (Video) -> Video): Video {
        val updated = videos.first { it.id == videoId }.let(transform)
        videos = videos.map { if (it.id == videoId) updated else it }
        return updated
    }

    private fun preview(id: String, status: VideoStatus, tags: List<String>, playCount: Int, rating: Int?, lastPlayedAt: String?, sharedKeyId: String, durationSeconds: Long) = Video(
        id = id,
        status = status,
        tags = tags,
        playCount = playCount,
        rating = rating,
        lastPlayedAt = lastPlayedAt,
        encryption = EncryptionMetadata("AES-GCM", 1_048_576, "v1", "nonce-preview", "encrypted-data-key", sharedKeyId),
        playbackUrl = previewPlaybackUrl,
        durationSeconds = durationSeconds,
    )
}
