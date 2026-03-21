package com.xmilo.milo;

import com.facebook.react.bridge.ReactApplicationContext;
import com.facebook.react.bridge.ReactContextBaseJavaModule;
import com.facebook.react.bridge.ReactMethod;
import com.xmilo.castle.Mobile;

/**
 * CastleModule exposes Mobile.start() from the gomobile-compiled castle.aar
 * to the React Native JavaScript bridge.
 *
 * PLACEMENT:
 *   android/app/src/main/java/com/xmilo/milo/CastleModule.java
 *
 * REGISTRATION:
 *   Add to your existing ReactPackage implementation:
 *
 *     @Override
 *     public List<NativeModule> createNativeModules(ReactApplicationContext ctx) {
 *         List<NativeModule> modules = new ArrayList<>();
 *         modules.add(new CastleModule(ctx));
 *         // ... existing modules
 *         return modules;
 *     }
 *
 * The EbitenView SurfaceView is registered automatically by the ebitenmobile
 * framework when castle.aar is on the classpath. No separate ViewManager
 * registration is needed — requireNativeComponent('EbitenView') in
 * CastleModule.tsx finds it automatically.
 */
public class CastleModule extends ReactContextBaseJavaModule {

    public CastleModule(ReactApplicationContext reactContext) {
        super(reactContext);
    }

    @Override
    public String getName() {
        return "CastleModule";
    }

    /**
     * Initializes the Ebiten game loop and connects it to PicoClaw.
     * Must be called once before the EbitenView SurfaceView is visible.
     * Safe to call multiple times — subsequent calls are no-ops inside Mobile.start().
     *
     * @param wsURL  PicoClaw WebSocket URL, always "ws://127.0.0.1:42817/ws" on device.
     */
    @ReactMethod
    public void start(String wsURL) {
        Mobile.start(wsURL);
    }
}
