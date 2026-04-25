package com.xmilo.castle.mobile;

import android.content.Context;
import android.graphics.PixelFormat;
import android.opengl.GLES20;
import android.opengl.GLSurfaceView;
import android.os.Handler;
import android.os.Looper;
import android.util.Log;

import java.lang.reflect.InvocationHandler;
import java.lang.reflect.Method;
import java.lang.reflect.Proxy;
import java.nio.ByteBuffer;
import java.nio.ByteOrder;

import javax.microedition.khronos.egl.EGL10;
import javax.microedition.khronos.egl.EGLConfig;
import javax.microedition.khronos.egl.EGLDisplay;
import javax.microedition.khronos.opengles.GL10;

import com.xmilo.castle.ebitenmobileview.Ebitenmobileview;

final class OpaqueEbitenSurfaceView extends GLSurfaceView {
    private static final String TAG = "xMiloCastle";
    private static final String PROBE_TAG = "GoLog";
    private boolean errored;
    private boolean onceSurfaceCreated;
    private volatile int surfacePixelW;
    private volatile int surfacePixelH;
    private static final int[] PROBE_FRAMES = {1, 30, 60};

    OpaqueEbitenSurfaceView(Context context) {
        super(context);
        initialize();
    }

    private void initialize() {
        setEGLContextClientVersion(2);
        setEGLConfigChooser(new OpaqueConfigChooser());
        getHolder().setFormat(PixelFormat.OPAQUE);
        setPreserveEGLContextOnPause(true);
        setRenderer(new AlphaFixRenderer());
        registerAsRenderRequester();
        Log.i(TAG, "OpaqueEbitenSurfaceView: initialized");
    }

    private void registerAsRenderRequester() {
        try {
            Class<?> rrInterface = Class.forName("com.xmilo.castle.ebitenmobileview.RenderRequester");
            Object proxy = Proxy.newProxyInstance(
                rrInterface.getClassLoader(),
                new Class<?>[] { rrInterface },
                new RenderRequesterHandler()
            );
            Method setRR = Ebitenmobileview.class.getMethod("setRenderRequester", rrInterface);
            setRR.invoke(null, proxy);
            Log.i(TAG, "OpaqueEbitenSurfaceView: registered RenderRequester proxy via reflection");
        } catch (ClassNotFoundException e) {
            Log.w(TAG, "RenderRequester not found, trying old Renderer API");
            registerAsRendererFallback();
        } catch (Exception e) {
            Log.e(TAG, "Failed to register RenderRequester: " + e.getMessage(), e);
            registerAsRendererFallback();
        }
    }

    private void registerAsRendererFallback() {
        try {
            Class<?> rendererInterface = Class.forName("com.xmilo.castle.ebitenmobileview.Renderer");
            Object proxy = Proxy.newProxyInstance(
                rendererInterface.getClassLoader(),
                new Class<?>[] { rendererInterface },
                new RenderRequesterHandler()
            );
            Method setR = Ebitenmobileview.class.getMethod("setRenderer", rendererInterface);
            setR.invoke(null, proxy);
            Log.i(TAG, "OpaqueEbitenSurfaceView: registered Renderer proxy via reflection (fallback)");
        } catch (Exception e) {
            Log.e(TAG, "Failed to register Renderer fallback: " + e.getMessage(), e);
        }
    }

    private class RenderRequesterHandler implements InvocationHandler {
        @Override
        public Object invoke(Object proxy, Method method, Object[] args) throws Throwable {
            String name = method.getName();
            switch (name) {
                case "requestRenderIfNeeded":
                    if (getRenderMode() == RENDERMODE_WHEN_DIRTY) {
                        requestRender();
                    }
                    break;
                case "setExplicitRenderingMode":
                    boolean explicit = (Boolean) args[0];
                    if (explicit) {
                        setRenderMode(RENDERMODE_WHEN_DIRTY);
                    } else {
                        setRenderMode(RENDERMODE_CONTINUOUSLY);
                    }
                    break;
                default:
                    if ("toString".equals(name)) return "OpaqueRenderRequesterProxy";
                    if ("hashCode".equals(name)) return System.identityHashCode(proxy);
                    if ("equals".equals(name)) return proxy == args[0];
                    break;
            }
            return null;
        }
    }

    private void onErrorOnGameUpdate(Exception e) {
        if (getParent() instanceof EbitenView) {
            ((EbitenView) getParent()).onErrorOnGameUpdate(e);
        } else {
            Log.e(TAG, "Ebiten update error (parent not EbitenView): " + e.getMessage());
        }
    }

    private void logCenterPixel(String label, long frameIndex) {
        if (frameIndex != 30 && frameIndex != 60) {
            return;
        }
        int surfW = surfacePixelW;
        int surfH = surfacePixelH;
        if (surfW <= 0 || surfH <= 0) {
            Log.i(TAG, "CenterPixelProbe " + label + " frame=" + frameIndex + " SKIP surface dims not captured");
            return;
        }
        int cx = surfW / 2;
        int cy = surfH / 2;
        ByteBuffer pixel = ByteBuffer.allocateDirect(4).order(ByteOrder.nativeOrder());
        GLES20.glReadPixels(cx, cy, 1, 1, GLES20.GL_RGBA, GLES20.GL_UNSIGNED_BYTE, pixel);
        int glErr = GLES20.glGetError();
        pixel.position(0);
        int r = pixel.get() & 0xff;
        int g = pixel.get() & 0xff;
        int b = pixel.get() & 0xff;
        int a = pixel.get() & 0xff;
        long timestamp = System.currentTimeMillis();
        Log.i(PROBE_TAG, "CenterPixelProbe " + label + " frame=" + frameIndex + " ts=" + timestamp + " cx=" + cx + " cy=" + cy + " rgba=" + r + "," + g + "," + b + "," + a + " glErr=0x" + Integer.toHexString(glErr));
    }

    private static class OpaqueConfigChooser implements GLSurfaceView.EGLConfigChooser {
        @Override
        public EGLConfig chooseConfig(EGL10 egl, EGLDisplay display) {
            int[] attribs = {
                EGL10.EGL_RED_SIZE, 8,
                EGL10.EGL_GREEN_SIZE, 8,
                EGL10.EGL_BLUE_SIZE, 8,
                EGL10.EGL_ALPHA_SIZE, 0,
                EGL10.EGL_DEPTH_SIZE, 0,
                EGL10.EGL_STENCIL_SIZE, 0,
                EGL10.EGL_RENDERABLE_TYPE, 4,
                EGL10.EGL_NONE
            };
            int[] numConfig = new int[1];
            egl.eglChooseConfig(display, attribs, null, 0, numConfig);
            if (numConfig[0] <= 0) {
                throw new RuntimeException("OpaqueConfigChooser: no usable EGL config found");
            }
            EGLConfig[] configs = new EGLConfig[numConfig[0]];
            egl.eglChooseConfig(display, attribs, configs, configs.length, numConfig);
            int[] alphaVal = new int[1];
            egl.eglGetConfigAttrib(display, configs[0], EGL10.EGL_ALPHA_SIZE, alphaVal);
            Log.i("xMiloCastle", "OpaqueConfigChooser: selected config alpha=" + alphaVal[0]);
            return configs[0];
        }
    }

    private class AlphaFixRenderer implements GLSurfaceView.Renderer {
        private int probeFrameIndex;
        private long drawCount;

        @Override
        public void onSurfaceCreated(GL10 gl, EGLConfig config) {
            if (!onceSurfaceCreated) {
                onceSurfaceCreated = true;
                Log.i(TAG, "AlphaFixRenderer: surface created (first time)");
                return;
            }
            Log.e(TAG, "AlphaFixRenderer: context lost, killing app");
            Runtime.getRuntime().exit(0);
        }

        @Override
        public void onSurfaceChanged(GL10 gl, int width, int height) {
            surfacePixelW = width;
            surfacePixelH = height;
            Log.i(TAG, "AlphaFixRenderer: surfaceChanged surface=" + width + "x" + height);
        }

        @Override
        public void onDrawFrame(GL10 gl) {
            if (errored) {
                return;
            }
            try {
                drawCount += 1;
                Ebitenmobileview.update();
                Log.i(PROBE_TAG, "XMILO_LIVE_RENDERER_RETURN_PROBE frame=" + drawCount + " surface=" + surfacePixelW + "x" + surfacePixelH);
                logCenterPixel("AFTER_UPDATE", drawCount);
                boolean shouldProbe = probeFrameIndex < PROBE_FRAMES.length
                        && drawCount == PROBE_FRAMES[probeFrameIndex];
                if (shouldProbe) {
                    logViewportCoverageProbes("after_update", drawCount);
                }
                logCenterPixel("AFTER_PRESENT", drawCount);
                if (shouldProbe) {
                    logViewportCoverageProbes("after_present", drawCount);
                    probeFrameIndex++;
                }
            } catch (final Exception e) {
                new Handler(Looper.getMainLooper()).post(new Runnable() {
                    @Override
                    public void run() {
                        onErrorOnGameUpdate(e);
                    }
                });
                errored = true;
            }
        }

        private void logViewportCoverageProbes(String stage, long frameIndex) {
            int[] vp = new int[4];
            GLES20.glGetIntegerv(GLES20.GL_VIEWPORT, vp, 0);
            int vx = vp[0];
            int vy = vp[1];
            int vw = vp[2];
            int vh = vp[3];

            int surfW = surfacePixelW;
            int surfH = surfacePixelH;

            Log.i(PROBE_TAG, "CoverageProbe " + stage
                + " frame=" + frameIndex
                + " viewport=" + vx + "," + vy + " " + vw + "x" + vh
                + " surface=" + surfW + "x" + surfH);

            int insideX = vx + Math.max(1, vw / 4);
            int insideY = vy + Math.max(1, vh / 4);
            logPixel("INSIDE_VIEWPORT", stage, frameIndex, insideX, insideY);

            int outsideX = vx + vw + 200;
            int outsideY = vy + Math.max(1, vh / 2);
            if (surfW > 0 && outsideX < surfW) {
                logPixel("OUTSIDE_VIEWPORT", stage, frameIndex, outsideX, outsideY);
            } else {
                Log.i(PROBE_TAG, "CoverageProbe OUTSIDE_VIEWPORT " + stage
                    + " frame=" + frameIndex
                    + " SKIP outsideX=" + outsideX + " surfW=" + surfW
                    + " (surface not wide enough to probe outside viewport)");
            }

            if (surfW > 0 && surfH > 0) {
                int scx = surfW / 2;
                int scy = surfH / 2;
                logPixel("SURFACE_CENTER", stage, frameIndex, scx, scy);
            } else {
                Log.i(PROBE_TAG, "CoverageProbe SURFACE_CENTER " + stage
                    + " frame=" + frameIndex
                    + " SKIP surface dims not yet captured");
            }
        }

        private void logPixel(String label, String stage, long frameIndex, int x, int y) {
            ByteBuffer pixel = ByteBuffer.allocateDirect(4).order(ByteOrder.nativeOrder());
            GLES20.glReadPixels(x, y, 1, 1, GLES20.GL_RGBA, GLES20.GL_UNSIGNED_BYTE, pixel);
            int glErr = GLES20.glGetError();
            pixel.position(0);
            int r = pixel.get() & 0xff;
            int g = pixel.get() & 0xff;
            int b = pixel.get() & 0xff;
            int a = pixel.get() & 0xff;
            Log.i(PROBE_TAG, "CoverageProbe " + label + " " + stage
                + " frame=" + frameIndex
                + " gl_xy=" + x + "," + y
                + " rgba=" + r + "," + g + "," + b + "," + a
                + " glErr=0x" + Integer.toHexString(glErr));
        }
    }
}
