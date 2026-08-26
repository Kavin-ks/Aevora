package com.aevora.vpn

import android.content.Context
import com.wireguard.android.backend.Backend
import com.wireguard.android.backend.GoBackend
import com.wireguard.android.backend.Tunnel
import com.wireguard.config.Config
import java.io.BufferedReader
import java.io.StringReader
import uniffi.aevora_core.AevoraClient
import uniffi.aevora_core.FfiStats
import uniffi.aevora_core.FfiTunnelConfig

/**
 * Drives the Android VPN using the shared Rust core + wireguard-android's
 * GoBackend. The core (via UniFFI) owns all API/auth/session/selection/key
 * logic; this class only turns the core's [FfiTunnelConfig] into a WireGuard
 * [Config] and brings the real tunnel up through [GoBackend].
 *
 * GoBackend runs its own VpnService (com.wireguard.android.backend.GoBackend$VpnService,
 * declared in the manifest); the caller must first obtain VpnService consent via
 * VpnService.prepare(context).
 */
class AevoraTunnelManager(context: Context, controlUrl: String) {

    private val backend: Backend = GoBackend(context)
    private val client = AevoraClient(controlUrl)

    /** A minimal Tunnel: GoBackend reports state changes here. */
    private val tunnel = object : Tunnel {
        override fun getName(): String = "aevora"
        override fun onStateChange(newState: Tunnel.State) {
            onState?.invoke(newState)
        }
    }

    var onState: ((Tunnel.State) -> Unit)? = null

    val core: AevoraClient get() = client

    /**
     * Prepares the connection through the core (select gateway + lease), builds a
     * WireGuard config, and brings the tunnel up. Returns the server name.
     */
    fun connect(countryCode: String): String {
        val conn = client.prepareConnection(countryCode)
        val config = Config.parse(BufferedReader(StringReader(wgQuick(conn.config))))
        backend.setState(tunnel, Tunnel.State.UP, config)
        client.markConnected()
        return conn.serverName
    }

    /** Tears the tunnel down and releases the lease. */
    fun disconnect() {
        backend.setState(tunnel, Tunnel.State.DOWN, null)
        client.disconnect()
    }

    fun keepAlive() = client.keepAlive()

    /**
     * Reads the real WireGuard byte counters from GoBackend and feeds them to the
     * core, which computes download/upload rates and renews the lease. Call
     * periodically (e.g. every 3s) while connected.
     */
    fun reportStats(): FfiStats? {
        val stats = backend.getStatistics(tunnel)
        var rx = 0L
        var tx = 0L
        for (key in stats.peers()) {
            stats.peer(key)?.let { rx += it.rxBytes; tx += it.txBytes }
        }
        return client.reportTunnelStats(rx.toULong(), tx.toULong(), null)
    }

    companion object {
        /** Renders a wg-quick config the wireguard-android parser understands. */
        fun wgQuick(c: FfiTunnelConfig): String = buildString {
            appendLine("[Interface]")
            appendLine("PrivateKey = ${c.privateKey}")
            appendLine("Address = ${c.addresses.joinToString(", ")}")
            if (c.dns.isNotEmpty()) appendLine("DNS = ${c.dns.joinToString(", ")}")
            appendLine()
            appendLine("[Peer]")
            appendLine("PublicKey = ${c.peerPublicKey}")
            appendLine("Endpoint = ${c.endpoint}")
            appendLine("AllowedIPs = ${c.allowedIps.joinToString(", ")}")
            appendLine("PersistentKeepalive = ${c.persistentKeepalive}")
        }
    }
}
