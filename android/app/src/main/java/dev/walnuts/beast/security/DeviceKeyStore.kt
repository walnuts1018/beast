package dev.walnuts.beast.security

import android.content.Context
import android.security.keystore.KeyGenParameterSpec
import android.security.keystore.KeyProperties
import java.io.ByteArrayOutputStream
import java.nio.ByteBuffer
import java.security.KeyFactory
import java.security.KeyPairGenerator
import java.security.KeyStore
import java.security.MessageDigest
import java.security.PrivateKey
import java.security.SecureRandom
import java.security.spec.ECGenParameterSpec
import java.security.spec.MGF1ParameterSpec
import java.security.spec.X509EncodedKeySpec
import java.util.Base64
import javax.crypto.Cipher
import javax.crypto.KeyAgreement
import javax.crypto.spec.GCMParameterSpec
import javax.crypto.spec.OAEPParameterSpec
import javax.crypto.spec.PSource
import javax.crypto.spec.SecretKeySpec

data class DevicePublicKey(val alias: String, val publicKeyBase64: String)

data class EncryptedSharedPrivateKey(
    val ephemeralPublicKey: String,
    val nonce: String,
    val ciphertext: String,
) {
    fun serialize(): String = listOf(ephemeralPublicKey, nonce, ciphertext).joinToString(".")

    companion object {
        fun parse(value: String): EncryptedSharedPrivateKey {
            val parts = value.split('.')
            require(parts.size == 3) { "Invalid Device Key envelope" }
            return EncryptedSharedPrivateKey(parts[0], parts[1], parts[2])
        }
    }
}

data class DataKeyEnvelope(
    val algorithm: String,
    val keyVersion: String,
    val nonce: ByteArray,
    val encryptedDataKey: ByteArray,
)

interface VideoDataKeyDecryptor {
    fun decryptDataKey(envelope: DataKeyEnvelope, sharedPrivateKeyPkcs8: ByteArray): ByteArray
    fun decryptChunk(ciphertext: ByteArray, dataKey: ByteArray, nonce: ByteArray, associatedData: ByteArray? = null): ByteArray
    fun decryptArtifact(ciphertext: ByteArray, envelope: DataKeyEnvelope, sharedPrivateKeyPkcs8: ByteArray): ByteArray
}

class AesGcmVideoDataKeyDecryptor : VideoDataKeyDecryptor {
    override fun decryptDataKey(envelope: DataKeyEnvelope, sharedPrivateKeyPkcs8: ByteArray): ByteArray {
        require(envelope.algorithm == "AES-256-GCM-CHUNKED-RSA-OAEP-SHA256") { "Unsupported data key algorithm: ${envelope.algorithm}" }
        val sharedPrivateKey = KeyFactory.getInstance("RSA").generatePrivate(java.security.spec.PKCS8EncodedKeySpec(sharedPrivateKeyPkcs8))
        val cipher = Cipher.getInstance("RSA/ECB/OAEPWithSHA-256AndMGF1Padding")
        cipher.init(
            Cipher.DECRYPT_MODE,
            sharedPrivateKey,
            OAEPParameterSpec("SHA-256", "MGF1", MGF1ParameterSpec.SHA256, PSource.PSpecified.DEFAULT),
        )
        return cipher.doFinal(envelope.encryptedDataKey)
    }

    override fun decryptChunk(ciphertext: ByteArray, dataKey: ByteArray, nonce: ByteArray, associatedData: ByteArray?): ByteArray {
        val cipher = Cipher.getInstance("AES/GCM/NoPadding")
        cipher.init(Cipher.DECRYPT_MODE, SecretKeySpec(dataKey, "AES"), GCMParameterSpec(128, nonce))
        associatedData?.let(cipher::updateAAD)
        return cipher.doFinal(ciphertext)
    }

    override fun decryptArtifact(ciphertext: ByteArray, envelope: DataKeyEnvelope, sharedPrivateKeyPkcs8: ByteArray): ByteArray {
        require(envelope.chunkSize > 0) { "Chunk size must be positive" }
        require(envelope.nonce.size == 12) { "Nonce must be 12 bytes" }
        val dataKey = decryptDataKey(envelope, sharedPrivateKeyPkcs8)
        val output = ByteArrayOutputStream(ciphertext.size)
        val input = ByteBuffer.wrap(ciphertext)
        var index = 0L
        while (input.hasRemaining()) {
            require(input.remaining() >= 4) { "Encrypted artifact length is truncated" }
            val length = input.int
            require(length >= 16 && length <= input.remaining()) { "Encrypted artifact chunk length is invalid" }
            val sealed = ByteArray(length)
            input.get(sealed)
            output.write(decryptChunk(sealed, dataKey, chunkNonce(envelope.nonce, index)))
            index++
        }
        return output.toByteArray()
    }
}

private fun chunkNonce(base: ByteArray, index: Long): ByteArray = base.copyOf().also { nonce ->
    repeat(8) { offset -> nonce[nonce.lastIndex - offset] = (nonce[nonce.lastIndex - offset].toInt() xor (index ushr (offset * 8)).toInt()).toByte() }
}

class DeviceKeyStore(context: Context) {
    private val alias = "beast-device-key-${context.packageName}"
    private val keyStore = KeyStore.getInstance("AndroidKeyStore").apply { load(null) }

    fun ensureDeviceKey(): DevicePublicKey {
        if (!keyStore.containsAlias(alias)) {
            KeyPairGenerator.getInstance(KeyProperties.KEY_ALGORITHM_EC, "AndroidKeyStore").apply {
                initialize(
                    KeyGenParameterSpec.Builder(alias, KeyProperties.PURPOSE_AGREE_KEY)
                        .setAlgorithmParameterSpec(ECGenParameterSpec("secp256r1"))
                        .build(),
                )
                generateKeyPair()
            }
        }
        val publicKey = keyStore.getCertificate(alias).publicKey.encoded
        return DevicePublicKey(alias, publicKey.encodeBase64())
    }

    fun decryptSharedPrivateKey(envelopeText: String): ByteArray {
        val envelope = EncryptedSharedPrivateKey.parse(envelopeText)
        val privateKey = keyStore.getKey(alias, null) as PrivateKey
        val ephemeralPublicKey = KeyFactory.getInstance("EC").generatePublic(
            X509EncodedKeySpec(envelope.ephemeralPublicKey.decodeBase64()),
        )
        val sessionKey = deriveSessionKey(privateKey, ephemeralPublicKey)
        val cipher = Cipher.getInstance("AES/GCM/NoPadding")
        cipher.init(Cipher.DECRYPT_MODE, sessionKey, GCMParameterSpec(128, envelope.nonce.decodeBase64()))
        return cipher.doFinal(envelope.ciphertext.decodeBase64())
    }

    private fun deriveSessionKey(privateKey: PrivateKey, publicKey: java.security.PublicKey): SecretKeySpec {
        val agreement = KeyAgreement.getInstance("ECDH")
        agreement.init(privateKey)
        agreement.doPhase(publicKey, true)
        val digest = MessageDigest.getInstance("SHA-256").digest(agreement.generateSecret())
        return SecretKeySpec(digest.copyOf(16), "AES")
    }
}

fun encryptSharedPrivateKeyForDevice(sharedPrivateKey: ByteArray, devicePublicKeyBase64: String): EncryptedSharedPrivateKey {
    val ephemeralGenerator = KeyPairGenerator.getInstance("EC")
    ephemeralGenerator.initialize(ECGenParameterSpec("secp256r1"))
    val ephemeralPair = ephemeralGenerator.generateKeyPair()
    val devicePublicKey = KeyFactory.getInstance("EC").generatePublic(X509EncodedKeySpec(devicePublicKeyBase64.decodeBase64()))
    val agreement = KeyAgreement.getInstance("ECDH")
    agreement.init(ephemeralPair.private)
    agreement.doPhase(devicePublicKey, true)
    val digest = MessageDigest.getInstance("SHA-256").digest(agreement.generateSecret())
    val cipher = Cipher.getInstance("AES/GCM/NoPadding")
    val nonce = ByteArray(12).also(SecureRandom()::nextBytes)
    cipher.init(Cipher.ENCRYPT_MODE, SecretKeySpec(digest.copyOf(16), "AES"), GCMParameterSpec(128, nonce))
    return EncryptedSharedPrivateKey(
        ephemeralPublicKey = ephemeralPair.public.encoded.encodeBase64(),
        nonce = nonce.encodeBase64(),
        ciphertext = cipher.doFinal(sharedPrivateKey).encodeBase64(),
    )
}

private fun ByteArray.encodeBase64(): String = Base64.getEncoder().withoutPadding().encodeToString(this)
private fun String.decodeBase64(): ByteArray = Base64.getDecoder().decode(this)
