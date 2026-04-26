package com.xmilo.castle.mobile;

import android.content.Context;
import android.graphics.PointF;
import android.opengl.GLSurfaceView;
import android.os.SystemClock;
import android.util.AttributeSet;
import android.util.Log;
import android.view.InputDevice;
import android.view.MotionEvent;
import android.view.View;
import android.view.ViewGroup;

import org.json.JSONArray;
import org.json.JSONObject;

import java.lang.ref.WeakReference;
import java.util.ArrayList;
import java.util.Collections;
import java.util.Comparator;
import java.util.HashMap;
import java.util.List;
import java.util.Map;

import com.xmilo.castle.ebitenmobileview.Ebitenmobileview;

public class PatchedEbitenView extends EbitenView {
    private static final String RESUME_PROBE = "XMILO_RESUME_PROBE_R1";
    private static WeakReference<PatchedEbitenView> activeView = new WeakReference<>(null);
    private boolean resumed;
    private int lastW;
    private int lastH;
    private long gestureDownTimeMs;
    private final Map<Integer, PointF> activeGestureTouches = new HashMap<>();
    private OpaqueEbitenSurfaceView opaqueSurface;

    public PatchedEbitenView(Context context) {
        super(context);
        replaceDefaultSurface();
    }

    public PatchedEbitenView(Context context, AttributeSet attrs) {
        super(context, attrs);
        replaceDefaultSurface();
    }

    private void replaceDefaultSurface() {
        View defaultSurface = getChildAt(0);
        if (defaultSurface instanceof GLSurfaceView) {
            Log.i("xMiloCastle", "PatchedEbitenView: keeping default EbitenSurfaceView active");
            return;
        }
        Log.w("xMiloCastle", "PatchedEbitenView: child 0 is not GLSurfaceView: " +
                (defaultSurface == null ? "null" : defaultSurface.getClass().getName()));
    }

    public static boolean hasActiveView() {
        return activeView.get() != null;
    }

    public static void suspendActiveFromActivity() {
        PatchedEbitenView view = activeView.get();
        Log.i("xMiloCastle", RESUME_PROBE + " suspendActiveFromActivity active=" + (view != null));
        if (view != null) {
            view.suspendFromHost("activity");
        }
    }

    public static void resumeActiveFromActivity() {
        PatchedEbitenView view = activeView.get();
        Log.i("xMiloCastle", RESUME_PROBE + " resumeActiveFromActivity active=" + (view != null));
        if (view != null) {
            view.resumeFromHost("activity");
        }
    }

    private static void registerActiveView(PatchedEbitenView view) {
        PatchedEbitenView previous = activeView.get();
        Log.i("xMiloCastle", RESUME_PROBE + " registerActiveView previous=" + (previous != null) + " same=" + (previous == view) + " view=" + view.probeState());
        if (previous != null && previous != view) {
            previous.suspendFromHost("active-view-replaced");
        }
        activeView = new WeakReference<>(view);
    }

    private static void unregisterActiveView(PatchedEbitenView view) {
        Log.i("xMiloCastle", RESUME_PROBE + " unregisterActiveView matches=" + (activeView.get() == view) + " view=" + view.probeState());
        if (activeView.get() == view) {
            activeView.clear();
        }
    }

    private String probeState() {
        View child = getChildAt(0);
        return "resumed=" + resumed +
                " last=" + lastW + "x" + lastH +
                " size=" + getWidth() + "x" + getHeight() +
                " visibility=" + getWindowVisibility() +
                " childExists=" + (child != null) +
                " childClass=" + (child == null ? "null" : child.getClass().getName()) +
                " childIsGLSurfaceView=" + (child instanceof GLSurfaceView);
    }

    public void applyGesturePacket(String gesturePacket) {
        if (gesturePacket == null || gesturePacket.trim().isEmpty()) {
            return;
        }
        try {
            JSONObject payload = new JSONObject(gesturePacket);
            String kind = payload.optString("kind", "move");
            JSONArray touches = payload.optJSONArray("touches");
            JSONArray changedTouches = payload.optJSONArray("changedTouches");
            List<TouchPoint> active = readTouches(touches);
            List<TouchPoint> changed = readTouches(changedTouches);
            MotionEvent event = buildMotionEvent(kind, active, changed);
            if (event == null) {
                return;
            }
            dispatchTouchEvent(event);
            event.recycle();
        } catch (Exception error) {
            Log.w("xMiloCastle", "gesture packet rejected: " + error.getMessage());
        }
    }

    private MotionEvent buildMotionEvent(String kind, List<TouchPoint> active, List<TouchPoint> changed) {
        long eventTime = SystemClock.uptimeMillis();
        if (gestureDownTimeMs == 0L) {
            gestureDownTimeMs = eventTime;
        }

        if ("start".equals(kind) || "move".equals(kind)) {
            activeGestureTouches.clear();
            for (TouchPoint touch : active) {
                activeGestureTouches.put(touch.id, touch.point);
            }
        } else if ("end".equals(kind) || "cancel".equals(kind)) {
            activeGestureTouches.clear();
            for (TouchPoint touch : active) {
                activeGestureTouches.put(touch.id, touch.point);
            }
            for (TouchPoint touch : changed) {
                if (!activeGestureTouches.containsKey(touch.id)) {
                    activeGestureTouches.put(touch.id, touch.point);
                }
            }
        }

        List<TouchPoint> pointers = new ArrayList<>(activeGestureTouches.size() + changed.size());
        for (Map.Entry<Integer, PointF> entry : activeGestureTouches.entrySet()) {
            pointers.add(new TouchPoint(entry.getKey(), entry.getValue().x, entry.getValue().y));
        }
        for (TouchPoint touch : changed) {
            boolean seen = false;
            for (TouchPoint pointer : pointers) {
                if (pointer.id == touch.id) {
                    seen = true;
                    break;
                }
            }
            if (!seen) {
                pointers.add(touch);
            }
        }
        if (pointers.isEmpty()) {
            return null;
        }
        Collections.sort(pointers, Comparator.comparingInt(pointer -> pointer.id));

        int action = MotionEvent.ACTION_MOVE;
        int actionIndex = 0;
        if ("start".equals(kind)) {
            action = pointers.size() == 1 ? MotionEvent.ACTION_DOWN : MotionEvent.ACTION_POINTER_DOWN;
            actionIndex = findPointerIndex(pointers, changed);
        } else if ("end".equals(kind)) {
            if (pointers.size() == 1) {
                action = MotionEvent.ACTION_UP;
            } else {
                action = MotionEvent.ACTION_POINTER_UP;
                actionIndex = findPointerIndex(pointers, changed);
            }
        } else if ("cancel".equals(kind)) {
            action = MotionEvent.ACTION_CANCEL;
        }

        MotionEvent.PointerProperties[] properties = new MotionEvent.PointerProperties[pointers.size()];
        MotionEvent.PointerCoords[] coords = new MotionEvent.PointerCoords[pointers.size()];
        for (int i = 0; i < pointers.size(); i++) {
            TouchPoint touch = pointers.get(i);
            MotionEvent.PointerProperties property = new MotionEvent.PointerProperties();
            property.id = touch.id;
            property.toolType = MotionEvent.TOOL_TYPE_FINGER;
            properties[i] = property;

            MotionEvent.PointerCoords coord = new MotionEvent.PointerCoords();
            coord.x = touch.point.x;
            coord.y = touch.point.y;
            coord.pressure = 1f;
            coord.size = 1f;
            coords[i] = coord;
        }

        int actionMasked = action;
        if (action == MotionEvent.ACTION_POINTER_DOWN || action == MotionEvent.ACTION_POINTER_UP) {
            actionMasked |= actionIndex << MotionEvent.ACTION_POINTER_INDEX_SHIFT;
        }

        if (action == MotionEvent.ACTION_UP || action == MotionEvent.ACTION_CANCEL) {
            activeGestureTouches.clear();
            gestureDownTimeMs = 0L;
        }

        return MotionEvent.obtain(
            gestureDownTimeMs,
            eventTime,
            actionMasked,
            pointers.size(),
            properties,
            coords,
            0,
            0,
            1f,
            1f,
            0,
            0,
            InputDevice.SOURCE_TOUCHSCREEN,
            0
        );
    }

    private List<TouchPoint> readTouches(JSONArray array) {
        List<TouchPoint> touches = new ArrayList<>();
        if (array == null) {
            return touches;
        }
        for (int i = 0; i < array.length(); i++) {
            JSONObject touch = array.optJSONObject(i);
            if (touch == null) {
                continue;
            }
            int id = touch.optInt("identifier", i);
            float x = (float) touch.optDouble("x", touch.optDouble("pageX", 0));
            float y = (float) touch.optDouble("y", touch.optDouble("pageY", 0));
            touches.add(new TouchPoint(id, x, y));
        }
        return touches;
    }

    private int findPointerIndex(List<TouchPoint> pointers, List<TouchPoint> changed) {
        if (changed != null) {
            for (TouchPoint touch : changed) {
                for (int i = 0; i < pointers.size(); i++) {
                    if (pointers.get(i).id == touch.id) {
                        return i;
                    }
                }
            }
        }
        return Math.max(0, pointers.size() - 1);
    }

    private static final class TouchPoint {
        final int id;
        final PointF point;

        TouchPoint(int id, float x, float y) {
            this.id = id;
            this.point = new PointF(x, y);
        }
    }

    @Override
    public void suspendGame() {
        Log.i("xMiloCastle", RESUME_PROBE + " suspendGame before " + probeState());
        try {
            super.suspendGame();
        } catch (Exception e) {
            Log.w("xMiloCastle", "PatchedEbitenView suspendGame failed: " + e.getMessage());
        }
        Log.i("xMiloCastle", RESUME_PROBE + " suspendGame after " + probeState());
    }

    @Override
    public void resumeGame() {
        Log.i("xMiloCastle", RESUME_PROBE + " resumeGame before " + probeState());
        try {
            super.resumeGame();
        } catch (Exception e) {
            Log.w("xMiloCastle", "PatchedEbitenView resumeGame failed: " + e.getMessage());
        }
        Log.i("xMiloCastle", RESUME_PROBE + " resumeGame after " + probeState());
    }

    private void suspendFromHost(String reason) {
        Log.i("xMiloCastle", RESUME_PROBE + " suspendFromHost reason=" + reason + " before " + probeState());
        if (!resumed) {
            return;
        }
        Log.i("xMiloCastle", "PatchedEbitenView suspending from " + reason);
        suspendGame();
        resumed = false;
        Log.i("xMiloCastle", RESUME_PROBE + " suspendFromHost reason=" + reason + " after " + probeState());
    }

    private void resumeFromHost(String reason) {
        Log.i("xMiloCastle", RESUME_PROBE + " resumeFromHost reason=" + reason + " before " + probeState());
        if (resumed) {
            return;
        }
        Log.i("xMiloCastle", "PatchedEbitenView resuming from " + reason);
        resumeGame();
        resumed = true;
        recoverLayoutAndRender();
        Log.i("xMiloCastle", RESUME_PROBE + " resumeFromHost reason=" + reason + " after " + probeState());
    }

    private void recoverLayoutAndRender() {
        Log.i("xMiloCastle", RESUME_PROBE + " recoverLayoutAndRender start " + probeState());
        resendLayoutIfMeasured();
        requestRenderIfPossible();
        post(new Runnable() {
            @Override
            public void run() {
                Log.i("xMiloCastle", RESUME_PROBE + " recoverLayoutAndRender posted " + probeState());
                resendLayoutIfMeasured();
                requestRenderIfPossible();
            }
        });
    }

    private void resendLayoutIfMeasured() {
        if (lastW <= 0 || lastH <= 0) {
            Log.i("xMiloCastle", RESUME_PROBE + " resendLayoutIfMeasured skipped " + probeState());
            return;
        }
        double scale = Ebitenmobileview.deviceScale();
        Log.i("xMiloCastle", RESUME_PROBE + " resendLayoutIfMeasured dp=" + (lastW / scale) + "x" + (lastH / scale) + " " + probeState());
        Ebitenmobileview.layout(lastW / scale, lastH / scale);
    }

    private void requestRenderIfPossible() {
        View child = getChildAt(0);
        Log.i("xMiloCastle", RESUME_PROBE + " requestRenderIfPossible " + probeState());
        if (child instanceof GLSurfaceView) {
            ((GLSurfaceView) child).requestRender();
        }
    }

    @Override
    protected void onLayout(boolean changed, int left, int top, int right, int bottom) {
        int w = right - left;
        int h = bottom - top;
        View child = getChildAt(0);
        if (child != null) {
            child.layout(0, 0, w, h);
        }
        double scale = Ebitenmobileview.deviceScale();
        double widthInDp = w / scale;
        double heightInDp = h / scale;
        Ebitenmobileview.layout(widthInDp, heightInDp);

        if (w != lastW || h != lastH) {
            lastW = w;
            lastH = h;
            Log.i("xMiloCastle", "PatchedEbitenView onLayout px=" + w + "x" + h + " changed=" + changed);
        }
        Log.i("xMiloCastle", RESUME_PROBE + " onLayout changed=" + changed + " bounds=" + w + "x" + h + " " + probeState());

        if (!resumed && w > 0 && h > 0) {
            resumeFromHost("onLayout");
        }
    }

    @Override
    protected void onAttachedToWindow() {
        super.onAttachedToWindow();
        registerActiveView(this);
        Log.i("xMiloCastle", "PatchedEbitenView attached");
        Log.i("xMiloCastle", RESUME_PROBE + " onAttachedToWindow " + probeState());
        if (getWindowVisibility() == VISIBLE) {
            resumeFromHost("attach");
        }
    }

    @Override
    protected void onWindowVisibilityChanged(int visibility) {
        super.onWindowVisibilityChanged(visibility);
        Log.i("xMiloCastle", RESUME_PROBE + " onWindowVisibilityChanged visibility=" + visibility + " " + probeState());
        if (visibility == VISIBLE) {
            resumeFromHost("visibility");
        } else {
            suspendFromHost("visibility");
        }
    }

    @Override
    protected void onDetachedFromWindow() {
        Log.i("xMiloCastle", RESUME_PROBE + " onDetachedFromWindow before " + probeState());
        suspendFromHost("detach");
        unregisterActiveView(this);
        super.onDetachedFromWindow();
        Log.i("xMiloCastle", RESUME_PROBE + " onDetachedFromWindow after " + probeState());
    }
}
