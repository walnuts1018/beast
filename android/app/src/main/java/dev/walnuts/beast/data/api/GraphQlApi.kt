package dev.walnuts.beast.data.api

import dev.walnuts.beast.domain.model.EncryptionMetadata
import dev.walnuts.beast.domain.model.Video
import dev.walnuts.beast.domain.model.VideoStatus
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.withContext
import kotlinx.serialization.Serializable
import kotlinx.serialization.json.Json
import kotlinx.serialization.json.JsonElement
import kotlinx.serialization.json.JsonObject
import kotlinx.serialization.json.buildJsonObject
import kotlinx.serialization.json.jsonArray
import kotlinx.serialization.json.jsonObject
import kotlinx.serialization.json.put
import okhttp3.MediaType.Companion.toMediaType
import okhttp3.OkHttpClient
import okhttp3.Request
import okhttp3.RequestBody.Companion.toRequestBody
import okhttp3.HttpUrl.Companion.toHttpUrl

@Serializable
data class GraphQlError(val message: String)

@Serializable
data class GraphQlResponse<T>(val data: T? = null, val errors: List<GraphQlError> = emptyList())

data class GraphQlOperation(
    val name: String,
    val query: String,
    val variables: JsonObject = buildJsonObject {},
)

interface GraphQlTransport {
    suspend fun execute(operation: GraphQlOperation, accessToken: String): Result<JsonElement>
}

class OkHttpGraphQlTransport(
    private val endpoint: String,
    private val client: OkHttpClient = OkHttpClient(),
    private val json: Json = Json { ignoreUnknownKeys = true },
) : GraphQlTransport {
    override suspend fun execute(operation: GraphQlOperation, accessToken: String): Result<JsonElement> = withContext(Dispatchers.IO) {
        runCatching {
            val requestBody = buildJsonObject {
                put("operationName", operation.name)
                put("query", operation.query)
                put("variables", operation.variables)
            }.toString().toRequestBody("application/json".toMediaType())
            val request = Request.Builder()
                .url(endpoint)
                .header("Authorization", "Bearer $accessToken")
                .post(requestBody)
                .build()
            client.newCall(request).execute().use { response ->
                check(response.isSuccessful) { "GraphQL request failed: ${response.code}" }
                val payload = json.parseToJsonElement(response.body?.string() ?: error("GraphQL response body is empty")).jsonObject
                payload["errors"]?.let { errors ->
                    check(errors.toString() == "[]") { "GraphQL operation ${operation.name} failed: $errors" }
                }
                payload["data"] ?: error("GraphQL operation ${operation.name} returned no data")
            }
        }
    }
}

object VideoGraphQlOperations {
    const val listVideos = """
        query ListVideos {
          videos {
            id status encryptedTags playCount rating lastPlayedAt progress
            encryption { algorithm chunkSize keyVersion nonce encryptedDataKey sharedKeyID }
          }
        }
    """
    const val rateVideo = """
        mutation RateVideo(${"$"}id: ID!, ${"$"}rating: Int) {
          rateVideo(id: ${"$"}id, rating: ${"$"}rating) { id status encryptedTags playCount rating lastPlayedAt progress
            encryption { algorithm chunkSize keyVersion nonce encryptedDataKey sharedKeyID } }
        }
    """
    const val recordPlayback = """
        mutation RecordPlayback(${"$"}id: ID!) {
          recordPlayback(id: ${"$"}id) { id status encryptedTags playCount rating lastPlayedAt progress
            encryption { algorithm chunkSize keyVersion nonce encryptedDataKey sharedKeyID } }
        }
    """
    const val registerSharedKey = """
        mutation RegisterSharedKey(${"$"}input: RegisterSharedKeyInput!) {
          registerSharedKey(input: ${"$"}input) { id version publicKey status }
        }
    """
    const val registerDeviceKey = """
        mutation RegisterDeviceKey(${"$"}input: RegisterDeviceKeyInput!) {
          registerDeviceKey(input: ${"$"}input) { id deviceID sharedKeyID encryptedSharedPrivateKey }
        }
    """
    const val deviceKeys = """
        query DeviceKeys {
          deviceKeys { id deviceID sharedKeyID encryptedSharedPrivateKey }
        }
    """
}

data class SharedKeyRegistration(val version: String, val publicKey: String)
data class DeviceKeyRegistration(val deviceId: String, val sharedKeyId: String, val encryptedSharedPrivateKey: String)

fun interface EncryptedTagsDecoder {
    suspend fun decode(video: Video): List<String>
}

object NoopEncryptedTagsDecoder : EncryptedTagsDecoder {
    override suspend fun decode(video: Video): List<String> = emptyList()
}

fun JsonElement.toVideo(apiEndpoint: String = ""): Video {
    val objectValue = jsonObject
    val videoId = objectValue["id"]!!.toString().trim('"')
    val encryption = objectValue["encryption"]!!.jsonObject
    return Video(
        id = videoId,
        status = VideoStatus.valueOf(objectValue["status"]!!.toString().trim('"')),
        tags = emptyList(),
        playCount = objectValue["playCount"]!!.toString().toInt(),
        rating = objectValue["rating"]?.takeUnless { it.toString() == "null" }?.toString()?.toInt(),
        lastPlayedAt = objectValue["lastPlayedAt"]?.takeUnless { it.toString() == "null" }?.toString()?.trim('"'),
        encryption = EncryptionMetadata(
            algorithm = encryption["algorithm"]!!.toString().trim('"'),
            chunkSize = encryption["chunkSize"]!!.toString().toInt(),
            keyVersion = encryption["keyVersion"]!!.toString().trim('"'),
            nonce = encryption["nonce"]!!.toString().trim('"'),
            encryptedDataKey = encryption["encryptedDataKey"]!!.toString().trim('"'),
            sharedKeyId = encryption["sharedKeyID"]!!.toString().trim('"'),
        ),
        encryptedTags = objectValue["encryptedTags"]?.toString()?.trim('"'),
        encryptedDashManifestUrl = apiEndpoint.takeIf(String::isNotBlank)?.let { endpoint ->
            runCatching {
                endpoint.toHttpUrl().newBuilder().encodedPath("/api/videos/$videoId/dash/manifest.mpd").query(null).build().toString()
            }.getOrNull()
        },
        progress = objectValue["progress"]?.toString()?.toFloat() ?: 0f,
    )
}
