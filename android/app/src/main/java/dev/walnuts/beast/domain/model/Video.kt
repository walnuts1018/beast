package dev.walnuts.beast.domain.model

import kotlinx.serialization.Serializable

@Serializable
enum class VideoStatus { UPLOADED, ENCODING, READY, FAILED }

@Serializable
enum class PlaybackSource { ENCRYPTED_DASH, DEBUG_PREVIEW }

@Serializable
data class EncryptionMetadata(
    val algorithm: String,
    val chunkSize: Int,
    val keyVersion: String,
    val nonce: String,
    val encryptedDataKey: String,
    val sharedKeyId: String,
)

@Serializable
data class Video(
    val id: String,
    val status: VideoStatus,
    val tags: List<String>,
    val playCount: Int,
    val rating: Int?,
    val lastPlayedAt: String?,
    val encryption: EncryptionMetadata,
    val encryptedTags: String? = null,
    val thumbnailUrl: String? = null,
    val playbackSource: PlaybackSource = PlaybackSource.ENCRYPTED_DASH,
    val encryptedDashManifestUrl: String? = null,
    val playbackUrl: String? = null,
    val durationSeconds: Long? = null,
    val progress: Float = if (status == VideoStatus.READY) 1f else 0f,
)

enum class RecommendationKind(val label: String, val description: String) {
    FAVORITES("あなたのお気に入り", "高く評価した動画"),
    RETURN_TO_FAVORITES("最近再生していないお気に入り", "少し時間が空いた動画"),
    UNWATCHED("まだ再生していない動画", "新しい動画を見つける"),
    RECENT("最近見た動画", "続きを楽しむ"),
}

fun Video.isFavorite() = rating != null && rating >= 4

fun Video.recommendationKinds(nowEpochSeconds: Long): Set<RecommendationKind> = buildSet {
    if (isFavorite()) add(RecommendationKind.FAVORITES)
    val lastPlayedEpoch = lastPlayedAt?.toLongOrNull()
    if (isFavorite() && (lastPlayedEpoch == null || nowEpochSeconds - lastPlayedEpoch > 60 * 60 * 24 * 14)) {
        add(RecommendationKind.RETURN_TO_FAVORITES)
    }
    if (playCount == 0) add(RecommendationKind.UNWATCHED)
    if (lastPlayedEpoch != null && nowEpochSeconds - lastPlayedEpoch <= 60 * 60 * 24 * 7) {
        add(RecommendationKind.RECENT)
    }
}
