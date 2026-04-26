package com.hatsunama.xmilo.dev

import android.content.Context
import android.content.Intent
import android.os.Bundle
import androidx.appcompat.app.AppCompatActivity
import com.xmilo.castle.mobile.EbitenView

class NativeCastleActivity : AppCompatActivity() {
  companion object {
    fun createIntent(context: Context): Intent {
      return Intent(context, NativeCastleActivity::class.java).apply {
        addFlags(Intent.FLAG_ACTIVITY_NEW_TASK)
      }
    }
  }

  override fun onCreate(savedInstanceState: Bundle?) {
    super.onCreate(savedInstanceState)
    CastleRuntime.ensureStartedIfPossible(applicationContext)
    setContentView(EbitenView(this))
  }
}
