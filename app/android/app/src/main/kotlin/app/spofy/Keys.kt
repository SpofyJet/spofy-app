package app.spofy

import android.content.Context
import android.provider.Settings
import android.security.keystore.KeyGenParameterSpec
import android.security.keystore.KeyProperties
import android.util.Base64
import java.io.File
import java.security.KeyStore
import java.security.MessageDigest
import java.security.SecureRandom
import javax.crypto.Cipher
import javax.crypto.KeyGenerator
import javax.crypto.SecretKey
import javax.crypto.spec.GCMParameterSpec

object Keys {
    private const val STORE = "AndroidKeyStore"
    private const val ALIAS = "spofy.state"
    private const val MODE = "AES/GCM/NoPadding"

    fun state(context: Context, folder: File): String {
        val wrapped = File(context.filesDir, "key")
        if (wrapped.exists()) {
            try {
                val body = wrapped.readBytes()
                val cipher = Cipher.getInstance(MODE)
                cipher.init(Cipher.DECRYPT_MODE, master(), GCMParameterSpec(128, body, 0, 12))
                return Base64.encodeToString(cipher.doFinal(body, 12, body.size - 12), Base64.NO_WRAP)
            } catch (e: Exception) {
                folder.deleteRecursively()
                wrapped.delete()
            }
        }
        val key = ByteArray(32).also { SecureRandom().nextBytes(it) }
        val cipher = Cipher.getInstance(MODE)
        cipher.init(Cipher.ENCRYPT_MODE, master())
        val fresh = File(context.filesDir, "key.new")
        fresh.writeBytes(cipher.iv + cipher.doFinal(key))
        if (!fresh.renameTo(wrapped)) {
            throw IllegalStateException("the key file cannot be written")
        }
        return Base64.encodeToString(key, Base64.NO_WRAP)
    }

    fun device(context: Context): String {
        val id = Settings.Secure.getString(context.contentResolver, Settings.Secure.ANDROID_ID) ?: ""
        val digest = MessageDigest.getInstance("SHA-256").digest("$id:${context.packageName}".toByteArray())
        return digest.take(16).joinToString("") { "%02x".format(it) }
    }

    private fun master(): SecretKey {
        val store = KeyStore.getInstance(STORE).apply { load(null) }
        (store.getKey(ALIAS, null) as? SecretKey)?.let { return it }
        val spec = KeyGenParameterSpec.Builder(ALIAS, KeyProperties.PURPOSE_ENCRYPT or KeyProperties.PURPOSE_DECRYPT)
            .setBlockModes(KeyProperties.BLOCK_MODE_GCM)
            .setEncryptionPaddings(KeyProperties.ENCRYPTION_PADDING_NONE)
            .setKeySize(256)
            .build()
        return KeyGenerator.getInstance(KeyProperties.KEY_ALGORITHM_AES, STORE).apply { init(spec) }.generateKey()
    }
}
