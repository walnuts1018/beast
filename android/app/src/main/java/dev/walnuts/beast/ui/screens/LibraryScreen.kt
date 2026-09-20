package dev.walnuts.beast.ui.screens

import androidx.compose.foundation.background
import androidx.compose.foundation.horizontalScroll
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.PaddingValues
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.LazyRow
import androidx.compose.foundation.lazy.items
import androidx.compose.foundation.rememberScrollState
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.outlined.Add
import androidx.compose.material.icons.outlined.Close
import androidx.compose.material.icons.outlined.Lock
import androidx.compose.material.icons.outlined.Search
import androidx.compose.material.icons.outlined.Tune
import androidx.compose.material3.AssistChip
import androidx.compose.material3.CircularProgressIndicator
import androidx.compose.material3.FilterChip
import androidx.compose.material3.Icon
import androidx.compose.material3.IconButton
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedTextField
import androidx.compose.material3.Surface
import androidx.compose.material3.Tab
import androidx.compose.material3.TabRow
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.runtime.Composable
import androidx.compose.runtime.collectAsState
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.input.ImeAction
import androidx.compose.ui.unit.dp
import androidx.compose.foundation.text.KeyboardOptions
import dev.walnuts.beast.LibraryTab
import dev.walnuts.beast.MainViewModel
import dev.walnuts.beast.domain.model.RecommendationKind
import dev.walnuts.beast.domain.model.Video
import dev.walnuts.beast.ui.components.FeaturedVideoCard
import dev.walnuts.beast.ui.components.VideoCard

@Composable
fun LibraryScreen(viewModel: MainViewModel, paddingValues: PaddingValues) {
    val state by viewModel.uiState.collectAsState()
    val filtered = viewModel.filteredVideos()
    Box(Modifier.fillMaxSize().background(MaterialTheme.colorScheme.background)) {
        if (state.playingVideo != null) {
            PlayerScreen(state.playingVideo!!, viewModel.encryptedDashDataSourceFactory, viewModel::closePlayer, viewModel::rate, viewModel::recordPlayback)
        } else {
            LazyColumn(contentPadding = PaddingValues(top = paddingValues.calculateTopPadding() + 24.dp, bottom = paddingValues.calculateBottomPadding() + 24.dp), verticalArrangement = Arrangement.spacedBy(24.dp)) {
                item { LibraryHeader(state.searchQuery, viewModel::setSearchQuery) }
                item { LibraryTabs(state.tab, viewModel::setTab) }
                item { TagFilters(state.videos.flatMap { it.tags }.distinct(), state.selectedTag, viewModel::setTag) }
                if (state.isLoading) {
                    item { LoadingLibrary() }
                } else if (state.error != null) {
                    item { Text(state.error!!, Modifier.padding(horizontal = 20.dp), color = MaterialTheme.colorScheme.error) }
                } else {
                    val featured = filtered.firstOrNull() ?: state.videos.firstOrNull()
                    if (featured != null) item { FeaturedVideoCard(featured, { viewModel.openPlayer(featured) }, Modifier.padding(horizontal = 20.dp)) }
                    item { RecommendationRail(RecommendationKind.FAVORITES, viewModel, viewModel::openPlayer, viewModel::editTags) }
                    item { RecommendationRail(RecommendationKind.UNWATCHED, viewModel, viewModel::openPlayer, viewModel::editTags) }
                    item { RecommendationRail(RecommendationKind.RETURN_TO_FAVORITES, viewModel, viewModel::openPlayer, viewModel::editTags) }
                    item { AllVideosHeader(filtered.size) }
                    item {
                        if (filtered.isEmpty()) EmptyLibrary() else LazyRow(contentPadding = PaddingValues(horizontal = 20.dp), horizontalArrangement = Arrangement.spacedBy(14.dp)) {
                            items(filtered, key = { it.id }) { video -> VideoCard(video, { viewModel.openPlayer(video) }, { viewModel.editTags(video) }) }
                        }
                    }
                }
            }
        }
    }
    state.editingVideo?.let { TagEditorDialog(it, viewModel::saveTags, viewModel::closeTagEditor) }
}

@Composable
private fun LibraryHeader(query: String, onQueryChanged: (String) -> Unit) {
    Column(Modifier.padding(horizontal = 20.dp), verticalArrangement = Arrangement.spacedBy(14.dp)) {
        Row(verticalAlignment = Alignment.Top, horizontalArrangement = Arrangement.SpaceBetween, modifier = Modifier.fillMaxWidth()) {
            Column {
                Text("こんばんは、ユウ", style = MaterialTheme.typography.labelLarge, color = MaterialTheme.colorScheme.primary)
                Text("今夜は何を見よう？", style = MaterialTheme.typography.headlineMedium, fontWeight = FontWeight.SemiBold)
            }
            Surface(shape = MaterialTheme.shapes.large, color = MaterialTheme.colorScheme.surfaceVariant) {
                Icon(Icons.Outlined.Lock, "暗号化済み", Modifier.padding(11.dp).size(18.dp), tint = MaterialTheme.colorScheme.primary)
            }
        }
        OutlinedTextField(value = query, onValueChange = onQueryChanged, modifier = Modifier.fillMaxWidth(), singleLine = true, leadingIcon = { Icon(Icons.Outlined.Search, null) }, trailingIcon = { if (query.isNotEmpty()) IconButton(onClick = { onQueryChanged("") }) { Icon(Icons.Outlined.Close, "検索をクリア") } }, placeholder = { Text("タグから探す") }, keyboardOptions = KeyboardOptions(imeAction = ImeAction.Search), shape = MaterialTheme.shapes.large)
    }
}

@Composable
private fun LibraryTabs(tab: LibraryTab, onTabChanged: (LibraryTab) -> Unit) {
    TabRow(selectedTabIndex = tab.ordinal, containerColor = Color.Transparent, contentColor = MaterialTheme.colorScheme.primary) {
        LibraryTab.entries.forEach { item -> Tab(selected = tab == item, onClick = { onTabChanged(item) }, text = { Text(item.label) }) }
    }
}

@Composable
private fun TagFilters(tags: List<String>, selectedTag: String?, onTagChanged: (String?) -> Unit) {
    Row(Modifier.horizontalScroll(rememberScrollState()).padding(horizontal = 20.dp), horizontalArrangement = Arrangement.spacedBy(8.dp)) {
        AssistChip(onClick = { onTagChanged(null) }, label = { Text("すべて") }, leadingIcon = { Icon(Icons.Outlined.Tune, null, Modifier.size(16.dp)) })
        tags.forEach { tag -> FilterChip(selected = selectedTag == tag, onClick = { onTagChanged(if (selectedTag == tag) null else tag) }, label = { Text("#$tag") }) }
        FilterChip(selected = false, onClick = {}, enabled = false, label = { Text("タグを追加") }, leadingIcon = { Icon(Icons.Outlined.Add, null, Modifier.size(16.dp)) })
    }
}

@Composable
private fun RecommendationRail(kind: RecommendationKind, viewModel: MainViewModel, onPlay: (Video) -> Unit, onEditTags: (Video) -> Unit) {
    val videos = viewModel.lane(kind)
    if (videos.isEmpty()) return
    Column(verticalArrangement = Arrangement.spacedBy(12.dp)) {
        Row(Modifier.padding(horizontal = 20.dp), verticalAlignment = Alignment.Bottom, horizontalArrangement = Arrangement.SpaceBetween) {
            Column(Modifier.weight(1f)) { Text(kind.label, style = MaterialTheme.typography.titleLarge, fontWeight = FontWeight.SemiBold); Text(kind.description, style = MaterialTheme.typography.bodySmall, color = MaterialTheme.colorScheme.onSurfaceVariant) }
            TextButton(onClick = {}) { Text("すべて見る") }
        }
        LazyRow(contentPadding = PaddingValues(horizontal = 20.dp), horizontalArrangement = Arrangement.spacedBy(14.dp)) { items(videos, key = { "${kind.name}-${it.id}" }) { VideoCard(it, { onPlay(it) }, { onEditTags(it) }) } }
    }
}

@Composable
private fun AllVideosHeader(count: Int) { Row(Modifier.padding(horizontal = 20.dp), verticalAlignment = Alignment.CenterVertically) { Text("すべての動画", style = MaterialTheme.typography.titleLarge, fontWeight = FontWeight.SemiBold); Spacer(Modifier.width(8.dp)); Text("${count}本", style = MaterialTheme.typography.bodySmall, color = MaterialTheme.colorScheme.onSurfaceVariant) } }

@Composable
private fun LoadingLibrary() { Box(Modifier.fillMaxWidth().height(280.dp), contentAlignment = Alignment.Center) { CircularProgressIndicator() } }

@Composable
private fun EmptyLibrary() { Column(Modifier.fillMaxWidth().padding(32.dp), horizontalAlignment = Alignment.CenterHorizontally, verticalArrangement = Arrangement.spacedBy(8.dp)) { Text("動画が見つかりません", style = MaterialTheme.typography.titleMedium); Text("別のタグやタブを選んでみてください。", color = MaterialTheme.colorScheme.onSurfaceVariant) } }

@Composable
private fun TagEditorDialog(video: Video, onSave: (String, List<String>) -> Unit, onDismiss: () -> Unit) {
    var text by remember(video.id) { mutableStateOf(video.tags.joinToString(" ")) }
    androidx.compose.material3.AlertDialog(onDismissRequest = onDismiss, title = { Text("タグを編集") }, text = { OutlinedTextField(value = text, onValueChange = { text = it }, label = { Text("タグ") }, supportingText = { Text("スペース区切りで入力") }, singleLine = true) }, confirmButton = { TextButton(onClick = { onSave(video.id, text.split(" ").map(String::trim).filter(String::isNotEmpty).distinct()) }) { Text("保存") } }, dismissButton = { TextButton(onClick = onDismiss) { Text("キャンセル") } })
}
