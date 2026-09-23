package dev.wayshard.app

import android.security.keystore.KeyGenParameterSpec
import android.security.keystore.KeyProperties
import android.util.Base64
import java.security.KeyStore
import javax.crypto.Cipher
import javax.crypto.KeyGenerator
import javax.crypto.SecretKey
import javax.crypto.spec.GCMParameterSpec

/**
 * Android Keystore-backed credential encryption.
 *
 * The AES-256-GCM key is generated in the Android Keystore and is
 * non-exportable; only the ciphertext returned by [encrypt] is ever persisted
 * (the Wayshard app stores it in app-private storage). [encrypt], [decrypt] and
 * [deleteKey] are called from Rust over JNI.
 */
object WayshardKeystore {
    private const val KEYSTORE = "AndroidKeyStore"
    private const val ALIAS = "wayshard.device-credential"
    private const val TRANSFORMATION = "AES/GCM/NoPadding"
    private const val TAG_BITS = 128
    private const val NONCE_BYTES = 12

    private fun keyStore(): KeyStore =
        KeyStore.getInstance(KEYSTORE).apply { load(null) }

    @Synchronized
    private fun key(): SecretKey {
        val ks = keyStore()
        (ks.getEntry(ALIAS, null) as? KeyStore.SecretKeyEntry)?.let { return it.secretKey }
        val generator = KeyGenerator.getInstance(KeyProperties.KEY_ALGORITHM_AES, KEYSTORE)
        generator.init(
            KeyGenParameterSpec.Builder(
                ALIAS,
                KeyProperties.PURPOSE_ENCRYPT or KeyProperties.PURPOSE_DECRYPT,
            )
                .setBlockModes(KeyProperties.BLOCK_MODE_GCM)
                .setEncryptionPaddings(KeyProperties.ENCRYPTION_PADDING_NONE)
                .setKeySize(256)
                .setRandomizedEncryptionRequired(true)
                .build(),
        )
        return generator.generateKey()
    }

    /** Encrypts [plaintext] and returns "base64(nonce).base64(ciphertext)". */
    @JvmStatic
    fun encrypt(plaintext: String): String {
        val cipher = Cipher.getInstance(TRANSFORMATION)
        cipher.init(Cipher.ENCRYPT_MODE, key())
        val ciphertext = cipher.doFinal(plaintext.toByteArray(Charsets.UTF_8))
        val nonce = Base64.encodeToString(cipher.iv, Base64.NO_WRAP)
        return nonce + "." + Base64.encodeToString(ciphertext, Base64.NO_WRAP)
    }

    /** Decrypts a token produced by [encrypt]. Throws on tampering or corruption. */
    @JvmStatic
    fun decrypt(token: String): String {
        val parts = token.split(".")
        require(parts.size == 2) { "malformed credential token" }
        val nonce = Base64.decode(parts[0], Base64.NO_WRAP)
        val ciphertext = Base64.decode(parts[1], Base64.NO_WRAP)
        require(nonce.size == NONCE_BYTES) { "malformed credential nonce" }
        val cipher = Cipher.getInstance(TRANSFORMATION)
        cipher.init(Cipher.DECRYPT_MODE, key(), GCMParameterSpec(TAG_BITS, nonce))
        return String(cipher.doFinal(ciphertext), Charsets.UTF_8)
    }

    /** Best-effort removal of the Keystore key. The argument is unused. */
    @JvmStatic
    fun deleteKey(ignored: String): String {
        val ks = keyStore()
        if (ks.containsAlias(ALIAS)) {
            ks.deleteEntry(ALIAS)
        }
        return "ok"
    }
}
