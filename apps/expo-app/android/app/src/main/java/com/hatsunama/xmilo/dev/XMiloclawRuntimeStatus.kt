package com.hatsunama.xmilo.dev

import com.facebook.react.bridge.Arguments
import com.facebook.react.bridge.WritableMap

data class XMiloclawRuntimeStatus(
  val notificationsGranted: Boolean,
  val appearOnTopGranted: Boolean,
  val batteryUnrestricted: Boolean,
  val accessibilityEnabled: Boolean,
  val runtimeHostStarted: Boolean,
  val bridgeConnected: Boolean,
  val hostReady: Boolean,
  val restartAttempted: Boolean = false,
  val restartSucceeded: Boolean? = null,
  val lastError: String? = null,
  val checkedAtMillis: Long = System.currentTimeMillis()
) {
  fun toWritableMap(): WritableMap =
    Arguments.createMap().apply {
      putBoolean("notificationsGranted", notificationsGranted)
      putBoolean("appearOnTopGranted", appearOnTopGranted)
      putBoolean("batteryUnrestricted", batteryUnrestricted)
      putBoolean("accessibilityEnabled", accessibilityEnabled)
      putBoolean("runtimeHostStarted", runtimeHostStarted)
      putBoolean("bridgeConnected", bridgeConnected)
      putBoolean("hostReady", hostReady)
      putBoolean("restartAttempted", restartAttempted)
      if (restartSucceeded != null) {
        putBoolean("restartSucceeded", restartSucceeded)
      }
      if (lastError != null) {
        putString("lastError", lastError)
      }
      putDouble("checkedAtMillis", checkedAtMillis.toDouble())
    }
}
