package com.hatsunama.xmilo.dev

import android.content.Context
import android.content.Intent
import android.os.Bundle
import androidx.appcompat.app.AppCompatActivity
import com.xmilo.castle.mobile.EbitenView

class NativeCastleActivity : AppCompatActivity() {
  companion object {
    const val EXTRA_WS_URL = "com.hatsunama.xmilo.dev.EXTRA_WS_URL"
    private const val FALLBACK_WS_URL = "ws://localhost:42817/ws"

    fun createIntent(context: Context, wsURL: String): Intent {
      return Intent(context, NativeCastleActivity::class.java).apply {
        putExtra(EXTRA_WS_URL, wsURL)
        addFlags(Intent.FLAG_ACTIVITY_NEW_TASK)
      }
    }
  }

  override fun onCreate(savedInstanceState: Bundle?) {
    super.onCreate(savedInstanceState)
    val wsURL = intent?.getStringExtra(EXTRA_WS_URL)?.takeUnless { it.isBlank() } ?: FALLBACK_WS_URL
    CastleRuntime.ensureStarted(wsURL, applicationContext)
    setContentView(EbitenView(this))
  }
}
