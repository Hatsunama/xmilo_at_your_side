package com.hatsunama.xmilo.dev

import android.content.BroadcastReceiver
import android.content.Context
import android.content.Intent
import androidx.core.content.ContextCompat

class XMiloclawBootReceiver : BroadcastReceiver() {
  override fun onReceive(context: Context, intent: Intent) {
    if (intent.action != Intent.ACTION_BOOT_COMPLETED) return
    try {
      ContextCompat.startForegroundService(
        context.applicationContext,
        Intent(context.applicationContext, XMiloclawRuntimeService::class.java)
      )
    } catch (_: Exception) {
      // Boot-time warm start is best-effort only.
    }
  }
}
