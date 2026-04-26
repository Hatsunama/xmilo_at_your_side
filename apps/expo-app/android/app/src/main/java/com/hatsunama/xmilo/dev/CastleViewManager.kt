package com.hatsunama.xmilo.dev

import android.util.Log
import com.facebook.react.uimanager.SimpleViewManager
import com.facebook.react.uimanager.ThemedReactContext
import com.facebook.react.uimanager.annotations.ReactProp
import com.xmilo.castle.mobile.PatchedEbitenView

class CastleViewManager : SimpleViewManager<PatchedEbitenView>() {
  override fun getName(): String = "EbitenView"

  override fun createViewInstance(reactContext: ThemedReactContext): PatchedEbitenView {
    // Do not start the renderer from an unauthenticated baked-in fallback URL.
    // The JS shell must provide a bearer-ready wsURL prop first; we start in setWsURL().
    return PatchedEbitenView(reactContext)
  }

  @ReactProp(name = "wsURL")
  fun setWsURL(view: PatchedEbitenView, wsURL: String?) {
    if (wsURL.isNullOrBlank()) return
    Log.i("xMiloCastle", "EbitenView wsURL prop received")
    // Critical: start the Ebiten runtime before the SurfaceView fully attaches.
    CastleRuntime.setWsURL(wsURL)
    CastleRuntime.ensureStarted(wsURL, view.context)
  }

  @ReactProp(name = "gesturePacket")
  fun setGesturePacket(view: PatchedEbitenView, gesturePacket: String?) {
    if (gesturePacket.isNullOrBlank()) return
    view.applyGesturePacket(gesturePacket)
  }
}
