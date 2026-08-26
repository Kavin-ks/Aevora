package com.aevora.vpn

import android.app.Application
import androidx.lifecycle.AndroidViewModel
import androidx.lifecycle.viewModelScope
import com.wireguard.android.backend.Tunnel
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.Job
import kotlinx.coroutines.delay
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.flow.update
import kotlinx.coroutines.launch
import kotlinx.coroutines.withContext
import uniffi.aevora_core.FfiLocation
import uniffi.aevora_core.FfiStats

enum class Phase { NeedsEnrollment, Disconnected, Connecting, Connected, Failed }

data class UiState(
    val phase: Phase = Phase.NeedsEnrollment,
    val locations: List<FfiLocation> = emptyList(),
    val selectedCountry: String? = null,
    val serverName: String = "",
    val durationText: String = "00:00:00",
    val latencyText: String = "—",
    val downloadText: String = "—",
    val uploadText: String = "—",
    val error: String? = null,
) {
    val selectedCountryName: String?
        get() = locations.firstOrNull { it.code == selectedCountry }?.country
}

/**
 * The single view model. All auth/API/selection/lease/key/stats logic lives in
 * the shared Rust core (via [AevoraTunnelManager.core]); this class orchestrates
 * and formats, and runs the real WireGuard tunnel through the manager.
 */
class AevoraViewModel(app: Application) : AndroidViewModel(app) {

    private val manager = AevoraTunnelManager(app.applicationContext, BuildConfig.CONTROL_URL)
    private val _state = MutableStateFlow(UiState())
    val state: StateFlow<UiState> = _state.asStateFlow()

    private var statsJob: Job? = null

    init {
        manager.onState = { s -> onTunnelState(s) }
        val restored = SessionStore.load(getApplication())
        if (restored != null) {
            manager.core.restore(restored)
            _state.update { it.copy(phase = Phase.Disconnected) }
            loadLocations()
        }
    }

    fun enroll(invite: String, email: String) = launchIO {
        val session = manager.core.enroll(
            inviteCode = invite, email = email, displayName = null,
            deviceName = android.os.Build.MODEL ?: "Android", platform = "android",
        )
        SessionStore.save(getApplication(), session)
        _state.update { it.copy(phase = Phase.Disconnected, error = null) }
        loadLocations()
    }

    fun loadLocations() = launchIO {
        val locs = manager.core.locations()
        _state.update { it.copy(locations = locs) }
    }

    fun select(code: String) = _state.update { it.copy(selectedCountry = code) }

    /** Establishes the tunnel. Call only after VPN consent has been granted. */
    fun connect() {
        val country = _state.value.selectedCountry ?: return
        _state.update { it.copy(phase = Phase.Connecting, error = null) }
        launchIO {
            try {
                val server = manager.connect(country) // real GoBackend tunnel
                _state.update { it.copy(phase = Phase.Connected, serverName = server) }
                startStatsLoop()
            } catch (e: Exception) {
                _state.update { it.copy(phase = Phase.Failed, error = e.message) }
            }
        }
    }

    fun disconnect() = launchIO {
        stopStatsLoop()
        try {
            manager.disconnect()
        } finally {
            _state.update { it.copy(phase = Phase.Disconnected, serverName = "") }
        }
    }

    // Unexpected tunnel drop -> reflect disconnected state.
    private fun onTunnelState(s: Tunnel.State) {
        if (s == Tunnel.State.DOWN && _state.value.phase == Phase.Connected) {
            stopStatsLoop()
            _state.update { it.copy(phase = Phase.Disconnected, serverName = "") }
        }
    }

    private fun startStatsLoop() {
        stopStatsLoop()
        statsJob = viewModelScope.launch {
            while (true) {
                val stats = withContext(Dispatchers.IO) { runCatching { manager.reportStats() }.getOrNull() }
                if (stats != null) applyStats(stats)
                delay(3000)
            }
        }
    }

    private fun stopStatsLoop() {
        statsJob?.cancel(); statsJob = null
    }

    private fun applyStats(s: FfiStats) {
        _state.update {
            it.copy(
                durationText = formatDuration(s.durationSeconds),
                downloadText = formatRate(s.downloadBps),
                uploadText = formatRate(s.uploadBps),
                latencyText = if (s.latencyMs > 0u) "${s.latencyMs} ms" else "—",
            )
        }
    }

    companion object {
        fun formatRate(bytesPerSec: ULong): String {
            val mbps = bytesPerSec.toDouble() * 8 / 1_000_000
            return if (mbps >= 1) String.format("%.1f Mbps", mbps)
            else String.format("%.0f KB/s", bytesPerSec.toDouble() / 1024)
        }

        fun formatDuration(seconds: ULong): String {
            val s = seconds.toLong()
            return String.format("%02d:%02d:%02d", s / 3600, (s % 3600) / 60, s % 60)
        }
    }

    private fun launchIO(block: suspend () -> Unit) = viewModelScope.launch {
        try {
            withContext(Dispatchers.IO) { block() }
        } catch (e: Exception) {
            _state.update { it.copy(error = e.message) }
        }
    }
}
