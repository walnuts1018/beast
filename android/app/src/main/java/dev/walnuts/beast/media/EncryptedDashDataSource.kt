package dev.walnuts.beast.media

import android.net.Uri
import androidx.media3.common.C
import androidx.media3.datasource.DataSource
import androidx.media3.datasource.DataSpec
import androidx.media3.datasource.TransferListener
import dev.walnuts.beast.security.DataKeyEnvelope
import dev.walnuts.beast.security.DeviceKeySession
import dev.walnuts.beast.security.VideoDataKeyDecryptor
import java.io.IOException
import okhttp3.OkHttpClient
import okhttp3.Request
import okhttp3.Headers
import java.util.Base64

class EncryptedDashDataSource(
    private val client: OkHttpClient,
    private val accessTokenProvider: () -> String?,
    private val deviceKeySession: DeviceKeySession,
    private val decryptor: VideoDataKeyDecryptor,
) : DataSource {
    private var uri: Uri? = null
    private var plainBytes = ByteArray(0)
    private var position = 0
    private var bytesRemaining = 0L

    override fun addTransferListener(transferListener: TransferListener) = Unit

    override fun open(dataSpec: DataSpec): Long {
        val token = accessTokenProvider()
            ?: throw IOException("ログインセッションがありません")
        val request = Request.Builder().url(dataSpec.uri.toString()).header("Authorization", "Bearer $token").get().build()
        val response = try {
            client.newCall(request).execute()
        } catch (error: IOException) {
            throw error
        }
        response.use {
            if (!it.isSuccessful) throw IOException("DASH artifactの取得に失敗しました: HTTP ${it.code}")
            val encrypted = it.body?.bytes() ?: throw IOException("DASH artifactの本文が空です")
            val metadata = it.headers.toDataKeyEnvelope()
            val sharedPrivateKey = deviceKeySession.privateKeyFor(metadata.sharedKeyId)
            plainBytes = try {
                decryptor.decryptArtifact(encrypted, metadata.envelope, sharedPrivateKey)
            } finally {
                sharedPrivateKey.fill(0)
            }
        }
        val start = dataSpec.position
        if (start > plainBytes.size) throw IOException("DASH artifactの読み込み位置が不正です")
        position = start.toInt()
        bytesRemaining = if (dataSpec.length == C.LENGTH_UNSET.toLong()) plainBytes.size - position.toLong() else dataSpec.length.coerceAtMost(plainBytes.size - position.toLong())
        uri = dataSpec.uri
        return bytesRemaining
    }

    override fun read(buffer: ByteArray, offset: Int, length: Int): Int {
        if (bytesRemaining == 0L) return C.RESULT_END_OF_INPUT
        val readLength = minOf(length.toLong(), bytesRemaining).toInt()
        plainBytes.copyInto(buffer, offset, position, position + readLength)
        position += readLength
        bytesRemaining -= readLength
        return readLength
    }

    override fun getUri(): Uri? = uri

    override fun getResponseHeaders(): Map<String, List<String>> = emptyMap()

    override fun close() {
        plainBytes.fill(0)
        plainBytes = ByteArray(0)
        position = 0
        bytesRemaining = 0
        uri = null
    }

    class Factory(
        private val client: OkHttpClient,
        private val accessTokenProvider: () -> String?,
        private val deviceKeySession: DeviceKeySession,
        private val decryptor: VideoDataKeyDecryptor,
    ) : DataSource.Factory {
        override fun createDataSource(): DataSource = EncryptedDashDataSource(client, accessTokenProvider, deviceKeySession, decryptor)
    }
}

private data class ArtifactEncryption(
    val sharedKeyId: String,
    val envelope: DataKeyEnvelope,
)

private fun Headers.toDataKeyEnvelope(): ArtifactEncryption {
    val algorithm = get("X-Encryption-Algorithm") ?: throw IOException("暗号化アルゴリズムがありません")
    val chunkSize = get("X-Encryption-Chunk-Size")?.toIntOrNull() ?: throw IOException("暗号化チャンクサイズがありません")
    val keyVersion = get("X-Encryption-Key-Version") ?: throw IOException("暗号化鍵バージョンがありません")
    val nonce = get("X-Encryption-Nonce")?.decodeBase64() ?: throw IOException("暗号化nonceがありません")
    val encryptedDataKey = get("X-Encryption-Data-Key")?.decodeBase64() ?: throw IOException("暗号化Data Keyがありません")
    val sharedKeyId = get("X-Encryption-Shared-Key-ID") ?: throw IOException("Shared Key IDがありません")
    return ArtifactEncryption(sharedKeyId, DataKeyEnvelope(algorithm, chunkSize, keyVersion, nonce, encryptedDataKey))
}

private fun String.decodeBase64(): ByteArray = runCatching { Base64.getDecoder().decode(this) }.getOrElse { throw IOException("暗号化メタデータのbase64形式が不正です") }
