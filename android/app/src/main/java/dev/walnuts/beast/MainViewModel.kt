package dev.walnuts.beast

import androidx.lifecycle.ViewModel
import androidx.lifecycle.viewModelScope
import dev.walnuts.beast.data.repository.VideoRepository
import dev.walnuts.beast.domain.model.RecommendationKind
import dev.walnuts.beast.domain.model.Video
import dev.walnuts.beast.domain.model.isFavorite
import dev.walnuts.beast.domain.model.recommendationKinds
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.flow.update
import kotlinx.coroutines.launch

enum class LibraryTab(val label: String) { FOR_YOU("あなたへ"), UNWATCHED("未視聴"), FAVORITES("お気に入り") }

data class LibraryUiState(
    val isLoading: Boolean = true,
    val videos: List<Video> = emptyList(),
    val selectedTag: String? = null,
    val searchQuery: String = "",
    val tab: LibraryTab = LibraryTab.FOR_YOU,
    val playingVideo: Video? = null,
    val editingVideo: Video? = null,
    val error: String? = null,
)

class MainViewModel(private val repository: VideoRepository) : ViewModel() {
    private val _uiState = MutableStateFlow(LibraryUiState())
    val uiState: StateFlow<LibraryUiState> = _uiState.asStateFlow()

    init { refresh() }

    fun refresh() = viewModelScope.launch {
        _uiState.update { it.copy(isLoading = true, error = null) }
        runCatching { repository.listVideos() }
            .onSuccess { videos -> _uiState.update { it.copy(isLoading = false, videos = videos) } }
            .onFailure { error -> _uiState.update { it.copy(isLoading = false, error = error.message ?: "動画を読み込めませんでした") } }
    }

    fun setSearchQuery(query: String) = _uiState.update { it.copy(searchQuery = query) }
    fun setTab(tab: LibraryTab) = _uiState.update { it.copy(tab = tab) }
    fun setTag(tag: String?) = _uiState.update { it.copy(selectedTag = tag) }
    fun openPlayer(video: Video) { _uiState.update { it.copy(playingVideo = video) } }
    fun closePlayer() { _uiState.update { it.copy(playingVideo = null) } }
    fun editTags(video: Video) { _uiState.update { it.copy(editingVideo = video) } }
    fun closeTagEditor() { _uiState.update { it.copy(editingVideo = null) } }

    fun saveTags(videoId: String, tags: List<String>) = viewModelScope.launch {
        _uiState.update { it.copy(editingVideo = null) }
        // TODO: backend schemaにタグ更新Mutationが追加されたら、現在の画面内更新を永続化処理へ置き換える。
        _uiState.update { state -> state.copy(videos = state.videos.map { if (it.id == videoId) it.copy(tags = tags) else it }) }
    }

    fun rate(video: Video, rating: Int?) = viewModelScope.launch {
        runCatching { repository.setRating(video.id, rating) }
            .onSuccess { updated -> _uiState.update { state -> state.copy(videos = state.videos.replace(updated), playingVideo = updated) } }
    }

    fun recordPlayback(video: Video) = viewModelScope.launch {
        runCatching { repository.recordPlayback(video.id) }
            .onSuccess { updated -> _uiState.update { state -> state.copy(videos = state.videos.replace(updated), playingVideo = updated) } }
    }

    fun filteredVideos(): List<Video> {
        val state = uiState.value
        val query = state.searchQuery.trim()
        return state.videos.filter { video ->
            val matchesTag = state.selectedTag == null || state.selectedTag in video.tags
            val matchesSearch = query.isEmpty() || video.tags.any { it.contains(query, ignoreCase = true) }
            val matchesTab = when (state.tab) {
                LibraryTab.FOR_YOU -> true
                LibraryTab.UNWATCHED -> video.playCount == 0
                LibraryTab.FAVORITES -> video.isFavorite()
            }
            matchesTag && matchesSearch && matchesTab
        }
    }

    fun lane(kind: RecommendationKind): List<Video> = uiState.value.videos.filter {
        kind in it.recommendationKinds(System.currentTimeMillis() / 1000)
    }

    private fun List<Video>.replace(updated: Video) = map { if (it.id == updated.id) updated else it }
}
