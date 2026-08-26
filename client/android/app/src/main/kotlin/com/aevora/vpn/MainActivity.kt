package com.aevora.vpn

import android.app.Activity
import android.net.VpnService
import android.os.Bundle
import androidx.activity.ComponentActivity
import androidx.activity.compose.setContent
import androidx.activity.result.contract.ActivityResultContracts
import androidx.activity.viewModels
import androidx.compose.runtime.collectAsState
import androidx.compose.runtime.getValue
import com.aevora.vpn.ui.AevoraApp
import com.aevora.vpn.ui.AevoraTheme

/**
 * Hosts the Compose UI and owns the Android VPN consent flow. When the user taps
 * Connect, we request VpnService consent; on approval, the view model brings up
 * the real WireGuard tunnel via the shared core + GoBackend.
 */
class MainActivity : ComponentActivity() {

    private val model: AevoraViewModel by viewModels()

    private val vpnConsent = registerForActivityResult(ActivityResultContracts.StartActivityForResult()) { result ->
        if (result.resultCode == Activity.RESULT_OK) {
            model.connect()
        }
    }

    override fun onCreate(savedInstanceState: Bundle?) {
        super.onCreate(savedInstanceState)
        setContent {
            AevoraTheme {
                val state by model.state.collectAsState()
                AevoraApp(
                    state = state,
                    onEnroll = { invite, email -> model.enroll(invite, email) },
                    onSelect = { code -> model.select(code) },
                    onConnect = { requestVpnThenConnect() },
                    onDisconnect = { model.disconnect() },
                    onRefresh = { model.loadLocations() },
                )
            }
        }
    }

    /** Requests VpnService consent (once), then connects. */
    private fun requestVpnThenConnect() {
        val intent = VpnService.prepare(this)
        if (intent != null) {
            vpnConsent.launch(intent)
        } else {
            model.connect()
        }
    }
}
