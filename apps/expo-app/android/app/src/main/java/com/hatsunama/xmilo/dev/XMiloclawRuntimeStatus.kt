package com.hatsunama.xmilo.dev

import com.facebook.react.bridge.Arguments
import com.facebook.react.bridge.WritableMap

data class XMiloclawRuntimeStatus(
  val notificationsGranted: Boolean,
  val appearOnTopGranted: Boolean,
  val batteryUnrestricted: Boolean,
  val dataSaverUnrestricted: Boolean,
  val dataSaverStatus: String,
  val accessibilityEnabled: Boolean,
  val runtimeHostStarted: Boolean,
  val foregroundServiceStarted: Boolean,
  val runtimeFilesPrepared: Boolean,
  val sidecarProcessLaunched: Boolean,
  val sidecarProcessAlive: Boolean,
  val bridgeConnected: Boolean,
  val taskRouteSurfaceReady: Boolean,
  val hostReady: Boolean,
  val llmMode: String? = null,
  val byokProvider: String? = null,
  val subscriptionEntitled: Boolean? = null,
  val bringYourOwnKeyActive: Boolean? = null,
  val phase9ApiKeyAccess: Boolean? = null,
  val firstTaskEligible: Boolean? = null,
  val relayLlmTurnAllowed: Boolean? = null,
  val localLlmTurnAllowed: Boolean? = null,
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
      putBoolean("dataSaverUnrestricted", dataSaverUnrestricted)
      putString("dataSaverStatus", dataSaverStatus)
      putBoolean("accessibilityEnabled", accessibilityEnabled)
      putBoolean("runtimeHostStarted", runtimeHostStarted)
      putBoolean("foregroundServiceStarted", foregroundServiceStarted)
      putBoolean("runtimeFilesPrepared", runtimeFilesPrepared)
      putBoolean("sidecarProcessLaunched", sidecarProcessLaunched)
      putBoolean("sidecarProcessAlive", sidecarProcessAlive)
      putBoolean("bridgeConnected", bridgeConnected)
      putBoolean("taskRouteSurfaceReady", taskRouteSurfaceReady)
      putBoolean("hostReady", hostReady)
      if (llmMode != null) {
        putString("llmMode", llmMode)
      }
      if (byokProvider != null) {
        putString("byokProvider", byokProvider)
      }
      if (subscriptionEntitled != null) {
        putBoolean("subscriptionEntitled", subscriptionEntitled)
      }
      if (bringYourOwnKeyActive != null) {
        putBoolean("bringYourOwnKeyActive", bringYourOwnKeyActive)
      }
      if (phase9ApiKeyAccess != null) {
        putBoolean("phase9ApiKeyAccess", phase9ApiKeyAccess)
      }
      if (firstTaskEligible != null) {
        putBoolean("firstTaskEligible", firstTaskEligible)
      }
      if (relayLlmTurnAllowed != null) {
        putBoolean("relayLlmTurnAllowed", relayLlmTurnAllowed)
      }
      if (localLlmTurnAllowed != null) {
        putBoolean("localLlmTurnAllowed", localLlmTurnAllowed)
      }
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
