package dev.walnuts.beast

import android.os.Bundle
import androidx.activity.ComponentActivity
import androidx.activity.compose.setContent
import androidx.activity.enableEdgeToEdge
import androidx.lifecycle.viewmodel.compose.viewModel
import androidx.lifecycle.ViewModel
import androidx.lifecycle.ViewModelProvider
import dev.walnuts.beast.data.api.OkHttpGraphQlTransport
import dev.walnuts.beast.data.repository.GraphQlVideoRepository
import dev.walnuts.beast.data.repository.PreviewVideoRepository
import dev.walnuts.beast.data.repository.VideoRepository
import dev.walnuts.beast.ui.BeastApp
import dev.walnuts.beast.ui.theme.BeastTheme

class MainActivity : ComponentActivity() {
    override fun onCreate(savedInstanceState: Bundle?) {
        super.onCreate(savedInstanceState)
        enableEdgeToEdge()
        setContent {
            BeastTheme {
                BeastApp(viewModel(factory = MainViewModelFactory(createRepository())))
            }
        }
    }

    private fun createRepository(): VideoRepository = if (BuildConfig.DEBUG) {
        PreviewVideoRepository()
    } else {
        GraphQlVideoRepository(OkHttpGraphQlTransport(BuildConfig.API_BASE_URL), accessTokenProvider = {
            getSharedPreferences("beast_session", MODE_PRIVATE).getString("access_token", null)
        })
    }
}

private class MainViewModelFactory(private val repository: VideoRepository) : ViewModelProvider.Factory {
    @Suppress("UNCHECKED_CAST")
    override fun <T : ViewModel> create(modelClass: Class<T>): T = MainViewModel(repository) as T
}
