package com.aevora.vpn

import android.app.Activity
import android.net.VpnService
import android.os.Bundle
import android.widget.Toast
import kotlin.concurrent.thread

/**
 * Minimal launcher activity. Phase 2b focuses on the tunnel path, not the UI;
 * this wires the VpnService consent flow to [AevoraTunnelManager] so the shared
 * core can drive a real connection. A full Compose UI (map, locations, stats)
 * comes later — this is intentionally a stub, not a fake tunnel.
 */
class MainActivity : Activity() {

    private lateinit var manager: AevoraTunnelManager
    private var pendingCountry: String? = null

    override fun onCreate(savedInstanceState: Bundle?) {
        super.onCreate(savedInstanceState)
        manager = AevoraTunnelManager(applicationContext, BuildConfig.CONTROL_URL)
        // TODO: build the consumer UI (enroll form, locations, connect/disconnect,
        // status + stats). For now, call connectTo(...) from your own UI/testing.
    }

    /** Requests VPN consent, then connects through the shared core off-thread. */
    fun connectTo(countryCode: String) {
        pendingCountry = countryCode
        val intent = VpnService.prepare(this)
        if (intent != null) {
            startActivityForResult(intent, REQ_VPN)
        } else {
            onActivityResult(REQ_VPN, RESULT_OK, null)
        }
    }

    override fun onActivityResult(requestCode: Int, resultCode: Int, data: android.content.Intent?) {
        super.onActivityResult(requestCode, resultCode, data)
        if (requestCode == REQ_VPN && resultCode == RESULT_OK) {
            val country = pendingCountry ?: return
            thread {
                try {
                    val server = manager.connect(country)
                    runOnUiThread { Toast.makeText(this, "Connected: $server", Toast.LENGTH_SHORT).show() }
                } catch (e: Exception) {
                    runOnUiThread { Toast.makeText(this, "Failed: ${e.message}", Toast.LENGTH_LONG).show() }
                }
            }
        }
    }

    fun disconnect() = thread { manager.disconnect() }

    companion object {
        private const val REQ_VPN = 1001
    }
}
