package dev.walnuts.beast

import android.os.Bundle
import androidx.activity.ComponentActivity
import androidx.activity.compose.setContent
import androidx.activity.enableEdgeToEdge
import androidx.lifecycle.viewmodel.compose.viewModel
import androidx.lifecycle.ViewModel
import androidx.lifecycle.ViewModelProvider
import androidx.media3.datasource.DataSource
import dev.walnuts.beast.data.api.OkHttpGraphQlTransport
import dev.walnuts.beast.data.repository.GraphQlVideoRepository
import dev.walnuts.beast.data.repository.PreviewVideoRepository
import dev.walnuts.beast.data.repository.VideoRepository
import dev.walnuts.beast.media.EncryptedDashDataSource
import dev.walnuts.beast.security.AesGcmVideoDataKeyDecryptor
import dev.walnuts.beast.security.DeviceKeySession
import okhttp3.OkHttpClient
import dev.walnuts.beast.ui.BeastApp
import dev.walnuts.beast.ui.theme.BeastTheme

class MainActivity : ComponentActivity() {
    override fun onCreate(savedInstanceState: Bundle?) {
        super.onCreate(savedInstanceState)
        enableEdgeToEdge()
        val dependencies = createDependencies()
        setContent {
            BeastTheme {
                BeastApp(viewModel(factory = MainViewModelFactory(dependencies.repository, dependencies.encryptedDashDataSourceFactory)))
            }
        }
    }

    private fun createDependencies(): AppDependencies {
        if (BuildConfig.DEBUG) return AppDependencies(PreviewVideoRepository(), null)
        val client = OkHttpClient()
        val tokenProvider = { getSharedPreferences("beast_session", MODE_PRIVATE).getString("access_token", null) }
        val suspendTokenProvider: suspend () -> String? = { tokenProvider() }
        val transport = OkHttpGraphQlTransport(BuildConfig.API_BASE_URL, client)
        val deviceKeySession = DeviceKeySession(this, transport, suspendTokenProvider)
        return AppDependencies(
            repository = GraphQlVideoRepository(transport, suspendTokenProvider, BuildConfig.API_BASE_URL, deviceKeySession),
            encryptedDashDataSourceFactory = EncryptedDashDataSource.Factory(client, tokenProvider, deviceKeySession, AesGcmVideoDataKeyDecryptor()),
        )
    }
}

private data class AppDependencies(val repository: VideoRepository, val encryptedDashDataSourceFactory: DataSource.Factory?)

private class MainViewModelFactory(private val repository: VideoRepository, private val encryptedDashDataSourceFactory: DataSource.Factory?) : ViewModelProvider.Factory {
    @Suppress("UNCHECKED_CAST")
    override fun <T : ViewModel> create(modelClass: Class<T>): T = MainViewModel(repository, encryptedDashDataSourceFactory) as T
}
