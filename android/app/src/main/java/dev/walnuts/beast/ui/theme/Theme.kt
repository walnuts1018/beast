package dev.walnuts.beast.ui.theme

import androidx.compose.foundation.isSystemInDarkTheme
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.darkColorScheme
import androidx.compose.runtime.Composable
import androidx.compose.ui.graphics.Color

private val BeastColors = darkColorScheme(
    primary = Color(0xFFFFB36B),
    onPrimary = Color(0xFF351300),
    secondary = Color(0xFFE0B8FF),
    onSecondary = Color(0xFF2D1248),
    background = Color(0xFF101011),
    onBackground = Color(0xFFF1EEE9),
    surface = Color(0xFF1B1A1B),
    onSurface = Color(0xFFF1EEE9),
    surfaceVariant = Color(0xFF282629),
    onSurfaceVariant = Color(0xFFC9C1C9),
    outline = Color(0xFF968D96),
)

@Composable
fun BeastTheme(content: @Composable () -> Unit) {
    MaterialTheme(colorScheme = BeastColors, content = content)
}
