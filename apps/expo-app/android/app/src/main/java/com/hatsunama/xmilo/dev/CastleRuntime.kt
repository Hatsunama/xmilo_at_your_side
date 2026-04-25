package com.hatsunama.xmilo.dev

import android.content.Context
import android.util.Log
import com.xmilo.castle.mobile.Mobile
import go.Seq

/**
 * CastleRuntime centralizes the "start exactly once" contract for the native
 * Ebiten renderer. The gobind-generated Mobile.start(wsURL) must be called
 * before the SurfaceView attaches, otherwise the renderer may never draw.
 *
 * Both the React Native module and the view manager call into this helper so
 * the start timing is robust and the logs are consistent.
 */
object CastleRuntime {
  @Volatile
  private var started = false

  @Volatile
  private var lastWsURL: String? = null

  private fun redactWsURL(wsURL: String): String {
    // token is a bearer-equivalent secret; do not leak it into logcat.
    val idx = wsURL.indexOf("token=")
    if (idx < 0) return wsURL
    val prefix = wsURL.substring(0, idx + "token=".length)
    return prefix + "<redacted>"
  }

  fun setWsURL(wsURL: String) {
    lastWsURL = wsURL
    Log.i("xMiloCastle", "wsURL stored (token redacted)=${redactWsURL(wsURL)}")
  }

  fun ensureStarted(wsURL: String, context: Context) {
    if (started) return
    synchronized(this) {
      if (started) return
      lastWsURL = wsURL
      Seq.setContext(context.applicationContext)
      Log.i("xMiloCastle", "Starting native castle renderer wsURL=${redactWsURL(wsURL)}")
      Mobile.start(wsURL)
      started = true
    }
  }

  fun ensureStartedIfPossible(context: Context) {
    val wsURL = lastWsURL ?: run {
      val fallback = BuildConfig.DEFAULT_CASTLE_WS_URL
      if (!fallback.isNullOrBlank()) fallback else null
    } ?: return
    ensureStarted(wsURL, context)
  }
}
