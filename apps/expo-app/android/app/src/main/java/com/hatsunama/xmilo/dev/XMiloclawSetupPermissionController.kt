package com.hatsunama.xmilo.dev

import android.Manifest
import android.app.AppOpsManager
import android.content.Context
import android.content.pm.PackageManager
import android.os.Build
import android.util.Log
import androidx.core.content.ContextCompat
import com.facebook.react.bridge.ReactApplicationContext
import org.json.JSONArray
import org.json.JSONObject

class XMiloclawSetupPermissionController(
  private val reactContext: ReactApplicationContext
) {
  private val proofTag = "XMILO_SETUP_PERMISSION_PROOF"
  private val settingsSourcePrefs = reactContext.getSharedPreferences("xmilo_setup_permission_settings_source", Context.MODE_PRIVATE)
  private val settingsSourceSchemaVersion = 1
  private val settingsSourceTtlMillis = 10 * 60 * 1000L
  private val settingsSessionKey = "app_settings_session"
  private val mediaRequestAttemptedKey = "media_request_attempted"
  private val physicalActivityRequestTriedKey = "physical_activity_request_tried"
  private val runtimePopupUnverifiedGrants = mutableMapOf<String, Long>()

  private data class PermissionTruthDetails(
    val grantScope: String,
    val appOpMode: String,
    val acceptable: Boolean,
    val currentQuality: String
  )

  private data class SettingsAcceptance(
    val accepted: Boolean,
    val reason: String
  )

  fun requestPermissions(row: String): Array<String> =
    when (row) {
      "camera" -> arrayOf(Manifest.permission.CAMERA)
      "microphone" -> arrayOf(Manifest.permission.RECORD_AUDIO)
      "location" -> arrayOf(Manifest.permission.ACCESS_FINE_LOCATION, Manifest.permission.ACCESS_COARSE_LOCATION)
      "media" -> mediaRuntimeRequestPermissions()
      "physical_activity" -> physicalActivityRuntimeRequestPermissions()
      else -> emptyArray()
    }

  fun snapshot(status: XMiloclawRuntimeStatus, allowSettingsSourceAcceptance: Boolean = false): JSONObject {
    val rows = listOf(
      rowState("camera", status, allowSettingsSourceAcceptance),
      rowState("microphone", status, allowSettingsSourceAcceptance),
      rowState("media", status, allowSettingsSourceAcceptance),
      rowState("location", status, allowSettingsSourceAcceptance),
      rowState("physical_activity", status, allowSettingsSourceAcceptance),
      foregroundRuntimeRow(status)
    )
    val permissionRows = rows.filter { isSettingsCorrectionRow(it.optString("row")) }
    val requiredRowsComplete = permissionRows.all { !it.optBoolean("required", true) || it.optBoolean("complete", false) }
    val foregroundComplete = rows.firstOrNull { it.optString("row") == "foreground_runtime" }?.optBoolean("complete", false) == true
    if (rows.filter { isSettingsCorrectionRow(it.optString("row")) }.all { it.optBoolean("complete", false) }) {
      consumeSettingsSessionSource()
    }
    val blocked = JSONArray()
    rows.filter { it.optBoolean("required", true) && !it.optBoolean("complete", false) }.forEach {
      blocked.put("${it.optString("row")}:${it.optString("blocked_reason_key", "incomplete")}")
    }
    val finalReadyWithoutReview = requiredRowsComplete && foregroundComplete
    logProof(
      "snapshot_generated",
      mapOf(
        "schema_version" to "1",
        "permissions_complete" to requiredRowsComplete.toString(),
        "foreground_runtime_complete" to foregroundComplete.toString(),
        "final_ready_without_review" to finalReadyWithoutReview.toString(),
        "blocked_count" to blocked.length().toString()
      )
    )
    rows.forEach {
      logProof(
        "snapshot_row",
        mapOf(
          "row" to it.optString("row", "unknown"),
          "complete" to it.optBoolean("complete", false).toString(),
          "status" to it.optString("status", "unknown"),
          "allowed_action" to it.optString("allowed_action", "none"),
          "grant_scope" to it.optString("grant_scope", "not_applicable"),
          "media_access" to it.optString("media_access", "not_applicable"),
          "location_accuracy" to it.optString("location_accuracy", "not_applicable"),
          "app_op_mode" to it.optString("app_op_mode", "not_applicable"),
          "blocked_reason" to it.optString("blocked_reason_key", "none")
        )
      )
    }
    return JSONObject()
      .put("schema_version", 1)
      .put("generated_at_millis", System.currentTimeMillis())
      .put("rows", JSONArray(rows))
      .put(
        "gate",
        JSONObject()
          .put("permissions_complete", requiredRowsComplete)
          .put("foreground_runtime_complete", foregroundComplete)
          .put("review_required", true)
          .put("final_ready_without_review", finalReadyWithoutReview)
          .put("blocked_reasons", blocked)
      )
      .put("blocked_reasons", blocked)
      .put("allowed_actions", JSONArray(rows.map { JSONObject().put("row", it.optString("row")).put("action", it.optString("allowed_action")) }))
  }

  fun rowSnapshot(row: String): JSONObject = rowState(row, XMiloclawRuntimeController.snapshot(reactContext), false)

  fun onPermissionRequestCompleted(row: String, resultReason: String) {
    if (row != "camera" && row != "microphone" && row != "location" && row != "media" && row != "physical_activity") return
    if (row == "media") {
      settingsSourcePrefs.edit().putBoolean(mediaRequestAttemptedKey, true).apply()
      logProof("media_request_attempted", mapOf("state" to "true"))
    }
    if (row == "physical_activity") {
      settingsSourcePrefs.edit().putBoolean(physicalActivityRequestTriedKey, true).apply()
    }
    if ((row == "camera" || row == "microphone" || row == "location") && resultReason == "granted") {
      runtimePopupUnverifiedGrants[row] = System.currentTimeMillis()
      logProof("runtime_prompt_grant", mapOf("row" to row, "state" to "runtime_popup_granted_unverified"))
    } else if (row == "camera" || row == "microphone" || row == "location") {
      clearRuntimePopupGrant(row, resultReason)
    }
    logProof(
      "permission_request_completed",
      mapOf(
        "row" to row,
        "reason" to resultReason
      )
    )
  }

  fun onPermissionSettingsOpened(row: String, result: Boolean, reason: String) {
    if (result && isSettingsCorrectionRow(row)) {
      val now = System.currentTimeMillis()
      settingsSourcePrefs.edit()
        .putInt(settingsSourceSchemaKey(row), settingsSourceSchemaVersion)
        .putString(settingsSourceSourceKey(row), "settings_opened")
        .putLong(settingsSourceOpenedAtKey(row), now)
        .putBoolean(settingsSourceConsumedKey(row), false)
        .putInt(settingsSessionSchemaKey(), settingsSourceSchemaVersion)
        .putString(settingsSessionSourceKey(), "app_settings_opened")
        .putLong(settingsSessionOpenedAtKey(), now)
        .putString(settingsSessionOpenedFromRowKey(), row)
        .putBoolean(settingsSessionConsumedKey(), false)
        .apply()
      logProof(
        "settings_correction_source",
        mapOf(
          "row" to row,
          "state" to "started",
          "age_ms" to "0",
          "result" to "true"
        )
      )
      logProof(
        "settings_session_source",
        mapOf(
          "state" to "started",
          "opened_from_row" to row,
          "age_ms" to "0",
          "result" to "true"
        )
      )
    }
    logProof(
      "permission_settings_opened",
      mapOf(
        "row" to row,
        "result" to result.toString(),
        "reason" to reason
      )
    )
  }

  private fun rowState(row: String, status: XMiloclawRuntimeStatus, allowSettingsSourceAcceptance: Boolean): JSONObject =
    when (row) {
      "camera" -> runtimePermissionRow(row, Manifest.permission.CAMERA, isPermissionDeclared(Manifest.permission.CAMERA), isPermissionGranted(Manifest.permission.CAMERA), allowSettingsSourceAcceptance)
      "microphone" -> runtimePermissionRow(row, Manifest.permission.RECORD_AUDIO, isPermissionDeclared(Manifest.permission.RECORD_AUDIO), isPermissionGranted(Manifest.permission.RECORD_AUDIO), allowSettingsSourceAcceptance)
      "location" -> locationRow(allowSettingsSourceAcceptance)
      "media" -> mediaRow(allowSettingsSourceAcceptance)
      "physical_activity" -> physicalActivityRow(allowSettingsSourceAcceptance)
      "foreground_runtime" -> foregroundRuntimeRow(status)
      else -> unsupportedRow(row)
  }

  private fun runtimePermissionRow(row: String, permission: String, declared: Boolean, granted: Boolean, allowSettingsSourceAcceptance: Boolean): JSONObject {
    val appOpMode = if (declared && granted) appOpModeName(permission) else "not_applicable"
    val compatible = declared && granted && appOpMode != "denied"
    val runtimePopupBlocked = runtimePopupUnverifiedGrants.containsKey(row)
    if (runtimePopupBlocked && !compatible) {
      clearRuntimePopupGrant(row, if (appOpMode == "denied") "app_op_denied" else "revoked")
    }
    val recheckAcceptsCurrentTruth = allowSettingsSourceAcceptance && compatible
    val settingsAcceptance = if (runtimePopupBlocked && !allowSettingsSourceAcceptance) {
      settingsSourceAcceptance(row, compatible, allowSettingsSourceAcceptance)
    } else {
      SettingsAcceptance(false, "none")
    }
    val settingsAccepted = settingsAcceptance.accepted
    var markerCleared = false
    if (settingsAccepted) {
      clearRuntimePopupGrant(row, settingsAcceptance.reason)
      markerCleared = runtimePopupBlocked
    } else if (runtimePopupBlocked && recheckAcceptsCurrentTruth) {
      clearRuntimePopupGrant(row, "recheck_current_truth_accepted")
      markerCleared = true
    }
    if (allowSettingsSourceAcceptance) {
      logRecheckPermissionResult(row, compatible, markerCleared, runtimePopupBlocked)
    }
    val complete = compatible && (recheckAcceptsCurrentTruth || !runtimePopupBlocked || settingsAccepted)
    val baseTruthDetails = permissionTruthDetails(
      permission,
      declared,
      granted,
      appOpMode
    )
    val truthDetails = if (compatible && runtimePopupBlocked && recheckAcceptsCurrentTruth) {
      baseTruthDetails.copy(currentQuality = "recheck_current_truth_accepted")
    } else if (compatible && runtimePopupBlocked && !settingsAccepted) {
      baseTruthDetails.copy(currentQuality = "runtime_popup_grant_unverified")
    } else {
      baseTruthDetails
    }
    if (complete) {
      maybeConsumeSettingsCorrectionSource(row, allowSettingsSourceAcceptance)
    }
    val promptState = promptStateForRuntimePermission(declared, granted, complete, appOpMode)
    return rowJson(
      row = row,
      complete = complete,
      required = true,
      status = if (complete) "complete" else truthDetails.grantScope,
      grantScope = truthDetails.grantScope,
      mediaAccess = "not_applicable",
      locationAccuracy = "not_applicable",
      promptState = promptState,
      currentlyGranted = granted,
      truthDetails = truthDetails
    )
  }

  private fun locationRow(allowSettingsSourceAcceptance: Boolean): JSONObject {
    val fineDeclared = isPermissionDeclared(Manifest.permission.ACCESS_FINE_LOCATION)
    val fineGranted = isPermissionGranted(Manifest.permission.ACCESS_FINE_LOCATION)
    val coarseGranted = isPermissionGranted(Manifest.permission.ACCESS_COARSE_LOCATION)
    val accuracy = locationAccuracy(fineGranted, coarseGranted)
    val appOpMode = when {
      fineGranted -> appOpModeName(Manifest.permission.ACCESS_FINE_LOCATION)
      coarseGranted -> appOpModeName(Manifest.permission.ACCESS_COARSE_LOCATION)
      else -> "not_applicable"
    }
    val compatible = fineDeclared && fineGranted && accuracy == "precise" && appOpMode != "denied"
    val runtimePopupBlocked = runtimePopupUnverifiedGrants.containsKey("location")
    if (runtimePopupBlocked && !compatible) {
      val clearReason = when {
        appOpMode == "denied" -> "app_op_denied"
        accuracy == "approximate" -> "location_not_precise"
        else -> "revoked"
      }
      clearRuntimePopupGrant("location", clearReason)
    }
    val recheckAcceptsCurrentTruth = allowSettingsSourceAcceptance && compatible
    val settingsAcceptance = if (runtimePopupBlocked && !allowSettingsSourceAcceptance) {
      settingsSourceAcceptance("location", compatible, allowSettingsSourceAcceptance)
    } else {
      SettingsAcceptance(false, "none")
    }
    val settingsAccepted = settingsAcceptance.accepted
    var markerCleared = false
    if (settingsAccepted) {
      clearRuntimePopupGrant("location", settingsAcceptance.reason)
      markerCleared = runtimePopupBlocked
    } else if (runtimePopupBlocked && recheckAcceptsCurrentTruth) {
      clearRuntimePopupGrant("location", "recheck_current_truth_accepted")
      markerCleared = true
    }
    if (allowSettingsSourceAcceptance) {
      logRecheckPermissionResult("location", compatible, markerCleared, runtimePopupBlocked)
    }
    val complete = compatible && (recheckAcceptsCurrentTruth || !runtimePopupBlocked || settingsAccepted)
    val baseTruthDetails = permissionTruthDetails(
      Manifest.permission.ACCESS_FINE_LOCATION,
      fineDeclared,
      fineGranted,
      appOpMode,
      if (fineGranted) null else "denied"
    )
    val truthDetails = if (compatible && runtimePopupBlocked && recheckAcceptsCurrentTruth) {
      baseTruthDetails.copy(currentQuality = "recheck_current_truth_accepted")
    } else if (compatible && runtimePopupBlocked && !settingsAccepted) {
      baseTruthDetails.copy(currentQuality = "runtime_popup_grant_unverified")
    } else {
      baseTruthDetails
    }
    if (complete) {
      maybeConsumeSettingsCorrectionSource("location", allowSettingsSourceAcceptance)
    }
    return rowJson(
      row = "location",
      complete = complete,
      required = true,
      status = if (complete) "complete" else "${truthDetails.grantScope}_$accuracy",
      grantScope = truthDetails.grantScope,
      mediaAccess = "not_applicable",
      locationAccuracy = accuracy,
      promptState = promptStateForLocation(fineDeclared, fineGranted || coarseGranted, complete, appOpMode),
      currentlyGranted = fineGranted || coarseGranted,
      truthDetails = truthDetails
    )
  }

  private fun mediaRow(allowSettingsSourceAcceptance: Boolean): JSONObject {
    val access = mediaAccess()
    val complete = access == "all"
    val requestAttempted = settingsSourcePrefs.getBoolean(mediaRequestAttemptedKey, false)
    val promptState = when {
      complete -> "not_applicable"
      access == "denied" && !requestAttempted && mediaRuntimeRequestPermissions().isNotEmpty() -> "requestable"
      else -> "settings_required"
    }
    logProof("media_request_attempted", mapOf("state" to requestAttempted.toString()))
    val settingsSourceAccepted = settingsSourceAcceptance("media", complete, allowSettingsSourceAcceptance)
    if (complete && settingsSourceAccepted.accepted) consumeSettingsCorrectionSource("media")
    if (allowSettingsSourceAcceptance) {
      logRecheckPermissionResult("media", complete, false)
    }
    return rowJson(
      row = "media",
      complete = complete,
      required = true,
      status = if (complete) "complete" else access,
      grantScope = "not_applicable",
      mediaAccess = access,
      locationAccuracy = "not_applicable",
      promptState = promptState,
      currentlyGranted = access == "all" || access == "limited",
      truthDetails = mediaTruthDetails(access)
    )
  }

  private fun physicalActivityRow(allowSettingsSourceAcceptance: Boolean): JSONObject {
    if (Build.VERSION.SDK_INT < Build.VERSION_CODES.Q) {
      if (allowSettingsSourceAcceptance) {
        logRecheckPermissionResult("physical_activity", true, false)
      }
      return rowJson(
        row = "physical_activity",
        complete = true,
        required = true,
        status = "not_required",
        grantScope = "not_applicable",
        mediaAccess = "not_applicable",
        locationAccuracy = "not_applicable",
        promptState = "not_applicable",
        currentlyGranted = true,
        truthDetails = PermissionTruthDetails(
          grantScope = "not_applicable",
          appOpMode = "not_applicable",
          acceptable = true,
          currentQuality = "not_required"
        )
      )
    }
    val permission = Manifest.permission.ACTIVITY_RECOGNITION
    val declared = isPermissionDeclared(permission)
    val granted = isPermissionGranted(permission)
    val appOpMode = if (declared && granted) appOpModeName(permission) else "not_applicable"
    val compatible = declared && granted && appOpMode != "denied"
    val requestTried = settingsSourcePrefs.getBoolean(physicalActivityRequestTriedKey, false)
    val settingsSourceAccepted = if (compatible) {
      settingsSourceAcceptance("physical_activity", true, allowSettingsSourceAcceptance)
    } else {
      SettingsAcceptance(false, "none")
    }
    if (compatible && settingsSourceAccepted.accepted) consumeSettingsCorrectionSource("physical_activity")
    if (allowSettingsSourceAcceptance) {
      logRecheckPermissionResult("physical_activity", compatible, false)
    }
    val promptState = promptStateForPhysicalActivity(declared, granted, compatible, appOpMode, requestTried)
    return rowJson(
      row = "physical_activity",
      complete = compatible,
      required = true,
      status = if (compatible) "complete" else if (declared && granted) "while_using" else "denied",
      grantScope = if (declared && granted) "while_using" else "denied",
      mediaAccess = "not_applicable",
      locationAccuracy = "not_applicable",
      promptState = promptState,
      currentlyGranted = granted,
      truthDetails = permissionTruthDetails(
        permission,
        declared,
        granted,
        appOpMode
      )
    )
  }

  private fun foregroundRuntimeRow(status: XMiloclawRuntimeStatus): JSONObject {
    val complete = status.runtimeHostStarted && status.foregroundServiceStarted
    return JSONObject()
      .put("row", "foreground_runtime")
      .put("complete", complete)
      .put("required", true)
      .put("status", if (complete) "complete" else "missing")
      .put("grant_scope", "not_applicable")
      .put("media_access", "not_applicable")
      .put("location_accuracy", "not_applicable")
      .put("prompt_state", "not_applicable")
      .put("allowed_action", if (complete) "none" else "request")
      .put("blocked_reason_key", if (complete) "none" else "foreground_runtime_incomplete")
      .put("user_reason_key", if (complete) "foreground_runtime_ready" else "foreground_runtime_required")
      .put("requirement_text", "Must start the foreground runtime service")
      .put("accepted_text", "Foreground runtime service is running.")
  }

  private fun rowJson(
    row: String,
    complete: Boolean,
    required: Boolean,
    status: String,
    grantScope: String,
    mediaAccess: String,
    locationAccuracy: String,
    promptState: String,
    currentlyGranted: Boolean,
    truthDetails: PermissionTruthDetails
  ): JSONObject {
    val blockedReason = blockedReason(row, complete, grantScope, mediaAccess, locationAccuracy, promptState, truthDetails)
    val allowedAction = allowedAction(row, complete, promptState, mediaAccess, locationAccuracy, truthDetails)
    val proofReason = when {
      truthDetails.currentQuality == "recheck_current_truth_accepted" -> "recheck_current_truth_accepted"
      complete && (row == "camera" || row == "microphone" || row == "location" || (row == "physical_activity" && truthDetails.currentQuality != "not_required")) -> "current_truth_accepted"
      truthDetails.currentQuality == "runtime_popup_grant_unverified" -> "runtime_popup_grant_unverified"
      else -> blockedReason
    }
    logProof(
      "permission_truth",
      mapOf(
        "row" to row,
        "correct" to complete.toString(),
        "reason" to proofReason,
        "grant_scope" to grantScope,
        "media_access" to mediaAccess,
        "location_accuracy" to locationAccuracy,
        "app_op_mode" to truthDetails.appOpMode,
        "quality" to truthDetails.currentQuality
      )
    )
    logProof(
      "permission_action",
      mapOf("row" to row, "action" to allowedAction, "reason" to blockedReason)
    )
    if (!complete) {
      logProof("permission_blocked", mapOf("row" to row, "reason" to blockedReason))
    }
    return JSONObject()
      .put("row", row)
      .put("complete", complete)
      .put("required", required)
      .put("status", status)
      .put("category", status)
      .put("currently_granted", currentlyGranted)
      .put("grant_scope", grantScope)
      .put("media_access", mediaAccess)
      .put("location_accuracy", locationAccuracy)
      .put("prompt_state", promptState)
      .put("allowed_action", allowedAction)
      .put("blocked_reason_key", blockedReason)
      .put("user_reason_key", userReasonKey(row, complete, blockedReason))
      .put("can_request_now", allowedAction == "request")
      .put("app_op_mode", truthDetails.appOpMode)
      .put("requirement_text", requirementText(row))
      .put("accepted_text", acceptedText(row))
  }

  private fun allowedAction(
    row: String,
    complete: Boolean,
    promptState: String,
    mediaAccess: String,
    locationAccuracy: String,
    truthDetails: PermissionTruthDetails
  ): String =
    when {
      complete -> "none"
      truthDetails.currentQuality == "runtime_popup_grant_unverified" -> "open_settings"
      truthDetails.appOpMode == "denied" -> "open_settings"
      row == "media" && promptState == "requestable" -> "request"
      row == "media" -> "open_settings"
      row == "location" && locationAccuracy == "approximate" -> "open_settings"
      promptState == "settings_required" -> "open_settings"
      promptState == "requestable" -> "request"
      promptState == "permanent_denial" || promptState == "prompt_unavailable" -> "open_settings"
      else -> "none"
    }

  private fun blockedReason(
    row: String,
    complete: Boolean,
    grantScope: String,
    mediaAccess: String,
    locationAccuracy: String,
    promptState: String,
    truthDetails: PermissionTruthDetails
  ): String =
    when {
      complete -> "none"
      truthDetails.currentQuality == "runtime_popup_grant_unverified" -> "settings_verification_required"
      truthDetails.appOpMode == "denied" -> "app_op_denied"
      row == "media" && mediaAccess == "limited" -> "limited_media"
      row == "media" -> "media_full_access_required"
      row == "location" && locationAccuracy == "approximate" -> "location_precise_required"
      row == "location" && grantScope == "denied" -> "location_permission_required"
      promptState == "prompt_unavailable" -> "${row}_permission_unavailable"
      grantScope == "denied" -> "${row}_permission_required"
      else -> "${row}_incomplete"
    }

  private fun userReasonKey(row: String, complete: Boolean, blockedReason: String): String =
    if (complete) "${row}_complete" else blockedReason

  private fun unsupportedRow(row: String): JSONObject =
    JSONObject()
      .put("row", row)
      .put("complete", false)
      .put("required", false)
      .put("status", "unsupported")
      .put("allowed_action", "none")
      .put("blocked_reason_key", "unsupported_row")

  private fun promptStateForRuntimePermission(declared: Boolean, granted: Boolean, complete: Boolean, appOpMode: String): String =
    when {
      complete -> "not_applicable"
      !declared -> "prompt_unavailable"
      appOpMode == "denied" -> "permanent_denial"
      granted -> "settings_required"
      else -> "requestable"
    }

  private fun promptStateForLocation(declared: Boolean, granted: Boolean, complete: Boolean, appOpMode: String): String =
    when {
      complete -> "not_applicable"
      !declared -> "prompt_unavailable"
      appOpMode == "denied" -> "permanent_denial"
      granted -> "settings_required"
      else -> "requestable"
    }

  private fun promptStateForPhysicalActivity(declared: Boolean, granted: Boolean, complete: Boolean, appOpMode: String, requestTried: Boolean): String =
    when {
      complete -> "not_applicable"
      !declared -> "prompt_unavailable"
      appOpMode == "denied" -> "permanent_denial"
      granted -> "settings_required"
      requestTried -> "settings_required"
      else -> "requestable"
    }

  private fun permissionTruthDetails(
    permission: String,
    declared: Boolean,
    granted: Boolean,
    appOpMode: String,
    scopeOverride: String? = null
  ): PermissionTruthDetails {
    val grantScope = scopeOverride ?: when {
      appOpMode == "denied" -> "denied"
      declared && granted -> "while_using"
      else -> "denied"
    }
    val acceptable = declared && granted && appOpMode != "denied"
    val quality = when {
      appOpMode == "denied" -> "app_op_denied"
      acceptable -> "acceptable"
      else -> "not_granted"
    }
    return PermissionTruthDetails(
      grantScope = grantScope,
      appOpMode = appOpMode,
      acceptable = acceptable,
      currentQuality = quality
    )
  }

  private fun mediaTruthDetails(access: String): PermissionTruthDetails =
    PermissionTruthDetails(
      grantScope = "not_applicable",
      appOpMode = "not_applicable",
      acceptable = access == "all",
      currentQuality = when (access) {
        "all" -> "acceptable"
        "limited" -> "limited"
        else -> "not_granted"
      }
    )

  private fun isSettingsCorrectionRow(row: String): Boolean =
    row == "camera" || row == "microphone" || row == "location" || row == "media" || row == "physical_activity"

  private fun maybeConsumeSettingsCorrectionSource(row: String, allowSettingsSourceAcceptance: Boolean) {
    if (!allowSettingsSourceAcceptance) return
    if (readSettingsCorrectionSource(row) == "valid") consumeSettingsCorrectionSource(row)
  }

  private fun settingsSourceAcceptance(row: String, compatible: Boolean, allowSettingsSourceAcceptance: Boolean): SettingsAcceptance {
    if (!isSettingsCorrectionRow(row)) return SettingsAcceptance(false, "none")
    val rowState = readSettingsCorrectionSource(row)
    val sessionState = readSettingsSessionSource()
    val sessionOpenedFromRow = settingsSourcePrefs.getString(settingsSessionOpenedFromRowKey(), "") ?: ""
    val sessionOpenedFromCorrectionRow = isSettingsCorrectionRow(sessionOpenedFromRow)
    val consumed = settingsSourcePrefs.getBoolean(settingsSourceConsumedKey(row), false)
    val sessionConsumed = settingsSourcePrefs.getBoolean(settingsSessionConsumedKey(), false)
    if (!compatible) {
      if (consumed) clearSettingsCorrectionSource(row)
      if (sessionConsumed && sessionOpenedFromRow == row) clearSettingsSessionSource()
      logProof(
        "settings_correction_source",
        mapOf(
          "row" to row,
          "state" to "eligible",
          "result" to "false"
        )
      )
      logProof(
        "settings_session_source",
        mapOf(
          "row" to row,
          "state" to "eligible",
          "same_row" to (sessionOpenedFromRow == row).toString(),
          "result" to "false"
        )
      )
      return SettingsAcceptance(false, "none")
    }
    val rowCanAccept = rowState == "valid" && (consumed || allowSettingsSourceAcceptance)
    val sameRowSessionCanAccept = sessionState == "valid" && sessionOpenedFromCorrectionRow && sessionOpenedFromRow == row && (sessionConsumed || allowSettingsSourceAcceptance)
    val globalRecheckSessionCanAccept = sessionState == "valid" && sessionOpenedFromCorrectionRow && allowSettingsSourceAcceptance && sessionOpenedFromRow != row
    val sessionCanAccept = sameRowSessionCanAccept || globalRecheckSessionCanAccept
    val sessionReason = when {
      sameRowSessionCanAccept -> "settings_accepted"
      globalRecheckSessionCanAccept -> "global_settings_recheck_current_truth"
      else -> "none"
    }
    val sourceScope = when {
      rowCanAccept -> "row_settings_source"
      sameRowSessionCanAccept -> "same_row_settings_session"
      globalRecheckSessionCanAccept -> "global_settings_recheck_current_truth"
      else -> "none"
    }
    logProof(
      "settings_correction_source",
      mapOf(
        "row" to row,
        "state" to "eligible",
        "result" to rowCanAccept.toString()
      )
    )
    logProof(
      "settings_session_source",
      mapOf(
        "row" to row,
        "state" to "eligible",
        "same_row" to (sessionOpenedFromRow == row).toString(),
        "global_recheck" to globalRecheckSessionCanAccept.toString(),
        "source_scope" to sourceScope,
        "result" to sessionCanAccept.toString()
      )
    )
    return when {
      rowCanAccept -> SettingsAcceptance(true, "settings_accepted")
      sessionCanAccept -> SettingsAcceptance(true, sessionReason)
      else -> SettingsAcceptance(false, "none")
    }
  }

  private fun logRecheckPermissionResult(row: String, compatible: Boolean, markerCleared: Boolean, markerIgnored: Boolean = false) {
    logProof(
      "recheck_permissions_result",
      mapOf(
        "row" to row,
        "current_truth" to if (compatible) "compatible" else "incompatible",
        "marker_ignored" to markerIgnored.toString(),
        "marker_cleared" to markerCleared.toString()
      )
    )
  }

  private fun readSettingsCorrectionSource(row: String): String {
    val schema = settingsSourcePrefs.getInt(settingsSourceSchemaKey(row), 0)
    val source = settingsSourcePrefs.getString(settingsSourceSourceKey(row), "")
    val openedAt = settingsSourcePrefs.getLong(settingsSourceOpenedAtKey(row), 0L)
    if (schema != settingsSourceSchemaVersion || source != "settings_opened" || openedAt <= 0L) {
      return "missing"
    }
    val age = System.currentTimeMillis() - openedAt
    if (age > settingsSourceTtlMillis) {
      clearSettingsCorrectionSource(row)
      logProof(
        "settings_correction_source",
        mapOf(
          "row" to row,
          "state" to "expired",
          "age_ms" to age.toString(),
          "result" to "false"
        )
      )
      return "expired"
    }
    return "valid"
  }

  private fun consumeSettingsCorrectionSource(row: String) {
    if (settingsSourcePrefs.getBoolean(settingsSourceConsumedKey(row), false)) return
    settingsSourcePrefs.edit().putBoolean(settingsSourceConsumedKey(row), true).apply()
    logProof("settings_correction_source", mapOf("row" to row, "state" to "consumed", "result" to "true"))
  }

  private fun readSettingsSessionSource(): String {
    val schema = settingsSourcePrefs.getInt(settingsSessionSchemaKey(), 0)
    val source = settingsSourcePrefs.getString(settingsSessionSourceKey(), "")
    val openedAt = settingsSourcePrefs.getLong(settingsSessionOpenedAtKey(), 0L)
    if (schema != settingsSourceSchemaVersion || source != "app_settings_opened" || openedAt <= 0L) {
      return "missing"
    }
    val age = System.currentTimeMillis() - openedAt
    if (age > settingsSourceTtlMillis) {
      clearSettingsSessionSource()
      logProof(
        "settings_session_source",
        mapOf(
          "state" to "expired",
          "age_ms" to age.toString(),
          "result" to "false"
        )
      )
      return "expired"
    }
    return "valid"
  }

  private fun consumeSettingsSessionSource() {
    if (settingsSourcePrefs.getBoolean(settingsSessionConsumedKey(), false)) return
    if (readSettingsSessionSource() != "valid") return
    settingsSourcePrefs.edit().putBoolean(settingsSessionConsumedKey(), true).apply()
    logProof("settings_session_source", mapOf("state" to "consumed", "result" to "true"))
  }

  private fun clearSettingsCorrectionSource(row: String) {
    settingsSourcePrefs.edit()
      .remove(settingsSourceSchemaKey(row))
      .remove(settingsSourceSourceKey(row))
      .remove(settingsSourceOpenedAtKey(row))
      .remove(settingsSourceConsumedKey(row))
      .apply()
  }

  private fun clearSettingsSessionSource() {
    settingsSourcePrefs.edit()
      .remove(settingsSessionSchemaKey())
      .remove(settingsSessionSourceKey())
      .remove(settingsSessionOpenedAtKey())
      .remove(settingsSessionOpenedFromRowKey())
      .remove(settingsSessionConsumedKey())
      .apply()
  }

  private fun settingsSourceSchemaKey(row: String): String = "settings_source_schema_$row"

  private fun settingsSourceSourceKey(row: String): String = "settings_source_source_$row"

  private fun settingsSourceOpenedAtKey(row: String): String = "settings_source_opened_at_$row"

  private fun settingsSourceConsumedKey(row: String): String = "settings_source_consumed_$row"

  private fun settingsSessionSchemaKey(): String = "${settingsSessionKey}_schema"

  private fun settingsSessionSourceKey(): String = "${settingsSessionKey}_source"

  private fun settingsSessionOpenedAtKey(): String = "${settingsSessionKey}_opened_at"

  private fun settingsSessionOpenedFromRowKey(): String = "${settingsSessionKey}_opened_from_row"

  private fun settingsSessionConsumedKey(): String = "${settingsSessionKey}_consumed"

  private fun clearRuntimePopupGrant(row: String, reason: String) {
    if (runtimePopupUnverifiedGrants.remove(row) != null) {
      logProof("runtime_prompt_grant", mapOf("row" to row, "state" to "cleared", "reason" to reason))
    }
  }

  private fun requirementText(row: String): String =
    when (row) {
      "camera" -> "Must give Camera access"
      "microphone" -> "Must give Microphone access"
      "media" -> "Must give Photos and videos full access"
      "location" -> "Must give Location: Precise and While using the app"
      "physical_activity" -> "Must give Physical activity access"
      "foreground_runtime" -> "Must start the foreground runtime service"
      else -> "Must complete this requirement"
    }

  private fun acceptedText(row: String): String =
    when (row) {
      "camera" -> "Camera access is verified by Android setup proof."
      "microphone" -> "Microphone access is verified by Android setup proof."
      "media" -> "Photos and videos are set to full access."
      "location" -> "Location is set to Precise and While using the app."
      "physical_activity" -> "Physical activity access is verified by Android setup proof."
      "foreground_runtime" -> "Foreground runtime service is running."
      else -> "Requirement is complete."
    }

  private fun mediaRuntimeRequestPermissions(): Array<String> =
    when {
      Build.VERSION.SDK_INT >= Build.VERSION_CODES.UPSIDE_DOWN_CAKE -> arrayOf(Manifest.permission.READ_MEDIA_IMAGES, Manifest.permission.READ_MEDIA_VIDEO, Manifest.permission.READ_MEDIA_VISUAL_USER_SELECTED)
      Build.VERSION.SDK_INT >= Build.VERSION_CODES.TIRAMISU -> arrayOf(Manifest.permission.READ_MEDIA_IMAGES, Manifest.permission.READ_MEDIA_VIDEO)
      else -> arrayOf(Manifest.permission.READ_EXTERNAL_STORAGE)
    }

  private fun physicalActivityRuntimeRequestPermissions(): Array<String> =
    if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.Q) arrayOf(Manifest.permission.ACTIVITY_RECOGNITION) else emptyArray()

  private fun isPermissionDeclared(permission: String): Boolean =
    try {
      val requested = if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.TIRAMISU) {
        reactContext.packageManager.getPackageInfo(reactContext.packageName, PackageManager.PackageInfoFlags.of(PackageManager.GET_PERMISSIONS.toLong())).requestedPermissions
      } else {
        @Suppress("DEPRECATION")
        reactContext.packageManager.getPackageInfo(reactContext.packageName, PackageManager.GET_PERMISSIONS).requestedPermissions
      }
      requested?.contains(permission) == true
    } catch (_: Exception) {
      false
    }

  private fun isPermissionGranted(permission: String): Boolean =
    ContextCompat.checkSelfPermission(reactContext, permission) == PackageManager.PERMISSION_GRANTED

  private fun isMediaDeclared(): Boolean =
    if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.TIRAMISU) {
      isPermissionDeclared(Manifest.permission.READ_MEDIA_IMAGES) && isPermissionDeclared(Manifest.permission.READ_MEDIA_VIDEO)
    } else {
      isPermissionDeclared(Manifest.permission.READ_EXTERNAL_STORAGE)
    }

  private fun mediaAccess(): String =
    if (!isMediaDeclared()) {
      "denied"
    } else if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.TIRAMISU) {
      val imagesGranted = isPermissionGranted(Manifest.permission.READ_MEDIA_IMAGES)
      val videoGranted = isPermissionGranted(Manifest.permission.READ_MEDIA_VIDEO)
      val limitedGranted = Build.VERSION.SDK_INT >= Build.VERSION_CODES.UPSIDE_DOWN_CAKE && isPermissionGranted(Manifest.permission.READ_MEDIA_VISUAL_USER_SELECTED)
      when {
        imagesGranted && videoGranted -> "all"
        limitedGranted -> "limited"
        else -> "denied"
      }
    } else if (isPermissionGranted(Manifest.permission.READ_EXTERNAL_STORAGE)) {
      "all"
    } else {
      "denied"
    }

  private fun locationAccuracy(fineGranted: Boolean, coarseGranted: Boolean): String =
    when {
      fineGranted -> "precise"
      coarseGranted -> "approximate"
      else -> "denied"
    }

  private fun appOpModeName(permission: String): String {
    val op = AppOpsManager.permissionToOp(permission) ?: return "unknown"
    val appOps = reactContext.getSystemService(Context.APP_OPS_SERVICE) as? AppOpsManager ?: return "unknown"
    val mode = try {
      if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.Q) {
        val method = AppOpsManager::class.java.getMethod("unsafeCheckOpRaw", String::class.java, Int::class.javaPrimitiveType, String::class.java)
        method.invoke(appOps, op, android.os.Process.myUid(), reactContext.packageName) as? Int
      } else {
        @Suppress("DEPRECATION")
        appOps.checkOpNoThrow(op, android.os.Process.myUid(), reactContext.packageName)
      }
    } catch (_: Exception) {
      try {
        @Suppress("DEPRECATION")
        appOps.checkOpNoThrow(op, android.os.Process.myUid(), reactContext.packageName)
      } catch (_: Exception) {
        null
      }
    } ?: return "unknown"
    return when (mode) {
      AppOpsManager.MODE_ALLOWED -> "allowed"
      AppOpsManager.MODE_FOREGROUND -> "foreground"
      AppOpsManager.MODE_IGNORED, AppOpsManager.MODE_ERRORED -> "denied"
      AppOpsManager.MODE_DEFAULT -> "default"
      else -> "unknown"
    }
  }

  private fun logProof(event: String, fields: Map<String, String>) {
    val parts = mutableListOf("event=$event")
    for ((key, value) in fields) parts.add("$key=${safeProofValue(value)}")
    Log.i(proofTag, parts.joinToString(" "))
  }

  private fun safeProofValue(value: String): String =
    value.replace(Regex("[^A-Za-z0-9_.:-]"), "_").take(80)

}
