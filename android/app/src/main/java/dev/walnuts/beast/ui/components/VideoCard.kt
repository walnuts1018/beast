package dev.walnuts.beast.ui.components

import androidx.compose.foundation.background
import androidx.compose.foundation.clickable
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.aspectRatio
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.outlined.Lock
import androidx.compose.material.icons.outlined.MoreVert
import androidx.compose.material.icons.outlined.PlayArrow
import androidx.compose.material.icons.outlined.Star
import androidx.compose.material3.Icon
import androidx.compose.material3.IconButton
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Surface
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.graphics.Brush
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.dp
import dev.walnuts.beast.domain.model.Video

private val cardColors = listOf(
    listOf(Color(0xFF50332D), Color(0xFFCE7E55)),
    listOf(Color(0xFF302B59), Color(0xFF8A75C3)),
    listOf(Color(0xFF1D4A45), Color(0xFF9EBD72)),
    listOf(Color(0xFF4C264F), Color(0xFFC275A0)),
)

@Composable
fun VideoCard(video: Video, onClick: () -> Unit, onEditTags: () -> Unit, modifier: Modifier = Modifier) {
    Column(modifier.width(196.dp).clickable(onClick = onClick)) {
        VideoThumbnail(video, modifier = modifier.fillMaxWidth().aspectRatio(1.48f))
        Spacer(Modifier.height(9.dp))
        Row(verticalAlignment = Alignment.Top, horizontalArrangement = Arrangement.spacedBy(4.dp)) {
            Column(Modifier.weight(1f)) {
                Text(video.tags.joinToString("  ·  "), style = MaterialTheme.typography.titleSmall, maxLines = 1, overflow = TextOverflow.Ellipsis)
                Text(video.detailLabel(), style = MaterialTheme.typography.bodySmall, color = MaterialTheme.colorScheme.onSurfaceVariant, maxLines = 1)
            }
            IconButton(onClick = onEditTags, modifier = Modifier.size(32.dp)) { Icon(Icons.Outlined.MoreVert, "タグを編集", Modifier.size(18.dp)) }
        }
    }
}

@Composable
fun FeaturedVideoCard(video: Video, onClick: () -> Unit, modifier: Modifier = Modifier) {
    Surface(modifier = modifier.fillMaxWidth().clip(RoundedCornerShape(24.dp)).clickable(onClick = onClick), color = Color.Transparent) {
        Box(Modifier.fillMaxWidth().aspectRatio(1.7f)) {
            VideoThumbnail(video, Modifier.fillMaxSize())
            Box(Modifier.fillMaxSize().background(Brush.verticalGradient(listOf(Color.Transparent, Color(0xE6000000)))))
            Column(Modifier.align(Alignment.BottomStart).padding(18.dp)) {
                Row(verticalAlignment = Alignment.CenterVertically, horizontalArrangement = Arrangement.spacedBy(6.dp)) {
                    Icon(Icons.Outlined.Lock, null, Modifier.size(13.dp), tint = Color(0xFFFFC890))
                    Text("暗号化されたライブラリ", style = MaterialTheme.typography.labelSmall, color = Color(0xFFFFC890))
                }
                Spacer(Modifier.height(7.dp))
                Text(video.tags.joinToString("  ·  "), style = MaterialTheme.typography.headlineSmall, color = Color.White, maxLines = 1, overflow = TextOverflow.Ellipsis)
                Text("${video.playCount}回再生  ·  ${video.durationLabel()}", style = MaterialTheme.typography.bodySmall, color = Color(0xFFD5D0CC))
            }
            Surface(Modifier.align(Alignment.TopEnd).padding(14.dp), shape = RoundedCornerShape(50), color = Color(0xCC161416)) {
                Icon(Icons.Outlined.PlayArrow, "再生", Modifier.padding(9.dp).size(22.dp), tint = Color.White)
            }
        }
    }
}

@Composable
private fun VideoThumbnail(video: Video, modifier: Modifier = Modifier) {
    val colors = cardColors[video.id.hashCode().absoluteValue % cardColors.size]
    Box(modifier.clip(RoundedCornerShape(16.dp)).background(Brush.linearGradient(colors))) {
        Box(Modifier.align(Alignment.Center).size(48.dp).clip(RoundedCornerShape(50)).background(Color(0x30FFFFFF)), contentAlignment = Alignment.Center) {
            Icon(Icons.Outlined.PlayArrow, null, Modifier.size(26.dp), tint = Color.White)
        }
        Row(Modifier.align(Alignment.BottomStart).padding(10.dp), horizontalArrangement = Arrangement.spacedBy(6.dp), verticalAlignment = Alignment.CenterVertically) {
            Icon(Icons.Outlined.Lock, null, Modifier.size(13.dp), tint = Color(0xDDFFFFFF))
            Text(video.durationLabel(), style = MaterialTheme.typography.labelSmall, color = Color.White)
        }
        if (video.rating != null) {
            Row(Modifier.align(Alignment.TopEnd).padding(10.dp), verticalAlignment = Alignment.CenterVertically) {
                Icon(Icons.Outlined.Star, null, Modifier.size(14.dp), tint = Color(0xFFFFC46E))
                Text("${video.rating}", style = MaterialTheme.typography.labelSmall, color = Color.White)
            }
        }
        if (video.status.name == "ENCODING") {
            Surface(Modifier.align(Alignment.TopStart).padding(10.dp), shape = RoundedCornerShape(6.dp), color = Color(0xD918171A)) {
                Text("準備中", Modifier.padding(horizontal = 7.dp, vertical = 4.dp), style = MaterialTheme.typography.labelSmall, color = Color(0xFFFFD9B2))
            }
        }
    }
}

private fun Video.detailLabel(): String = "${playCount}回再生  ·  ${durationLabel()}"

private fun Video.durationLabel(): String {
    val seconds = durationSeconds ?: 0
    return if (seconds >= 3600) "%d時間%d分".format(seconds / 3600, (seconds % 3600) / 60) else "%d:%02d".format(seconds / 60, seconds % 60)
}

private val Int.absoluteValue: Int get() = if (this == Int.MIN_VALUE) 0 else kotlin.math.abs(this)
