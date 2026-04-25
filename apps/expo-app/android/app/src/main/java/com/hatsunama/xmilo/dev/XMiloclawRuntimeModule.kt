package com.hatsunama.xmilo.dev

import android.content.ComponentName
import android.content.Intent
import android.provider.Settings
import com.facebook.react.bridge.Promise
import com.facebook.react.bridge.ReactApplicationContext
import com.facebook.react.bridge.ReactContextBaseJavaModule
import com.facebook.react.bridge.ReactMethod
import java.io.File
import java.util.UUID

class XMiloclawRuntimeModule(private val reactContext: ReactApplicationContext) :
  ReactContextBaseJavaModule(reactContext) {

  override fun getName(): String = "XMiloclawRuntimeModule"

  @ReactMethod
  fun getStatus(promise: Promise) {
    try {
      promise.resolve(XMiloclawRuntimeController.snapshot(reactContext).toWritableMap())
    } catch (error: Exception) {
      promise.reject("XMILO_RUNTIME_STATUS_FAILED", error)
    }
  }

  @ReactMethod
  fun startRuntimeHost(promise: Promise) {
    try {
      promise.resolve(XMiloclawRuntimeController.start(reactContext).toWritableMap())
    } catch (error: Exception) {
      promise.reject("XMILO_RUNTIME_START_FAILED", error)
    }
  }

  @ReactMethod
  fun restartRuntimeHost(promise: Promise) {
    try {
      promise.resolve(XMiloclawRuntimeController.restart(reactContext).toWritableMap())
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
      val prefs = reactContext.getSharedPreferences("xmilo_runtime_host", android.content.Context.MODE_PRIVATE)
      var token = prefs.getString("localhost_bearer_token", null)
      if (token.isNullOrBlank()) {
        token = UUID.randomUUID().toString()
        prefs.edit().putString("localhost_bearer_token", token).apply()
      }
      try {
        File(reactContext.filesDir, "xmilo_localhost_bearer_token.txt").writeText(token)
      } catch (_: Exception) {
      }
      promise.resolve(token)
    } catch (error: Exception) {
      promise.reject("XMILO_RUNTIME_LOCALHOST_TOKEN_FAILED", error)
    }
  }
}
