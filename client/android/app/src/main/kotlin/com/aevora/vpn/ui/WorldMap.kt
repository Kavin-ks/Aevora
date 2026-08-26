package com.aevora.vpn.ui

import androidx.compose.foundation.gestures.detectTapGestures
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.ui.Modifier
import androidx.compose.ui.geometry.Offset
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.graphics.drawscope.Stroke
import androidx.compose.ui.input.pointer.pointerInput
import androidx.compose.foundation.Canvas
import androidx.compose.runtime.Composable
import androidx.compose.ui.unit.dp
import uniffi.aevora_core.FfiLocation

// A lightweight world map: countries are placed by an equirectangular projection
// of a static lat/long lookup (presentation only). Which countries are shown and
// whether they are available comes entirely from the control plane (locations).

data class GeoPoint(val lat: Double, val lon: Double)

object CountryCoordinates {
    private val table = mapOf(
        "de" to GeoPoint(50.11, 8.68), "nl" to GeoPoint(52.37, 4.90),
        "gb" to GeoPoint(51.51, -0.13), "fr" to GeoPoint(48.85, 2.35),
        "us" to GeoPoint(40.71, -74.01), "ca" to GeoPoint(43.65, -79.38),
        "sg" to GeoPoint(1.35, 103.82), "jp" to GeoPoint(35.68, 139.69),
        "in" to GeoPoint(19.08, 72.88), "au" to GeoPoint(-33.87, 151.21),
        "br" to GeoPoint(-23.55, -46.63), "ae" to GeoPoint(25.20, 55.27),
    )
    fun lookup(code: String): GeoPoint? = table[code.lowercase()]
}

@Composable
fun WorldMap(
    locations: List<FfiLocation>,
    selected: String?,
    onSelect: (String) -> Unit,
    modifier: Modifier = Modifier,
) {
    val plotted = locations.mapNotNull { loc ->
        CountryCoordinates.lookup(loc.code)?.let { Triple(loc, it, loc.available) }
    }

    Canvas(
        modifier = modifier
            .fillMaxWidth()
            .height(220.dp)
            .pointerInput(plotted) {
                detectTapGestures { tap ->
                    // Select the nearest available marker within a small radius.
                    val nearest = plotted.minByOrNull { (_, geo, _) ->
                        val p = project(geo, size.width.toFloat(), size.height.toFloat())
                        (p - tap).getDistanceSquared()
                    }
                    if (nearest != null) {
                        val (loc, geo, available) = nearest
                        val p = project(geo, size.width.toFloat(), size.height.toFloat())
                        if (available && (p - tap).getDistance() < 44f) onSelect(loc.code)
                    }
                }
            }
    ) {
        // Backdrop.
        drawRoundRectCompat(Color(0x11000000))
        // Simple graticule for a "map" feel.
        val grid = Color(0x22000000)
        for (i in 1..3) drawLineH(size.height * i / 4f, grid)
        for (i in 1..5) drawLineV(size.width * i / 6f, grid)

        plotted.forEach { (_, geo, available) ->
            val p = project(geo, size.width, size.height)
            val isSel = selected != null && plotted.any { it.first.code == selected && it.second == geo }
            val color = if (available) AevoraTeal else Color(0x66888888)
            if (isSel) drawCircle(AevoraTeal, radius = 16f, center = p, style = Stroke(width = 3f))
            drawCircle(color, radius = if (isSel) 9f else 6f, center = p)
        }
    }
}

private fun project(g: GeoPoint, w: Float, h: Float): Offset {
    val x = ((g.lon + 180.0) / 360.0 * w).toFloat()
    val y = ((90.0 - g.lat) / 180.0 * h).toFloat()
    return Offset(x, y)
}

// Small drawscope helpers to keep the Canvas body readable.
private fun androidx.compose.ui.graphics.drawscope.DrawScope.drawRoundRectCompat(color: Color) =
    drawRect(color)

private fun androidx.compose.ui.graphics.drawscope.DrawScope.drawLineH(y: Float, color: Color) =
    drawLine(color, Offset(0f, y), Offset(size.width, y), strokeWidth = 1f)

private fun androidx.compose.ui.graphics.drawscope.DrawScope.drawLineV(x: Float, color: Color) =
    drawLine(color, Offset(x, 0f), Offset(x, size.height), strokeWidth = 1f)
