package com.hatsunama.xmilo.dev

import android.content.Context
import android.os.PowerManager

object XMiloclawWakeController {
  @Volatile
  private var wakeLock: PowerManager.WakeLock? = null

  fun acquire(context: Context) {
    if (wakeLock?.isHeld == true) return

    val powerManager = context.applicationContext.getSystemService(Context.POWER_SERVICE) as PowerManager
    val lock =
      powerManager.newWakeLock(
        PowerManager.PARTIAL_WAKE_LOCK,
        "xMilo:HiddenRuntimeHost"
      )
    lock.setReferenceCounted(false)
    lock.acquire(10 * 60 * 1000L)
    wakeLock = lock
  }

  fun release() {
    wakeLock?.let { lock ->
      if (lock.isHeld) {
        lock.release()
      }
    }
    wakeLock = null
  }
}
