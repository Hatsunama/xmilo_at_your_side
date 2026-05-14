package com.hatsunama.xmilo.dev

import android.Manifest
import android.content.ComponentName
import android.content.Context
import android.content.Intent
import android.content.pm.PackageManager
import android.hardware.Sensor
import android.hardware.SensorManager
import android.os.Build
import android.provider.Settings
import androidx.core.content.ContextCompat
import com.facebook.react.bridge.Arguments
import com.facebook.react.bridge.Promise
import com.facebook.react.bridge.ReactApplicationContext
import com.facebook.react.bridge.ReactContextBaseJavaModule
import com.facebook.react.bridge.ReactMethod
import com.facebook.react.bridge.WritableArray
import com.facebook.react.bridge.WritableMap
import java.text.SimpleDateFormat
import java.util.Date
import java.util.Locale
import java.util.TimeZone
import org.json.JSONArray
import org.json.JSONObject

class XMiloclawRuntimeModule(private val reactContext: ReactApplicationContext) :
  ReactContextBaseJavaModule(reactContext) {

  override fun getName(): String = "XMiloclawRuntimeModule"

  @ReactMethod
  fun getStatus(promise: Promise) {
    try {
      val status = XMiloclawRuntimeController.snapshot(reactContext)
      promise.resolve(status.toProofWritableMap("runtime_host_status"))
    } catch (error: Exception) {
      promise.reject("XMILO_RUNTIME_STATUS_FAILED", error)
    }
  }

  @ReactMethod
  fun startRuntimeHost(promise: Promise) {
    try {
      val status = XMiloclawRuntimeController.start(reactContext)
      promise.resolve(status.toProofWritableMap("runtime_host_start"))
    } catch (error: Exception) {
      promise.reject("XMILO_RUNTIME_START_FAILED", error)
    }
  }

  @ReactMethod
  fun restartRuntimeHost(promise: Promise) {
    try {
      val status = XMiloclawRuntimeController.restart(reactContext)
      promise.resolve(status.toProofWritableMap("runtime_host_restart"))
    } catch (error: Exception) {
      promise.reject("XMILO_RUNTIME_RESTART_FAILED", error)
    }
  }

  @ReactMethod
  fun openAccessibilitySettings(promise: Promise) {
    try {
      val detailsIntent = Intent("android.settings.ACCESSIBILITY_DETAILS_SETTINGS").apply {
        addFlags(Intent.FLAG_ACTIVITY_NEW_TASK)
        val component = ComponentName(reactContext, XMiloclawAccessibilityService::class.java)
        putExtra("android.provider.extra.ACCESSIBILITY_COMPONENT_NAME", component.flattenToString())
      }

      try {
        reactContext.startActivity(detailsIntent)
      } catch (_: Exception) {
        val fallback = Intent(Settings.ACTION_ACCESSIBILITY_SETTINGS).apply {
          addFlags(Intent.FLAG_ACTIVITY_NEW_TASK)
        }
        reactContext.startActivity(fallback)
      }
      promise.resolve(true)
    } catch (error: Exception) {
      promise.reject("XMILO_RUNTIME_OPEN_ACCESSIBILITY_FAILED", error)
    }
  }

  @ReactMethod
  fun openNotificationSettings(promise: Promise) {
    try {
      val intent = Intent(Settings.ACTION_APP_NOTIFICATION_SETTINGS).apply {
        addFlags(Intent.FLAG_ACTIVITY_NEW_TASK)
        putExtra(Settings.EXTRA_APP_PACKAGE, reactContext.packageName)
      }
      reactContext.startActivity(intent)
      promise.resolve(true)
    } catch (error: Exception) {
      promise.reject("XMILO_RUNTIME_OPEN_NOTIFICATION_FAILED", error)
    }
  }

  @ReactMethod
  fun openOverlaySettings(promise: Promise) {
    try {
      val intent = Intent(Settings.ACTION_MANAGE_OVERLAY_PERMISSION).apply {
        addFlags(Intent.FLAG_ACTIVITY_NEW_TASK)
        data = android.net.Uri.parse("package:${reactContext.packageName}")
      }
      reactContext.startActivity(intent)
      promise.resolve(true)
    } catch (error: Exception) {
      promise.reject("XMILO_RUNTIME_OPEN_OVERLAY_FAILED", error)
    }
  }

  @ReactMethod
  fun openBatteryOptimizationSettings(promise: Promise) {
    try {
      val intent = Intent(Settings.ACTION_APPLICATION_DETAILS_SETTINGS).apply {
        addFlags(Intent.FLAG_ACTIVITY_NEW_TASK)
        data = android.net.Uri.parse("package:${reactContext.packageName}")
      }
      reactContext.startActivity(intent)
      promise.resolve(true)
    } catch (error: Exception) {
      promise.reject("XMILO_RUNTIME_OPEN_BATTERY_FAILED", error)
    }
  }

  @ReactMethod
  fun getLocalhostBearerToken(promise: Promise) {
    try {
      promise.resolve(XMiloclawSidecarProcessController.resolveBearerToken(reactContext))
    } catch (error: Exception) {
      promise.reject("XMILO_RUNTIME_LOCALHOST_TOKEN_FAILED", error)
    }
  }

  @ReactMethod
  fun saveLocalByokApiKey(provider: String, apiKey: String, baseUrl: String, model: String, promise: Promise) {
    try {
      promise.resolve(XMiloclawSidecarProcessController.saveLocalBYOKConfig(reactContext, provider, apiKey, baseUrl, model).toString())
    } catch (error: Exception) {
      promise.reject("XMILO_RUNTIME_SAVE_BYOK_KEY_FAILED", error)
    }
  }

  @ReactMethod
  fun deactivateLocalByokRouting(promise: Promise) {
    try {
      promise.resolve(XMiloclawSidecarProcessController.deactivateLocalBYOKRouting(reactContext).toString())
    } catch (error: Exception) {
      promise.reject("XMILO_RUNTIME_DEACTIVATE_BYOK_FAILED", error)
    }
  }

  @ReactMethod
  fun getLocalByokStatus(promise: Promise) {
    try {
      promise.resolve(XMiloclawSidecarProcessController.localBYOKStatus(reactContext).toString())
    } catch (error: Exception) {
      promise.reject("XMILO_RUNTIME_BYOK_STATUS_FAILED", error)
    }
  }

  @ReactMethod
  fun getCapabilityState(promise: Promise) {
    try {
      promise.resolve(buildCapabilityState().toString())
    } catch (error: Exception) {
      promise.reject("XMILO_CAPABILITY_STATE_FAILED", error)
    }
  }

  private fun XMiloclawRuntimeStatus.toProofWritableMap(operation: String): WritableMap {
    val map = toWritableMap()
    map.putMap("bridgeProof", jsonObjectToWritableMap(XMiloclawSidecarProcessController.runtimeStatusBridgeProof(this, operation)))
    return map
  }

  private fun buildCapabilityState(): JSONObject {
    val status = XMiloclawRuntimeController.snapshot(reactContext)
    val checkedAt = checkedAtIso()
    val sensorManager = reactContext.getSystemService(Context.SENSOR_SERVICE) as? SensorManager
    val cameraDeclared = isPermissionDeclared(Manifest.permission.CAMERA)
    val cameraGranted = isPermissionGranted(Manifest.permission.CAMERA)
    val microphoneDeclared = isPermissionDeclared(Manifest.permission.RECORD_AUDIO)
    val microphoneGranted = isPermissionGranted(Manifest.permission.RECORD_AUDIO)
    val fineLocationDeclared = isPermissionDeclared(Manifest.permission.ACCESS_FINE_LOCATION)
    val fineLocationGranted = isPermissionGranted(Manifest.permission.ACCESS_FINE_LOCATION)
    val coarseLocationGranted = isPermissionGranted(Manifest.permission.ACCESS_COARSE_LOCATION)
    val locationAccuracy = locationAccuracy(fineLocationGranted, coarseLocationGranted)
    val mediaAccess = mediaAccess()
    val capabilities = JSONObject()
      .put("camera_rear", permissionedCapability(
        permission = Manifest.permission.CAMERA,
        declared = cameraDeclared,
        granted = cameraGranted,
        available = reactContext.packageManager.hasSystemFeature(PackageManager.FEATURE_CAMERA_ANY),
        toolAvailable = false,
        tested = false,
        checkedAt = checkedAt,
        grantScope = permissionGrantScope(cameraGranted),
        acceptedForSetup = cameraDeclared && cameraGranted,
        repairHint = if (cameraGranted) "Permission is granted; Android does not expose one-time versus while-using scope to this build." else "Choose While using the app, not This time only.",
        permissionOnlyNote = "Camera permission state is inspectable, but Phase 9 does not include an app-owned camera capture tool."
      ))
      .put("camera_front", permissionedCapability(
        permission = Manifest.permission.CAMERA,
        declared = cameraDeclared,
        granted = cameraGranted,
        available = reactContext.packageManager.hasSystemFeature(PackageManager.FEATURE_CAMERA_FRONT),
        toolAvailable = false,
        tested = false,
        checkedAt = checkedAt,
        grantScope = permissionGrantScope(cameraGranted),
        acceptedForSetup = cameraDeclared && cameraGranted,
        repairHint = if (cameraGranted) "Permission is granted; Android does not expose one-time versus while-using scope to this build." else "Choose While using the app, not This time only.",
        permissionOnlyNote = "Front camera permission state is inspectable, but Phase 9 does not include an app-owned camera capture tool."
      ))
      .put("microphone", permissionedCapability(
        permission = Manifest.permission.RECORD_AUDIO,
        declared = microphoneDeclared,
        granted = microphoneGranted,
        available = reactContext.packageManager.hasSystemFeature(PackageManager.FEATURE_MICROPHONE),
        toolAvailable = false,
        tested = false,
        checkedAt = checkedAt,
        grantScope = permissionGrantScope(microphoneGranted),
        acceptedForSetup = microphoneDeclared && microphoneGranted,
        repairHint = if (microphoneGranted) "Permission is granted; Android does not expose one-time versus while-using scope to this build." else "Choose While using the app, not This time only.",
        permissionOnlyNote = "Microphone permission state is inspectable, but Phase 9 does not include an app-owned audio capture/readout tool."
      ))
      .put("location", permissionedCapability(
        permission = Manifest.permission.ACCESS_FINE_LOCATION,
        declared = fineLocationDeclared,
        granted = fineLocationGranted,
        available = true,
        toolAvailable = false,
        tested = false,
        checkedAt = checkedAt,
        grantScope = permissionGrantScope(fineLocationGranted || coarseLocationGranted),
        locationAccuracy = locationAccuracy,
        acceptedForSetup = fineLocationDeclared && fineLocationGranted && locationAccuracy == "precise",
        repairHint = when {
          locationAccuracy == "approximate" -> "Choose Precise location."
          fineLocationGranted -> "Permission is granted; Android does not expose one-time versus while-using scope to this build."
          else -> "Choose While using the app, not This time only. Choose Precise location."
        },
        permissionOnlyNote = "Location permission state is inspectable, but Phase 9 does not include an app-owned location readout tool."
      ))
      .put("media_library", JSONObject()
        .put("declared", isMediaDeclared())
        .put("requested", isMediaDeclared())
        .put("granted", mediaAccess == "all")
        .put("available", true)
        .put("tool_available", true)
        .put("tested", true)
        .put("media_access", mediaAccess)
        .put("accepted_for_setup", mediaAccess == "all")
        .put("last_verified_at", checkedAt)
        .put("failure_stage", if (mediaAccess == "all") JSONObject.NULL else "permission")
        .put("repair_hint", if (mediaAccess == "all") JSONObject.NULL else "Choose Allow all photos and videos, not limited access.")
        .put("note", "Media library setup accepts only all photos and videos access; limited selected-media access is not accepted for Phase 9 setup proof.")
      )
      .put("accelerometer", sensorCapability(sensorManager, Sensor.TYPE_ACCELEROMETER, checkedAt))
      .put("gyroscope", sensorCapability(sensorManager, Sensor.TYPE_GYROSCOPE, checkedAt))
      .put("magnetometer", sensorCapability(sensorManager, Sensor.TYPE_MAGNETIC_FIELD, checkedAt))
      .put("battery", basicCapability(available = true, toolAvailable = true, tested = true, checkedAt = checkedAt))
      .put("network", basicCapability(available = true, toolAvailable = true, tested = true, checkedAt = checkedAt))
      .put("file_access", basicCapability(available = true, toolAvailable = true, tested = true, checkedAt = checkedAt))
      .put("runtime_host", JSONObject()
        .put("available", status.hostReady)
        .put("tool_available", status.taskRouteSurfaceReady)
        .put("tested", true)
        .put("last_verified_at", checkedAt)
        .put("failure_stage", if (status.hostReady) JSONObject.NULL else "runtime")
        .put("note", if (status.hostReady) "Runtime host is verified." else (status.lastError ?: "Runtime host is not verified."))
      )

    val state = JSONObject()
      .put("schema_version", 1)
      .put("checked_at", checkedAt)
      .put("runtime_host", JSONObject()
        .put("online", status.hostReady)
        .put("version", BuildConfig.VERSION_NAME)
        .put("health", if (status.hostReady) "ready" else if (status.runtimeHostStarted) "degraded" else "offline")
      )
      .put("capabilities", capabilities)
    state.put("bridgeProof", jsonObjectToWritableMapCompatible(XMiloclawSidecarProcessController.runtimeStatusBridgeProof(status, "capability_state_snapshot")))
    return state
  }

  private fun permissionedCapability(
    permission: String,
    declared: Boolean,
    granted: Boolean,
    available: Boolean,
    toolAvailable: Boolean,
    tested: Boolean,
    checkedAt: String,
    grantScope: String? = null,
    locationAccuracy: String? = null,
    acceptedForSetup: Boolean? = null,
    repairHint: String? = null,
    permissionOnlyNote: String
  ): JSONObject =
    JSONObject()
      .put("declared", declared)
      .put("requested", declared)
      .put("granted", granted)
      .put("available", available)
      .put("tool_available", toolAvailable)
      .put("tested", tested)
      .put("last_verified_at", checkedAt)
      .apply {
        if (grantScope != null) put("grant_scope", grantScope)
        if (locationAccuracy != null) put("location_accuracy", locationAccuracy)
        if (acceptedForSetup != null) put("accepted_for_setup", acceptedForSetup)
        if (repairHint != null) put("repair_hint", repairHint)
      }
      .put("failure_stage", when {
        !declared -> "manifest"
        !granted -> "permission"
        !available -> "device"
        !toolAvailable -> "tool"
        !tested -> "tool"
        else -> JSONObject.NULL
      })
      .put("note", permissionOnlyNote)

  private fun sensorCapability(sensorManager: SensorManager?, sensorType: Int, checkedAt: String): JSONObject {
    val exists = sensorManager?.getDefaultSensor(sensorType) != null
    return JSONObject()
      .put("available", exists)
      .put("tool_available", false)
      .put("tested", false)
      .put("last_verified_at", checkedAt)
      .put("failure_stage", if (exists) "tool" else "device")
      .put("note", if (exists) "Sensor exists, but Phase 9 does not include a live app-owned sensor readout tool." else "Sensor is not reported by this device.")
  }

  private fun basicCapability(available: Boolean, toolAvailable: Boolean, tested: Boolean, checkedAt: String): JSONObject =
    JSONObject()
      .put("available", available)
      .put("tool_available", toolAvailable)
      .put("tested", tested)
      .put("last_verified_at", checkedAt)
      .put("failure_stage", if (available && toolAvailable && tested) JSONObject.NULL else "tool")

  private fun isPermissionDeclared(permission: String): Boolean {
    return try {
      val requested = if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.TIRAMISU) {
        val flags = PackageManager.PackageInfoFlags.of(PackageManager.GET_PERMISSIONS.toLong())
        reactContext.packageManager.getPackageInfo(reactContext.packageName, flags).requestedPermissions
      } else {
        @Suppress("DEPRECATION")
        reactContext.packageManager.getPackageInfo(reactContext.packageName, PackageManager.GET_PERMISSIONS).requestedPermissions
      }
      requested?.contains(permission) == true
    } catch (_: Exception) {
      false
    }
  }

  private fun isPermissionGranted(permission: String): Boolean =
    ContextCompat.checkSelfPermission(reactContext, permission) == PackageManager.PERMISSION_GRANTED

  private fun permissionGrantScope(granted: Boolean): String =
    if (granted) "unknown" else "denied"

  private fun locationAccuracy(fineGranted: Boolean, coarseGranted: Boolean): String =
    when {
      fineGranted -> "precise"
      coarseGranted -> "approximate"
      else -> "denied"
    }

  private fun isMediaDeclared(): Boolean {
    return if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.TIRAMISU) {
      isPermissionDeclared(Manifest.permission.READ_MEDIA_IMAGES) &&
        isPermissionDeclared(Manifest.permission.READ_MEDIA_VIDEO)
    } else {
      isPermissionDeclared(Manifest.permission.READ_EXTERNAL_STORAGE)
    }
  }

  private fun mediaAccess(): String {
    return if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.TIRAMISU) {
      val imagesGranted = isPermissionGranted(Manifest.permission.READ_MEDIA_IMAGES)
      val videoGranted = isPermissionGranted(Manifest.permission.READ_MEDIA_VIDEO)
      val limitedGranted =
        Build.VERSION.SDK_INT >= Build.VERSION_CODES.UPSIDE_DOWN_CAKE &&
          isPermissionGranted(Manifest.permission.READ_MEDIA_VISUAL_USER_SELECTED)
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
  }

  private fun checkedAtIso(): String {
    val formatter = SimpleDateFormat("yyyy-MM-dd'T'HH:mm:ss'Z'", Locale.US)
    formatter.timeZone = TimeZone.getTimeZone("UTC")
    return formatter.format(Date())
  }

  private fun jsonObjectToWritableMapCompatible(json: JSONObject): JSONObject = json

  private fun jsonObjectToWritableMap(json: JSONObject): WritableMap {
    val map = Arguments.createMap()
    val keys = json.keys()
    while (keys.hasNext()) {
      val key = keys.next()
      when (val value = json.opt(key)) {
        null, JSONObject.NULL -> map.putNull(key)
        is Boolean -> map.putBoolean(key, value)
        is Int -> map.putInt(key, value)
        is Long -> map.putDouble(key, value.toDouble())
        is Double -> map.putDouble(key, value)
        is Number -> map.putDouble(key, value.toDouble())
        is JSONObject -> map.putMap(key, jsonObjectToWritableMap(value))
        is JSONArray -> map.putArray(key, jsonArrayToWritableArray(value))
        else -> map.putString(key, value.toString())
      }
    }
    return map
  }

  private fun jsonArrayToWritableArray(json: JSONArray): WritableArray {
    val array = Arguments.createArray()
    for (index in 0 until json.length()) {
      when (val value = json.opt(index)) {
        null, JSONObject.NULL -> array.pushNull()
        is Boolean -> array.pushBoolean(value)
        is Int -> array.pushInt(value)
        is Long -> array.pushDouble(value.toDouble())
        is Double -> array.pushDouble(value)
        is Number -> array.pushDouble(value.toDouble())
        is JSONObject -> array.pushMap(jsonObjectToWritableMap(value))
        is JSONArray -> array.pushArray(jsonArrayToWritableArray(value))
        else -> array.pushString(value.toString())
      }
    }
    return array
  }
}
