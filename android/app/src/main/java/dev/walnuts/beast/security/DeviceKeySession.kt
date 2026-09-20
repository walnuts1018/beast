package dev.walnuts.beast.security

import android.content.Context
import dev.walnuts.beast.data.api.GraphQlOperation
import dev.walnuts.beast.data.api.GraphQlTransport
import dev.walnuts.beast.data.api.VideoGraphQlOperations
import java.security.KeyPairGenerator
import java.security.spec.RSAKeyGenParameterSpec
import java.util.Base64
import kotlinx.coroutines.sync.Mutex
import kotlinx.coroutines.sync.withLock
import kotlinx.serialization.json.buildJsonObject
import kotlinx.serialization.json.jsonArray
import kotlinx.serialization.json.jsonObject
import kotlinx.serialization.json.put

data class SharedKeySession(
    val sharedKeyId: String,
    private val privateKeyPkcs8: ByteArray,
) {
    fun privateKeyCopy(): ByteArray = privateKeyPkcs8.copyOf()
}

class DeviceKeySession(
    context: Context,
    private val transport: GraphQlTransport,
    private val accessTokenProvider: suspend () -> String?,
) {
    private val deviceKeyStore = DeviceKeyStore(context)
    private val initializationMutex = Mutex()
    @Volatile private var current: SharedKeySession? = null

    suspend fun ensureReady(): SharedKeySession = initializationMutex.withLock {
        current ?: initialize().also { current = it }
    }

    fun privateKeyFor(sharedKeyId: String): ByteArray {
        val session = current ?: error("Device Keyセッションが初期化されていません")
        require(session.sharedKeyId == sharedKeyId) { "Shared Keyが動画の暗号化メタデータと一致しません" }
        return session.privateKeyCopy()
    }

    private suspend fun initialize(): SharedKeySession {
        val token = accessTokenProvider() ?: error("ログインセッションがありません")
        val device = deviceKeyStore.ensureDeviceKey()
        val deviceKeys = transport.execute(GraphQlOperation("DeviceKeys", VideoGraphQlOperations.deviceKeys), token)
            .getOrThrow().jsonObject["deviceKeys"]!!.jsonArray
        val registered = deviceKeys.firstOrNull { it.jsonObject["deviceID"]?.toString()?.trim('"') == device.alias }
        if (registered != null) {
            val encrypted = registered.jsonObject["encryptedSharedPrivateKey"]!!.toString().trim('"')
            val privateKey = deviceKeyStore.decryptSharedPrivateKey(encrypted)
            return SharedKeySession(registered.jsonObject["sharedKeyID"]!!.toString().trim('"'), privateKey)
        }

        val sharedKeyPair = KeyPairGenerator.getInstance("RSA").apply {
            initialize(RSAKeyGenParameterSpec(3072, RSAKeyGenParameterSpec.F4))
        }.generateKeyPair()
        val sharedKey = transport.execute(
            GraphQlOperation(
                "RegisterSharedKey",
                VideoGraphQlOperations.registerSharedKey,
                buildJsonObject {
                    put("input", buildJsonObject {
                        put("version", "android-${System.currentTimeMillis()}")
                        put("publicKey", sharedKeyPair.public.encoded.toPem("PUBLIC KEY"))
                    })
                },
            ),
            token,
        ).getOrThrow().jsonObject["registerSharedKey"]!!.jsonObject
        val sharedKeyId = sharedKey["id"]!!.toString().trim('"')
        val envelope = encryptSharedPrivateKeyForDevice(sharedKeyPair.private.encoded, device.publicKeyBase64)
        val encodedEnvelope = Base64.getEncoder().withoutPadding().encodeToString(envelope.serialize().toByteArray(Charsets.UTF_8))
        transport.execute(
            GraphQlOperation(
                "RegisterDeviceKey",
                VideoGraphQlOperations.registerDeviceKey,
                buildJsonObject {
                    put("input", buildJsonObject {
                        put("deviceID", device.alias)
                        put("sharedKeyID", sharedKeyId)
                        put("encryptedSharedPrivateKey", encodedEnvelope)
                    })
                },
            ),
            token,
        ).getOrThrow()
        return SharedKeySession(sharedKeyId, sharedKeyPair.private.encoded)
    }
}

private fun ByteArray.toPem(type: String): String {
    val body = Base64.getMimeEncoder(64, byteArrayOf('\n'.code.toByte())).encodeToString(this)
    return "-----BEGIN $type-----\n$body\n-----END $type-----"
}
