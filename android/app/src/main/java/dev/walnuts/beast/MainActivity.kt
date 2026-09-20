package dev.walnuts.beast

import android.content.Intent
import android.net.Uri
import android.os.Bundle
import androidx.activity.ComponentActivity
import androidx.activity.compose.setContent
import androidx.activity.enableEdgeToEdge
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.setValue
import androidx.compose.runtime.remember
import androidx.lifecycle.viewmodel.compose.viewModel
import androidx.lifecycle.ViewModel
import androidx.lifecycle.ViewModelProvider
import androidx.media3.datasource.DataSource
import dev.walnuts.beast.data.api.OkHttpGraphQlTransport
import dev.walnuts.beast.data.repository.GraphQlVideoRepository
import dev.walnuts.beast.data.repository.PreviewVideoRepository
import dev.walnuts.beast.data.repository.VideoRepository
import dev.walnuts.beast.media.AuthenticatedDataSource
import okhttp3.OkHttpClient
import dev.walnuts.beast.auth.SessionTokenStore
import dev.walnuts.beast.ui.BeastApp
import dev.walnuts.beast.ui.LoginScreen
import dev.walnuts.beast.ui.theme.BeastTheme
import java.security.SecureRandom
import android.util.Base64

class MainActivity : ComponentActivity() {
    private lateinit var sessionStore: SessionTokenStore
    private var sessionToken by mutableStateOf<String?>(null)
    private var authError by mutableStateOf<String?>(null)

    override fun onCreate(savedInstanceState: Bundle?) {
        super.onCreate(savedInstanceState)
        enableEdgeToEdge()
        sessionStore = SessionTokenStore(this)
        sessionToken = sessionStore.accessToken()?.value
        handleNativeCallback(intent)
        setContent {
            BeastTheme {
                if (BuildConfig.DEBUG) {
                    val dependencies = remember { createDependencies() }
                    BeastApp(viewModel(factory = MainViewModelFactory(dependencies.repository, dependencies.authenticatedDataSourceFactory)))
                } else if (sessionToken == null) {
                    LoginScreen(error = authError, onLogin = ::startLogin)
                } else {
                    val dependencies = remember(sessionToken) { createDependencies() }
                    BeastApp(
                        viewModel(key = sessionToken, factory = MainViewModelFactory(dependencies.repository, dependencies.authenticatedDataSourceFactory)),
                        onLogout = {
                            sessionStore.clearAccessToken()
                            sessionToken = null
                        },
                    )
                }
            }
        }
    }

    override fun onNewIntent(intent: Intent) {
        super.onNewIntent(intent)
        setIntent(intent)
        handleNativeCallback(intent)
    }

    private fun createDependencies(): AppDependencies {
        if (BuildConfig.DEBUG) return AppDependencies(PreviewVideoRepository(), null)
        val client = OkHttpClient()
        val tokenProvider = { sessionStore.accessToken()?.value }
        val suspendTokenProvider: suspend () -> String? = { tokenProvider() }
        val transport = OkHttpGraphQlTransport(BuildConfig.API_BASE_URL, client)
        return AppDependencies(
            repository = GraphQlVideoRepository(transport, suspendTokenProvider, BuildConfig.API_BASE_URL),
            authenticatedDataSourceFactory = AuthenticatedDataSource.Factory(client, tokenProvider),
        )
    }

    private fun startLogin() {
        val state = ByteArray(32).also(SecureRandom()::nextBytes).let { Base64.encodeToString(it, Base64.URL_SAFE or Base64.NO_WRAP or Base64.NO_PADDING) }
        sessionStore.savePendingState(state)
        authError = null
        val loginURL = Uri.parse("${BuildConfig.WEB_BASE_URL}/api/auth/mobile/login?state=${Uri.encode(state)}")
        startActivity(Intent(Intent.ACTION_VIEW, loginURL))
    }

    private fun handleNativeCallback(intent: Intent?) {
        val data = intent?.data ?: return
        if (data.scheme != "dev.walnuts.beast" || data.host != "oauth2redirect") return
        val parameters = data.fragment.orEmpty().split('&').filter { it.isNotEmpty() }.mapNotNull { pair ->
            val (key, value) = pair.split('=', limit = 2).let { it.first() to it.getOrElse(1) { "" } }
            key to Uri.decode(value)
        }.toMap()
        val expectedState = sessionStore.takePendingState()
        if (expectedState == null || expectedState != parameters["state"]) {
            authError = "ログイン状態を確認できませんでした。もう一度お試しください"
            return
        }
        val accessToken = parameters["access_token"]
        if (accessToken.isNullOrEmpty()) {
            authError = "ログインに失敗しました。もう一度お試しください"
            return
        }
        sessionStore.saveAccessToken(accessToken, parameters["expires_in"]?.toLongOrNull() ?: 3600L)
        authError = null
        sessionToken = sessionStore.accessToken()?.value
    }
}

private data class AppDependencies(val repository: VideoRepository, val authenticatedDataSourceFactory: DataSource.Factory?)

private class MainViewModelFactory(private val repository: VideoRepository, private val authenticatedDataSourceFactory: DataSource.Factory?) : ViewModelProvider.Factory {
    @Suppress("UNCHECKED_CAST")
    override fun <T : ViewModel> create(modelClass: Class<T>): T = MainViewModel(repository, authenticatedDataSourceFactory) as T
}
