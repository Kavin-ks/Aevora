package com.aevora.vpn.ui

import androidx.compose.foundation.isSystemInDarkTheme
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.darkColorScheme
import androidx.compose.material3.lightColorScheme
import androidx.compose.runtime.Composable
import androidx.compose.ui.graphics.Color

// Original Aevora identity — a calm teal accent on a clean neutral surface.
val AevoraTeal = Color(0xFF0C9E90)
val AevoraTealDark = Color(0xFF2ED0BE)

private val LightColors = lightColorScheme(
    primary = AevoraTeal,
    secondary = AevoraTeal,
)
private val DarkColors = darkColorScheme(
    primary = AevoraTealDark,
    secondary = AevoraTealDark,
)

@Composable
fun AevoraTheme(content: @Composable () -> Unit) {
    MaterialTheme(
        colorScheme = if (isSystemInDarkTheme()) DarkColors else LightColors,
        content = content,
    )
}
