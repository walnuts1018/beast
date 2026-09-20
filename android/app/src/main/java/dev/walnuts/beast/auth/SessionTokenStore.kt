package dev.walnuts.beast.auth

import android.content.Context
import android.security.keystore.KeyGenParameterSpec
import android.security.keystore.KeyProperties
import android.util.Base64
import java.nio.charset.StandardCharsets
import java.security.KeyStore
import javax.crypto.Cipher
import javax.crypto.KeyGenerator
import javax.crypto.SecretKey
import javax.crypto.spec.GCMParameterSpec

data class SessionToken(val value: String, val expiresAtEpochSeconds: Long)

class SessionTokenStore(context: Context) {
    private val preferences = context.getSharedPreferences(PREFERENCES_NAME, Context.MODE_PRIVATE)
    private val key: SecretKey by lazy { loadOrCreateKey() }

    fun accessToken(): SessionToken? {
        val value = decrypt(preferences.getString(ACCESS_TOKEN_KEY, null)) ?: return null
        val expiresAt = decrypt(preferences.getString(EXPIRES_AT_KEY, null))?.toLongOrNull() ?: 0L
        if (expiresAt <= System.currentTimeMillis() / 1000L) {
            clearAccessToken()
            return null
        }
        return SessionToken(value, expiresAt)
    }

    fun saveAccessToken(value: String, expiresInSeconds: Long) {
        val expiresAt = System.currentTimeMillis() / 1000L + expiresInSeconds.coerceAtLeast(60L)
        preferences.edit()
            .putString(ACCESS_TOKEN_KEY, encrypt(value))
            .putString(EXPIRES_AT_KEY, encrypt(expiresAt.toString()))
            .apply()
    }

    fun clearAccessToken() {
        preferences.edit().remove(ACCESS_TOKEN_KEY).remove(EXPIRES_AT_KEY).apply()
    }

    fun savePendingState(state: String) {
        preferences.edit().putString(PENDING_STATE_KEY, encrypt(state)).apply()
    }

    fun takePendingState(): String? {
        val state = decrypt(preferences.getString(PENDING_STATE_KEY, null))
        preferences.edit().remove(PENDING_STATE_KEY).apply()
        return state
    }

    private fun encrypt(value: String): String {
        val cipher = Cipher.getInstance(TRANSFORMATION)
        cipher.init(Cipher.ENCRYPT_MODE, key)
        val encrypted = cipher.iv + cipher.doFinal(value.toByteArray(StandardCharsets.UTF_8))
        return Base64.encodeToString(encrypted, Base64.NO_WRAP)
    }

    private fun decrypt(value: String?): String? {
        if (value.isNullOrEmpty()) return null
        return runCatching {
            val encrypted = Base64.decode(value, Base64.NO_WRAP)
            val iv = encrypted.copyOfRange(0, IV_SIZE)
            val ciphertext = encrypted.copyOfRange(IV_SIZE, encrypted.size)
            val cipher = Cipher.getInstance(TRANSFORMATION)
            cipher.init(Cipher.DECRYPT_MODE, key, GCMParameterSpec(TAG_SIZE_BITS, iv))
            String(cipher.doFinal(ciphertext), StandardCharsets.UTF_8)
        }.getOrNull()
    }

    private fun loadOrCreateKey(): SecretKey {
        val keyStore = KeyStore.getInstance(ANDROID_KEYSTORE).apply { load(null) }
        (keyStore.getKey(KEY_ALIAS, null) as? SecretKey)?.let { return it }
        val generator = KeyGenerator.getInstance(KeyProperties.KEY_ALGORITHM_AES, ANDROID_KEYSTORE)
        generator.init(
            KeyGenParameterSpec.Builder(KEY_ALIAS, KeyProperties.PURPOSE_ENCRYPT or KeyProperties.PURPOSE_DECRYPT)
                .setBlockModes(KeyProperties.BLOCK_MODE_GCM)
                .setEncryptionPaddings(KeyProperties.ENCRYPTION_PADDING_NONE)
                .setRandomizedEncryptionRequired(true)
                .build(),
        )
        return generator.generateKey()
    }

    private companion object {
        const val ANDROID_KEYSTORE = "AndroidKeyStore"
        const val KEY_ALIAS = "beast.session.aes"
        const val PREFERENCES_NAME = "beast_secure_session"
        const val ACCESS_TOKEN_KEY = "access_token"
        const val EXPIRES_AT_KEY = "expires_at"
        const val PENDING_STATE_KEY = "pending_state"
        const val TRANSFORMATION = "AES/GCM/NoPadding"
        const val IV_SIZE = 12
        const val TAG_SIZE_BITS = 128
    }
}
