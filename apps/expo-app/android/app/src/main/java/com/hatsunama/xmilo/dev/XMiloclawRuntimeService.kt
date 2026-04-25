package com.hatsunama.xmilo.dev

import android.app.Notification
import android.app.PendingIntent
import android.app.Service
import android.content.pm.ServiceInfo
import android.content.Intent
import android.os.Binder
import android.os.IBinder
import androidx.core.app.NotificationCompat
import androidx.core.app.ServiceCompat

class XMiloclawRuntimeService : Service() {
  class RuntimeBinder : Binder() {
    fun ping(): Boolean = true
  }

  companion object {
    private const val NOTIFICATION_ID = 0x7842

    @Volatile
    var isRunning: Boolean = false

    @Volatile
    var bridgeConnected: Boolean = false
  }

  private val binder = RuntimeBinder()

  override fun onCreate() {
    super.onCreate()
    isRunning = true
    XMiloclawWakeController.acquire(applicationContext)
    XMiloclawNotificationController.ensureChannel(applicationContext)
    ServiceCompat.startForeground(
      this,
      NOTIFICATION_ID,
      buildNotification(),
      ServiceInfo.FOREGROUND_SERVICE_TYPE_SPECIAL_USE
    )
    Thread { XMiloclawSidecarController.ensureRunning(applicationContext) }.start()
  }

  override fun onStartCommand(intent: Intent?, flags: Int, startId: Int): Int {
    isRunning = true
    return START_STICKY
  }

  override fun onBind(intent: Intent): IBinder {
    return binder
  }

  override fun onDestroy() {
    bridgeConnected = false
    isRunning = false
    XMiloclawWakeController.release()
    XMiloclawSidecarController.stop()
    super.onDestroy()
  }

  private fun buildNotification(): Notification {
    val launchIntent =
      packageManager.getLaunchIntentForPackage(packageName)
        ?: Intent(applicationContext, MainActivity::class.java)
    val pendingIntent =
      PendingIntent.getActivity(
        applicationContext,
        0,
        launchIntent,
        PendingIntent.FLAG_IMMUTABLE or PendingIntent.FLAG_UPDATE_CURRENT
      )

    return NotificationCompat.Builder(applicationContext, XMiloclawNotificationController.CHANNEL_ID)
      .setSmallIcon(R.mipmap.ic_launcher)
      .setContentTitle("xMilo runtime host")
      .setContentText("Hidden runtime host is active")
      .setOngoing(true)
      .setSilent(true)
      .setContentIntent(pendingIntent)
      .build()
  }
}
