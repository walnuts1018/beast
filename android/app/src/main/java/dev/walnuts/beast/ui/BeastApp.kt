package dev.walnuts.beast.ui

import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.outlined.Add
import androidx.compose.material.icons.outlined.Home
import androidx.compose.material.icons.outlined.VideoLibrary
import androidx.compose.material3.Icon
import androidx.compose.material3.NavigationBar
import androidx.compose.material3.NavigationBarItem
import androidx.compose.material3.Scaffold
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableIntStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import dev.walnuts.beast.MainViewModel
import dev.walnuts.beast.ui.screens.LibraryScreen

@Composable
fun BeastApp(viewModel: MainViewModel) {
    var selectedDestination by remember { mutableIntStateOf(0) }
    Scaffold(
        bottomBar = {
            NavigationBar {
                NavigationBarItem(selected = selectedDestination == 0, onClick = { selectedDestination = 0 }, icon = { Icon(Icons.Outlined.Home, null) }, label = { Text("ホーム") })
                NavigationBarItem(selected = selectedDestination == 1, onClick = { selectedDestination = 1 }, icon = { Icon(Icons.Outlined.VideoLibrary, null) }, label = { Text("ライブラリ") })
                NavigationBarItem(selected = selectedDestination == 2, onClick = { selectedDestination = 2 }, icon = { Icon(Icons.Outlined.Add, null) }, label = { Text("追加") })
            }
        },
    ) { paddingValues ->
        LibraryScreen(viewModel, paddingValues)
    }
}
