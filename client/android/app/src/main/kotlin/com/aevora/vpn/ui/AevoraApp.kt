package com.aevora.vpn.ui

import androidx.compose.foundation.layout.*
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.items
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.filled.Refresh
import androidx.compose.material3.*
import androidx.compose.runtime.*
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.style.TextAlign
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import com.aevora.vpn.Phase
import com.aevora.vpn.UiState

@Composable
fun AevoraApp(
    state: UiState,
    onEnroll: (String, String) -> Unit,
    onSelect: (String) -> Unit,
    onConnect: () -> Unit,
    onDisconnect: () -> Unit,
    onRefresh: () -> Unit,
) {
    Surface(modifier = Modifier.fillMaxSize()) {
        Column(
            modifier = Modifier.fillMaxSize().padding(20.dp),
            horizontalAlignment = Alignment.CenterHorizontally,
        ) {
            Row(verticalAlignment = Alignment.CenterVertically) {
                Text("Aevora", fontSize = 28.sp, fontWeight = FontWeight.Bold)
                Text("•", fontSize = 28.sp, fontWeight = FontWeight.Bold, color = AevoraTeal)
            }
            Spacer(Modifier.height(16.dp))

            if (state.phase == Phase.NeedsEnrollment) {
                EnrollScreen(onEnroll)
            } else {
                MainScreen(state, onSelect, onConnect, onDisconnect, onRefresh)
            }

            state.error?.let {
                Spacer(Modifier.height(12.dp))
                Text(it, color = MaterialTheme.colorScheme.error, fontSize = 12.sp)
            }
        }
    }
}

@Composable
private fun EnrollScreen(onEnroll: (String, String) -> Unit) {
    var invite by remember { mutableStateOf("") }
    var email by remember { mutableStateOf("") }
    Column(
        horizontalAlignment = Alignment.CenterHorizontally,
        verticalArrangement = Arrangement.spacedBy(12.dp),
        modifier = Modifier.padding(top = 40.dp).widthIn(max = 360.dp),
    ) {
        Text("Enroll this device", style = MaterialTheme.typography.titleMedium)
        OutlinedTextField(invite, { invite = it }, label = { Text("Invite code") }, singleLine = true)
        OutlinedTextField(email, { email = it }, label = { Text("Email") }, singleLine = true)
        Button(
            onClick = { onEnroll(invite, email) },
            enabled = invite.isNotBlank() && email.isNotBlank(),
            colors = ButtonDefaults.buttonColors(containerColor = AevoraTeal),
        ) { Text("Enroll") }
    }
}

@Composable
private fun MainScreen(
    state: UiState,
    onSelect: (String) -> Unit,
    onConnect: () -> Unit,
    onDisconnect: () -> Unit,
    onRefresh: () -> Unit,
) {
    WorldMap(state.locations, state.selectedCountry, onSelect)
    Spacer(Modifier.height(14.dp))

    // Control panel.
    Surface(
        shape = RoundedCornerShape(14.dp),
        color = MaterialTheme.colorScheme.surfaceVariant.copy(alpha = 0.4f),
        modifier = Modifier.fillMaxWidth(),
    ) {
        Column(
            Modifier.padding(16.dp),
            horizontalAlignment = Alignment.CenterHorizontally,
            verticalArrangement = Arrangement.spacedBy(8.dp),
        ) {
            Text(
                stateText(state.phase),
                style = MaterialTheme.typography.titleMedium,
                color = if (state.phase == Phase.Connected) AevoraTeal else MaterialTheme.colorScheme.onSurfaceVariant,
            )
            Text(
                if (state.phase == Phase.Connected) state.serverName
                else (state.selectedCountryName ?: "Select a location"),
                style = MaterialTheme.typography.bodyMedium,
            )
            val connected = state.phase == Phase.Connected || state.phase == Phase.Connecting
            Button(
                onClick = { if (connected) onDisconnect() else onConnect() },
                enabled = connected || state.selectedCountry != null,
                colors = ButtonDefaults.buttonColors(
                    containerColor = if (state.phase == Phase.Connected) MaterialTheme.colorScheme.error else AevoraTeal
                ),
                modifier = Modifier.fillMaxWidth().height(48.dp),
            ) {
                Text(
                    when (state.phase) {
                        Phase.Connected -> "Disconnect"
                        Phase.Connecting -> "Cancel"
                        else -> "Connect"
                    }
                )
            }
            if (state.phase == Phase.Connected) StatsGrid(state)
        }
    }

    Spacer(Modifier.height(10.dp))
    Row(verticalAlignment = Alignment.CenterVertically) {
        Text("Locations", style = MaterialTheme.typography.titleSmall, modifier = Modifier.weight(1f))
        IconButton(onClick = onRefresh) { Icon(Icons.Filled.Refresh, contentDescription = "Refresh") }
    }
    LazyColumn(Modifier.fillMaxWidth().heightIn(max = 220.dp)) {
        items(state.locations) { loc ->
            ListItem(
                headlineContent = { Text(loc.country) },
                trailingContent = {
                    if (loc.available) {
                        TextButton(onClick = { onSelect(loc.code) }) {
                            Text(if (state.selectedCountry == loc.code) "Selected" else "Select")
                        }
                    } else Text("unavailable", color = MaterialTheme.colorScheme.onSurfaceVariant)
                },
            )
        }
    }
}

@Composable
private fun StatsGrid(state: UiState) {
    Column(Modifier.padding(top = 6.dp), verticalArrangement = Arrangement.spacedBy(4.dp)) {
        StatRow("Duration", state.durationText)
        StatRow("Latency", state.latencyText)
        StatRow("Download", state.downloadText)
        StatRow("Upload", state.uploadText)
    }
}

@Composable
private fun StatRow(label: String, value: String) {
    Row(Modifier.fillMaxWidth()) {
        Text(label, style = MaterialTheme.typography.labelMedium, modifier = Modifier.weight(1f))
        Text(value, style = MaterialTheme.typography.labelMedium, textAlign = TextAlign.End)
    }
}

private fun stateText(p: Phase) = when (p) {
    Phase.Connected -> "CONNECTED"
    Phase.Connecting -> "CONNECTING…"
    Phase.Failed -> "FAILED"
    else -> "DISCONNECTED"
}
