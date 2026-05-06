package com.hatsunama.xmilo.dev

import com.facebook.react.bridge.Arguments
import com.facebook.react.bridge.WritableMap

data class XMiloclawRuntimeStatus(
  val notificationsGranted: Boolean,
  val appearOnTopGranted: Boolean,
  val batteryUnrestricted: Boolean,
  val accessibilityEnabled: Boolean,
  val runtimeHostStarted: Boolean,
  val foregroundServiceStarted: Boolean,
  val runtimeFilesPrepared: Boolean,
  val sidecarProcessLaunched: Boolean,
  val sidecarProcessAlive: Boolean,
  val bridgeConnected: Boolean,
  val taskRouteSurfaceReady: Boolean,
  val hostReady: Boolean,
  val lastRuntimeStage: String = "unknown",
  val lastHealthCode: Int? = null,
  val lastReadyCode: Int? = null,
  val lastHealthCategory: String = "unknown",
  val lastReadyCategory: String = "unknown",
  val lastProcessExitCode: Int? = null,
  val lastProcessUptimeMillis: Long? = null,
  val firstSafeStdoutLine: String? = null,
  val firstSafeStdoutCategory: String = "none",
  val firstSafeStderrLine: String? = null,
  val firstSafeStderrCategory: String = "none",
  val lastProcessErrorSummary: String? = null,
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
      putBoolean("foregroundServiceStarted", foregroundServiceStarted)
      putBoolean("runtimeFilesPrepared", runtimeFilesPrepared)
      putBoolean("sidecarProcessLaunched", sidecarProcessLaunched)
      putBoolean("sidecarProcessAlive", sidecarProcessAlive)
      putBoolean("bridgeConnected", bridgeConnected)
      putBoolean("taskRouteSurfaceReady", taskRouteSurfaceReady)
      putBoolean("hostReady", hostReady)
      putString("lastRuntimeStage", lastRuntimeStage)
      if (lastHealthCode != null) {
        putInt("lastHealthCode", lastHealthCode)
      }
      if (lastReadyCode != null) {
        putInt("lastReadyCode", lastReadyCode)
      }
      putString("lastHealthCategory", lastHealthCategory)
      putString("lastReadyCategory", lastReadyCategory)
      if (lastProcessExitCode != null) {
        putInt("lastProcessExitCode", lastProcessExitCode)
      }
      if (lastProcessUptimeMillis != null) {
        putDouble("lastProcessUptimeMillis", lastProcessUptimeMillis.toDouble())
      }
      if (firstSafeStdoutLine != null) {
        putString("firstSafeStdoutLine", firstSafeStdoutLine)
      }
      putString("firstSafeStdoutCategory", firstSafeStdoutCategory)
      if (firstSafeStderrLine != null) {
        putString("firstSafeStderrLine", firstSafeStderrLine)
      }
      putString("firstSafeStderrCategory", firstSafeStderrCategory)
      if (lastProcessErrorSummary != null) {
        putString("lastProcessErrorSummary", lastProcessErrorSummary)
      }
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
