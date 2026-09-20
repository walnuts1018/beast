package dev.walnuts.beast.ui.screens

import android.net.Uri
import androidx.compose.foundation.background
import androidx.compose.foundation.gestures.detectTapGestures
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.outlined.ArrowBack
import androidx.compose.material.icons.outlined.Lock
import androidx.compose.material.icons.outlined.Pause
import androidx.compose.material.icons.outlined.PlayArrow
import androidx.compose.material.icons.outlined.Star
import androidx.compose.material.icons.outlined.StarBorder
import androidx.compose.material.icons.outlined.Speed
import androidx.compose.material3.Icon
import androidx.compose.material3.IconButton
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Slider
import androidx.compose.material3.Surface
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.DisposableEffect
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableFloatStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.rememberCoroutineScope
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.input.pointer.pointerInput
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.unit.dp
import androidx.compose.ui.viewinterop.AndroidView
import androidx.media3.common.MediaItem
import androidx.media3.datasource.DataSource
import androidx.media3.exoplayer.ExoPlayer
import androidx.media3.exoplayer.hls.HlsMediaSource
import androidx.media3.ui.PlayerView
import dev.walnuts.beast.domain.model.Video
import dev.walnuts.beast.domain.model.PlaybackSource
import kotlinx.coroutines.delay
import kotlinx.coroutines.launch

@Composable
fun PlayerScreen(video: Video, authenticatedDataSourceFactory: DataSource.Factory?, onClose: () -> Unit, onRate: (Video, Int?) -> Unit, onPlaybackRecorded: (Video) -> Unit) {
    val context = LocalContext.current
    val scope = rememberCoroutineScope()
    val isPlayablePreview = video.playbackSource == PlaybackSource.DEBUG_PREVIEW && video.playbackUrl != null
    val isPlayableHls = video.playbackSource == PlaybackSource.AUTHENTICATED_HLS && video.playbackManifestUrl != null && authenticatedDataSourceFactory != null
    val player = remember(video.id, video.playbackSource, video.playbackUrl, video.playbackManifestUrl, authenticatedDataSourceFactory) {
        ExoPlayer.Builder(context).build().apply {
            video.playbackUrl?.takeIf { isPlayablePreview }?.let {
                setMediaItem(MediaItem.fromUri(Uri.parse(it)))
                prepare()
                playWhenReady = true
            }
            if (isPlayableHls) {
                val mediaSource = HlsMediaSource.Factory(authenticatedDataSourceFactory!!).createMediaSource(MediaItem.fromUri(Uri.parse(video.playbackManifestUrl!!)))
                setMediaSource(mediaSource)
                prepare()
                playWhenReady = true
            }
        }
    }
    var playbackProgress by remember(video.id) { mutableFloatStateOf(0f) }

    DisposableEffect(player) {
        onDispose { player.release() }
    }
    LaunchedEffect(video.id, video.playbackSource, video.playbackUrl, video.playbackManifestUrl) {
        if (!isPlayablePreview && !isPlayableHls) return@LaunchedEffect
        var playbackRecorded = false
        while (true) {
            if (!playbackRecorded && player.playbackState == androidx.media3.common.Player.STATE_READY) {
                onPlaybackRecorded(video)
                playbackRecorded = true
            }
            val duration = player.duration
            if (duration > 0) playbackProgress = (player.currentPosition.toFloat() / duration).coerceIn(0f, 1f)
            delay(500)
        }
    }

    Box(Modifier.fillMaxSize().background(Color.Black)) {
        if (isPlayablePreview || isPlayableHls) {
            AndroidView(factory = { PlayerView(it).apply { this.player = player; useController = false } }, modifier = Modifier.fillMaxWidth().align(Alignment.Center))
        } else {
            Text("認証済みHLSを再生できません", color = Color.White, modifier = Modifier.align(Alignment.Center))
        }
        Box(
            Modifier.fillMaxSize().pointerInput(video.id) {
                detectTapGestures(
                    onTap = { if (player.isPlaying) player.pause() else player.play() },
                    onDoubleTap = { offset ->
                        val jump = if (offset.x < size.width / 2f) -10_000 else 10_000
                        player.seekTo((player.currentPosition + jump).coerceIn(0, player.duration.coerceAtLeast(0)))
                    },
                    onLongPress = {
                        player.setPlaybackSpeed(1.75f)
                        scope.launch { delay(1800); player.setPlaybackSpeed(1f) }
                    },
                )
            },
        )
        Row(Modifier.fillMaxWidth().align(Alignment.TopCenter).padding(top = 12.dp, start = 8.dp, end = 16.dp), verticalAlignment = Alignment.CenterVertically) {
            IconButton(onClick = onClose) { Icon(Icons.Outlined.ArrowBack, "ライブラリに戻る", tint = Color.White) }
            Surface(shape = RoundedCornerShape(50), color = Color(0xAA171518)) {
                Row(Modifier.padding(horizontal = 10.dp, vertical = 6.dp), verticalAlignment = Alignment.CenterVertically, horizontalArrangement = Arrangement.spacedBy(5.dp)) {
                    Icon(Icons.Outlined.Lock, null, Modifier.size(13.dp), tint = MaterialTheme.colorScheme.primary)
                    Text("サーバー側暗号化", style = MaterialTheme.typography.labelSmall, color = Color.White)
                }
            }
        }
        Column(Modifier.fillMaxWidth().align(Alignment.BottomCenter).background(Color(0xC9000000)).padding(horizontal = 16.dp, vertical = 18.dp), verticalArrangement = Arrangement.spacedBy(10.dp)) {
            Slider(value = playbackProgress, onValueChange = { value -> playbackProgress = value; if (player.duration > 0) player.seekTo((player.duration * value).toLong()) }, modifier = Modifier.fillMaxWidth())
            Row(verticalAlignment = Alignment.CenterVertically, horizontalArrangement = Arrangement.spacedBy(8.dp)) {
                IconButton(onClick = { if (player.isPlaying) player.pause() else player.play() }) { Icon(if (player.isPlaying) Icons.Outlined.Pause else Icons.Outlined.PlayArrow, "再生・一時停止", tint = Color.White) }
                Icon(Icons.Outlined.Speed, null, Modifier.size(18.dp), tint = Color(0xFFDED8D2))
                Text("長押しで1.75倍", style = MaterialTheme.typography.labelSmall, color = Color(0xFFDED8D2))
                Spacer(Modifier.weight(1f))
                Text("ダブルタップで±10秒", style = MaterialTheme.typography.labelSmall, color = Color(0xFFDED8D2))
            }
            Row(Modifier.fillMaxWidth(), verticalAlignment = Alignment.CenterVertically, horizontalArrangement = Arrangement.spacedBy(3.dp)) {
                Text("評価", style = MaterialTheme.typography.labelMedium, color = Color.White, modifier = Modifier.padding(end = 7.dp))
                (1..5).forEach { rating ->
                    IconButton(onClick = { onRate(video, if (video.rating == rating) null else rating) }, modifier = Modifier.size(34.dp)) { Icon(if ((video.rating ?: 0) >= rating) Icons.Outlined.Star else Icons.Outlined.StarBorder, "${rating}つ星", tint = Color(0xFFFFC46E)) }
                }
                Spacer(Modifier.weight(1f))
                Text(video.tags.joinToString("  ·  "), style = MaterialTheme.typography.labelSmall, color = Color(0xFFDED8D2), maxLines = 1)
            }
        }
    }
}
