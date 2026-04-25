package com.hatsunama.xmilo.dev

import android.content.ComponentName
import android.content.Context
import android.content.Intent
import android.content.ServiceConnection
import android.os.IBinder
import android.provider.Settings
import android.util.Log
import androidx.core.content.ContextCompat
import java.util.concurrent.atomic.AtomicBoolean
import java.util.concurrent.CountDownLatch
import java.util.concurrent.TimeUnit

object XMiloclawRuntimeController {
  private const val BIND_TIMEOUT_MS = 2_000L
  private const val TAG = "XMiloclawRuntime"

  @Volatile
  private var lastRestartAttempted = false

  @Volatile
  private var lastRestartSucceeded: Boolean? = null

  @Volatile
  private var lastError: String? = null

  fun snapshot(context: Context): XMiloclawRuntimeStatus {
    val notificationsGranted = XMiloclawNotificationController.areNotificationsGranted(context)
    val appearOnTopGranted = if (android.os.Build.VERSION.SDK_INT >= android.os.Build.VERSION_CODES.M) {
      Settings.canDrawOverlays(context)
    } else {
      true
    }
    val batteryUnrestricted = if (android.os.Build.VERSION.SDK_INT >= android.os.Build.VERSION_CODES.M) {
      val powerManager = context.getSystemService(Context.POWER_SERVICE) as android.os.PowerManager
      powerManager.isIgnoringBatteryOptimizations(context.packageName)
    } else {
      true
    }
    val accessibilityEnabled = XMiloclawAccessibilityService.isEnabled(context)
    val runtimeHostStarted = XMiloclawRuntimeService.isRunning
    val bridgeConnected = XMiloclawSidecarController.readyOk()
    val hostReady =
      notificationsGranted &&
        appearOnTopGranted &&
        batteryUnrestricted &&
        accessibilityEnabled &&
        runtimeHostStarted &&
        bridgeConnected

    val status =
      XMiloclawRuntimeStatus(
      notificationsGranted = notificationsGranted,
      appearOnTopGranted = appearOnTopGranted,
      batteryUnrestricted = batteryUnrestricted,
      accessibilityEnabled = accessibilityEnabled,
      runtimeHostStarted = runtimeHostStarted,
      bridgeConnected = bridgeConnected,
      hostReady = hostReady,
      restartAttempted = lastRestartAttempted,
      restartSucceeded = lastRestartSucceeded,
      lastError = lastError
    )
    Log.d(TAG, "snapshot runtimeHostStarted=$runtimeHostStarted bridgeConnected=$bridgeConnected hostReady=$hostReady lastError=${lastError ?: ""}")
    return status
  }

  fun start(context: Context): XMiloclawRuntimeStatus {
    val appContext = context.applicationContext
    lastRestartAttempted = false
    lastRestartSucceeded = null
    lastError = null

    Log.d(TAG, "startRuntimeHost invoked")
    ContextCompat.startForegroundService(appContext, Intent(appContext, XMiloclawRuntimeService::class.java))
    val bridged = waitForBridge(appContext)
    Log.d(TAG, "startRuntimeHost waitForBridge bridged=$bridged")
    XMiloclawSidecarController.ensureRunning(context)
    if (!XMiloclawSidecarController.readyOk()) {
      lastError = XMiloclawSidecarController.getLastError() ?: "localhost bridge not ready"
    }
    return snapshot(appContext)
  }

  fun restart(context: Context): XMiloclawRuntimeStatus {
    val appContext = context.applicationContext
    lastRestartAttempted = true
    lastRestartSucceeded = false
    lastError = null

    appContext.stopService(Intent(appContext, XMiloclawRuntimeService::class.java))
    XMiloclawRuntimeService.bridgeConnected = false
    XMiloclawRuntimeService.isRunning = false

    try {
      Log.d(TAG, "restartRuntimeHost invoked")
      ContextCompat.startForegroundService(appContext, Intent(appContext, XMiloclawRuntimeService::class.java))
      val bridged = waitForBridge(appContext)
      lastRestartSucceeded = bridged
      if (!bridged) {
        lastError = "bridge timeout"
      }
      XMiloclawSidecarController.ensureRunning(appContext)
      if (!XMiloclawSidecarController.readyOk()) {
        lastError = XMiloclawSidecarController.getLastError() ?: "localhost bridge not ready"
      }
    } catch (error: Exception) {
      lastRestartSucceeded = false
      lastError = error.message ?: "restart failed"
    }
    Log.d(TAG, "restartRuntimeHost result restartSucceeded=$lastRestartSucceeded lastError=${lastError ?: ""}")

    return snapshot(appContext)
  }

  private fun waitForBridge(context: Context): Boolean {
    val latch = CountDownLatch(1)
    val handshakeOpen = AtomicBoolean(true)
    var bridged = false
    var bound = false
    val connection =
      object : ServiceConnection {
        override fun onServiceConnected(name: ComponentName?, service: IBinder?) {
          if (!handshakeOpen.getAndSet(false)) return
          bridged = (service as? XMiloclawRuntimeService.RuntimeBinder)?.ping() == true
          XMiloclawRuntimeService.bridgeConnected = bridged
          Log.d(TAG, "onServiceConnected bridged=$bridged binderClass=${service?.javaClass?.name ?: ""}")
          latch.countDown()
        }

        override fun onServiceDisconnected(name: ComponentName?) {
          XMiloclawRuntimeService.bridgeConnected = false
          Log.w(TAG, "onServiceDisconnected")
        }
      }

    return try {
      bound = context.bindService(Intent(context, XMiloclawRuntimeService::class.java), connection, Context.BIND_AUTO_CREATE)
      if (!bound) {
        lastError = "bind failed"
        Log.w(TAG, "waitForBridge bindService returned false")
        false
      } else {
        val awaited = latch.await(BIND_TIMEOUT_MS, TimeUnit.MILLISECONDS)
        Log.d(TAG, "waitForBridge awaited=$awaited bridged=$bridged")
        bridged
      }
    } catch (error: Exception) {
      lastError = error.message ?: "bridge failed"
      Log.e(TAG, "waitForBridge exception ${lastError ?: ""}", error)
      false
    } finally {
      handshakeOpen.set(false)
      if (bound) {
        try {
          context.unbindService(connection)
        } catch (_: Exception) {
          // ignore
        }
      }
    }
  }
}
