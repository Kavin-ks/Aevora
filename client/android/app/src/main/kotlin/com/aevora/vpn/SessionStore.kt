package com.aevora.vpn

import android.content.Context
import androidx.security.crypto.EncryptedSharedPreferences
import androidx.security.crypto.MasterKey
import uniffi.aevora_core.FfiSession

/**
 * Persists the session (device id, refresh token, and WireGuard private key) in
 * EncryptedSharedPreferences, whose master key lives in the Android Keystore.
 * Only the public key is ever sent to the control plane.
 */
object SessionStore {
    private const val FILE = "aevora_session"

    private fun prefs(ctx: Context) = EncryptedSharedPreferences.create(
        ctx,
        FILE,
        MasterKey.Builder(ctx).setKeyScheme(MasterKey.KeyScheme.AES256_GCM).build(),
        EncryptedSharedPreferences.PrefKeyEncryptionScheme.AES256_SIV,
        EncryptedSharedPreferences.PrefValueEncryptionScheme.AES256_GCM,
    )

    fun save(ctx: Context, s: FfiSession) {
        prefs(ctx).edit()
            .putString("device_id", s.deviceId)
            .putString("user_id", s.userId)
            .putString("refresh_token", s.refreshToken)
            .putString("private_key", s.privateKey)
            .putString("public_key", s.publicKey)
            .apply()
    }

    fun load(ctx: Context): FfiSession? {
        val p = prefs(ctx)
        val id = p.getString("device_id", null) ?: return null
        return FfiSession(
            deviceId = id,
            userId = p.getString("user_id", "") ?: "",
            refreshToken = p.getString("refresh_token", "") ?: "",
            privateKey = p.getString("private_key", "") ?: "",
            publicKey = p.getString("public_key", "") ?: "",
        )
    }

    fun clear(ctx: Context) = prefs(ctx).edit().clear().apply()
}
