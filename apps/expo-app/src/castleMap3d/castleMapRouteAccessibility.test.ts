import { existsSync, readFileSync } from "fs";
import path from "path";

const appRoot = path.resolve(__dirname, "..", "..");

function readAppFile(relativePath: string) {
  return readFileSync(path.join(appRoot, relativePath), "utf8");
}

describe("castle 3D map route accessibility", () => {
  it("keeps the first-viewport Main Hall entry on the commercial path and removes the removed route entry points", () => {
    const indexSource = readAppFile("app/index.tsx");
    expect(indexSource).toContain("Enter the Lair");
    expect(indexSource).not.toContain("Phase 9 proof");
    expect(indexSource).not.toContain("Phase 20 3D Map Proof");
    expect(indexSource).not.toContain("Open 3D Castle Map");
    expect(indexSource).not.toContain('router.push("/castle-map")');
    expect(indexSource).not.toContain('openMenuRoute("/castle-map")');
    expect(indexSource).not.toContain('label="3D Castle Map"');
    expect(indexSource).toContain("App-owned sidecar runtime, local BYOK access, and global task events are active.");
  });

  it("removes the standalone route registration and file", () => {
    const layoutSource = readAppFile("app/_layout.tsx");
    expect(layoutSource).not.toContain('name="castle-map"');
    expect(existsSync(path.join(appRoot, "app/castle-map.tsx"))).toBe(false);
  });

  it("keeps 3D controls, a visible renderer failure fallback, and the first-run legend dismissal overlay", () => {
    const viewSource = readAppFile("src/castleMap3d/CastleMap3DView.tsx");
    expect(viewSource).not.toContain("Drag / pinch / rotate / tilt");
    expect(viewSource).not.toContain("One finger drags the map");
    expect(viewSource).not.toContain("titleBlock");
    expect(viewSource).not.toContain("kicker");
    expect(viewSource).not.toContain("subtitle");
    expect(viewSource).toContain('ControlButton label="Recenter"');
    expect(viewSource).toContain("3D map renderer unavailable");
    expect(viewSource).toContain("castle_map_legend_dismissed_phase20");
    expect(viewSource).toContain("Dismiss castle map legend");
    expect(viewSource).toContain("shouldShowCastleMapLegendOverlay");
    expect(viewSource).toContain("CastleMapLegendPanel");
    expect(viewSource).toContain("Ground floor");
    expect(viewSource).toContain("Upper floor");
    expect(viewSource).toContain("Active link");
    expect(viewSource).toContain("Stair");
    expect(viewSource).not.toContain("Orbit mode");
    expect(viewSource).not.toContain("Pan mode");
    expect(viewSource).not.toContain("pinch or +/- to zoom");
    expect(viewSource).not.toContain("Zoom in castle map");
    expect(viewSource).not.toContain("Zoom out castle map");
  });

  it("uses gesture-handler controls instead of the old map PanResponder path", () => {
    const viewSource = readAppFile("src/castleMap3d/CastleMap3DView.tsx");
    expect(viewSource).toContain('import { Gesture, GestureDetector } from "react-native-gesture-handler"');
    expect(viewSource).toContain("Gesture.Pan()");
    expect(viewSource).toContain("Gesture.Pinch()");
    expect(viewSource).toContain("Gesture.Rotation()");
    expect(viewSource).toContain("Gesture.Simultaneous");
    expect(viewSource).toContain("focalX");
    expect(viewSource).toContain("focalY");
    expect(viewSource).toContain("anchorX");
    expect(viewSource).toContain("anchorY");
    expect(viewSource).toContain("onTouchesDown");
    expect(viewSource).toContain("onTouchesMove");
    expect(viewSource).toContain("readGestureTouchPoints");
    expect(viewSource).toContain("updateMapSurfaceSize");
    expect(viewSource).toContain("runOnJS(true)");
    expect(viewSource).not.toContain("PanResponder");
    expect(viewSource).not.toContain("panHandlers");
    expect(viewSource).not.toContain("readTouches");
  });

  it("uses R3F raycast pan surfaces as the one-finger pan authority", () => {
    const viewSource = readAppFile("src/castleMap3d/CastleMap3DView.tsx");
    const beginPanBlock = viewSource.slice(viewSource.indexOf("const beginOneFingerPan"), viewSource.indexOf("const updateOneFingerPan"));
    const updatePanBlock = viewSource.slice(viewSource.indexOf("const updateOneFingerPan"), viewSource.indexOf("const updateMapSurfaceSize"));

    expect(viewSource).toContain("PanRaycastSurfaces");
    expect(viewSource).toContain("panRaycasterRef");
    expect(viewSource).toContain("capturePanStart");
    expect(viewSource).toContain("raycastPanPoint");
    expect(viewSource).toContain("raycaster.setFromCamera");
    expect(viewSource).toContain("intersectObjects");
    expect(viewSource).toContain("panCastleMapCameraByWorldPoints");
    expect(viewSource).toContain("createPanSurfaceDefinitions");
    expect(viewSource).toContain("surface_");
    expect(viewSource).toContain("room_");
    expect(viewSource).toContain("stair_");
    expect(beginPanBlock).not.toContain("projectScreenPointToMapPlane");
    expect(updatePanBlock).not.toContain("projectScreenPointToMapPlane");
    expect(viewSource).not.toContain("panCastleMapCameraByGrabbedPoint");
  });

  it("keeps the commercial variant branch from forking gesture and camera behavior", () => {
    const viewSource = readAppFile("src/castleMap3d/CastleMap3DView.tsx");
    const gestureBlock = viewSource.slice(viewSource.indexOf("const mapGesture = useMemo"), viewSource.indexOf("function resetCamera"));
    expect(viewSource).toContain('export function CastleMap3DView({');
    expect(viewSource).not.toContain('variant = "proof"');
    expect(viewSource).toContain('const isLair = variant === "lair";');
    expect(gestureBlock).not.toContain("variant");
    expect(gestureBlock).not.toContain("isLair");
  });

  it("deletes the legacy native surface wrapper files", () => {
    expect(existsSync(path.join(appRoot, "src/components/PersistentCastleSurface.tsx"))).toBe(false);
    expect(existsSync(path.join(appRoot, "src/components/CastleModule.tsx"))).toBe(false);
  });
});
