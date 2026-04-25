package com.hatsunama.xmilo.dev

import android.accessibilityservice.AccessibilityService
import android.content.ComponentName
import android.content.Context
import android.provider.Settings
import android.view.accessibility.AccessibilityEvent

class XMiloclawAccessibilityService : AccessibilityService() {
  override fun onServiceConnected() {
    AccessibilityActionDispatcher.attach(this)
  }

  override fun onAccessibilityEvent(event: AccessibilityEvent?) {
    // No-op: the host exposes readiness and action dispatch, not background event processing.
  }

  override fun onInterrupt() {
    // No-op.
  }

  override fun onDestroy() {
    AccessibilityActionDispatcher.detach(this)
    super.onDestroy()
  }

  companion object {
    fun isEnabled(context: Context): Boolean {
      val enabled = Settings.Secure.getInt(context.contentResolver, Settings.Secure.ACCESSIBILITY_ENABLED, 0) == 1
      if (!enabled) return false

      val enabledServices =
        Settings.Secure.getString(context.contentResolver, Settings.Secure.ENABLED_ACCESSIBILITY_SERVICES)
          ?: return false
      val componentName = ComponentName(context, XMiloclawAccessibilityService::class.java).flattenToString()
      return enabledServices.split(':').any { it == componentName }
    }
  }
}
