package com.hatsunama.xmilo.dev

object AccessibilityActionDispatcher {
  @Volatile
  private var accessibilityService: XMiloclawAccessibilityService? = null

  fun attach(service: XMiloclawAccessibilityService) {
    accessibilityService = service
  }

  fun detach(service: XMiloclawAccessibilityService) {
    if (accessibilityService === service) {
      accessibilityService = null
    }
  }

  fun isAttached(): Boolean = accessibilityService != null

  fun performGlobalAction(action: Int): Boolean =
    accessibilityService?.performGlobalAction(action) == true
}
