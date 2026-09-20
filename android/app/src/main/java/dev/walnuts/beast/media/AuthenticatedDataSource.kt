package dev.walnuts.beast.media

import android.net.Uri
import androidx.media3.common.C
import androidx.media3.datasource.DataSource
import androidx.media3.datasource.DataSpec
import androidx.media3.datasource.TransferListener
import java.io.IOException
import okhttp3.OkHttpClient
import okhttp3.Request
import okhttp3.Response
import okio.BufferedSource

/**
 * 認証済みのHLSマニフェストとメディアセグメントをHTTPのまま読み込むDataSourceです。
 * Media3から渡されるRangeをそのままHTTP Rangeへ変換し、シーク時に動画全体をメモリへ読み込みません。
 */
class AuthenticatedDataSource(
    private val client: OkHttpClient,
    private val accessTokenProvider: () -> String?,
) : DataSource {
    private var response: Response? = null
    private var source: BufferedSource? = null
    private var uri: Uri? = null
    private var bytesRemaining = 0L

    override fun addTransferListener(transferListener: TransferListener) = Unit

    override fun open(dataSpec: DataSpec): Long {
        close()
        val token = accessTokenProvider() ?: throw IOException("ログインセッションがありません")
        val request = Request.Builder()
            .url(dataSpec.uri.toString())
            .header("Authorization", "Bearer $token")
            .apply {
                if (dataSpec.position != 0L || dataSpec.length != C.LENGTH_UNSET.toLong()) {
                    val end = if (dataSpec.length == C.LENGTH_UNSET.toLong()) "" else {
                        (dataSpec.position + dataSpec.length - 1).toString()
                    }
                    header("Range", "bytes=${dataSpec.position}-$end")
                }
            }
            .build()
        val opened = client.newCall(request).execute()
        if (!opened.isSuccessful) {
            opened.close()
            throw IOException("メディアの取得に失敗しました: HTTP ${opened.code}")
        }
        if (dataSpec.position > 0L && opened.code != 206) {
            opened.close()
            throw IOException("メディアサーバーがRange要求に対応していません")
        }
        val body = opened.body ?: run {
            opened.close()
            throw IOException("メディアの本文が空です")
        }
        response = opened
        source = body.source()
        uri = dataSpec.uri
        bytesRemaining = when {
            dataSpec.length != C.LENGTH_UNSET.toLong() -> dataSpec.length
            body.contentLength() != -1L -> body.contentLength()
            else -> C.LENGTH_UNSET.toLong()
        }
        return bytesRemaining
    }

    override fun read(buffer: ByteArray, offset: Int, length: Int): Int {
        if (length == 0) return 0
        if (bytesRemaining == 0L) return C.RESULT_END_OF_INPUT
        val requested = if (bytesRemaining == C.LENGTH_UNSET.toLong()) length.toLong() else minOf(length.toLong(), bytesRemaining)
        val read = source?.read(buffer, offset, requested.toInt()) ?: throw IOException("メディアの読み込みが開始されていません")
        if (read == -1) {
            bytesRemaining = 0
            return C.RESULT_END_OF_INPUT
        }
        if (bytesRemaining != C.LENGTH_UNSET.toLong()) bytesRemaining -= read
        return read
    }

    override fun getUri(): Uri? = uri

    override fun getResponseHeaders(): Map<String, List<String>> = response?.headers?.let { headers ->
        headers.names().associateWith { name -> headers.values(name) }
    } ?: emptyMap()

    override fun close() {
        source = null
        response?.close()
        response = null
        uri = null
        bytesRemaining = 0L
    }

    class Factory(
        private val client: OkHttpClient,
        private val accessTokenProvider: () -> String?,
    ) : DataSource.Factory {
        override fun createDataSource(): DataSource = AuthenticatedDataSource(client, accessTokenProvider)
    }
}
