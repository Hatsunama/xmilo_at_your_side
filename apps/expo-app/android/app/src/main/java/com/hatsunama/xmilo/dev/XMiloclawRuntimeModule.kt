package com.hatsunama.xmilo.dev

import android.content.ComponentName
import android.content.Intent
import android.provider.Settings
import com.facebook.react.bridge.Arguments
import com.facebook.react.bridge.Promise
import com.facebook.react.bridge.ReactApplicationContext
import com.facebook.react.bridge.ReactContextBaseJavaModule
import com.facebook.react.bridge.ReactMethod
import com.facebook.react.bridge.WritableArray
import com.facebook.react.bridge.WritableMap
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
  fun getLocalByokStatus(promise: Promise) {
    try {
      promise.resolve(XMiloclawSidecarProcessController.localBYOKStatus(reactContext).toString())
    } catch (error: Exception) {
      promise.reject("XMILO_RUNTIME_BYOK_STATUS_FAILED", error)
    }
  }

  private fun XMiloclawRuntimeStatus.toProofWritableMap(operation: String): WritableMap {
    val map = toWritableMap()
    map.putMap("bridgeProof", jsonObjectToWritableMap(XMiloclawSidecarProcessController.runtimeStatusBridgeProof(this, operation)))
    return map
  }

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
