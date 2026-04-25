package com.hatsunama.xmilo.dev

import android.app.NotificationChannel
import android.app.NotificationManager
import android.content.Context
import android.os.Build
import androidx.core.app.NotificationManagerCompat

object XMiloclawNotificationController {
  const val CHANNEL_ID = "xmilo_runtime_host"

  fun areNotificationsGranted(context: Context): Boolean =
    NotificationManagerCompat.from(context).areNotificationsEnabled()

  fun ensureChannel(context: Context) {
    if (Build.VERSION.SDK_INT < Build.VERSION_CODES.O) return

    val manager = context.getSystemService(Context.NOTIFICATION_SERVICE) as NotificationManager
    if (manager.getNotificationChannel(CHANNEL_ID) != null) return

    val channel = NotificationChannel(
      CHANNEL_ID,
      "xMilo runtime host",
      NotificationManager.IMPORTANCE_LOW
    )
    channel.description = "Hidden runtime host used for xMilo setup and verification."
    manager.createNotificationChannel(channel)
  }
}
