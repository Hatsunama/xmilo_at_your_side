import { castleMapDefinition } from "./castleMapData";
import {
  applyCastleMapGestureTransform,
  chooseCastleMapRotationPivot,
  createCastleMapFocalReference,
  footprintContainedByBounds,
  getCastleMapCameraRig,
  getCastleOverviewCamera,
  getCastleSceneBounds,
  panCastleMapCamera,
  panCastleMapCameraByGrabbedPoint,
  panCastleMapCameraByScreenPoints,
  panCastleMapCameraByWorldPoints,
  projectScreenPointToMapPlane,
  requiredDistanceToFrameBounds,
  rotateCastleMapCamera,
  rotateCastleMapCameraAroundPoint,
  tiltCastleMapCamera,
  zoomCastleMapCamera,
  zoomCastleMapCameraAtPoint,
  type CastleMapCameraState,
} from "./castleMapCamera";

describe("castle 3D map camera framing", () => {
  it("includes every room, surface, and obstacle footprint in the scene bounds", () => {
    const bounds = getCastleSceneBounds();
    for (const room of castleMapDefinition.rooms) {
      expect(footprintContainedByBounds(room.footprint, bounds)).toBe(true);
    }
    for (const surface of castleMapDefinition.surfaces) {
      expect(footprintContainedByBounds(surface.footprint, bounds)).toBe(true);
    }
    for (const obstacle of castleMapDefinition.obstacles) {
      expect(footprintContainedByBounds(obstacle.footprint, bounds)).toBe(true);
    }
  });

  it("bases the overview target on the full scene bounds", () => {
    const bounds = getCastleSceneBounds();
    const overview = getCastleOverviewCamera();
    expect(overview.targetX).toBeCloseTo(bounds.centerX, 6);
    expect(overview.targetZ).toBeCloseTo(bounds.centerZ, 6);
  });

  it("sets overview and max zoom-out distance far enough to frame the full map", () => {
    const rig = getCastleMapCameraRig();
    const requiredDistance = requiredDistanceToFrameBounds(rig.sceneBounds);
    expect(rig.overview.distance).toBeGreaterThanOrEqual(requiredDistance);
    expect(rig.maxDistance).toBeGreaterThanOrEqual(rig.overview.distance);
  });

  it("uses the overview camera as the reset/recenter target", () => {
    expect(getCastleOverviewCamera()).toEqual(getCastleMapCameraRig().overview);
  });

  it("keeps the overview pitch centered inside the tightened tilt band", () => {
    const rig = getCastleMapCameraRig();
    expect(rig.minPitch).toBe(34);
    expect(rig.maxPitch).toBe(66);
    expect(rig.overview.pitch).toBe(56);
    expect(rig.overview.pitch).toBeGreaterThanOrEqual(rig.minPitch);
    expect(rig.overview.pitch).toBeLessThanOrEqual(rig.maxPitch);
  });

  it("keeps pan bounds padded beyond the full scene", () => {
    const rig = getCastleMapCameraRig();
    expect(rig.panBounds.minTargetX).toBeLessThan(rig.sceneBounds.minX);
    expect(rig.panBounds.maxTargetX).toBeGreaterThan(rig.sceneBounds.maxX);
    expect(rig.panBounds.minTargetZ).toBeLessThan(rig.sceneBounds.minZ);
    expect(rig.panBounds.maxTargetZ).toBeGreaterThan(rig.sceneBounds.maxZ);
  });

  it("pans the map target from one-finger screen deltas", () => {
    const rig = getCastleMapCameraRig();
    const start: CastleMapCameraState = { ...rig.overview, targetX: 0, targetZ: 0, yaw: 0, distance: 40 };
    const rightDrag = panCastleMapCamera(start, 80, 0, rig);
    const downDrag = panCastleMapCamera(start, 0, 80, rig);

    expect(rightDrag.targetX).toBeLessThan(start.targetX);
    expect(Math.abs(rightDrag.targetZ - start.targetZ)).toBeLessThan(0.001);
    expect(downDrag.targetZ).toBeLessThan(start.targetZ);
    expect(Math.abs(downDrag.targetX - start.targetX)).toBeLessThan(0.001);
  });

  it("pans by keeping the dragged map point under the finger", () => {
    const rig = getCastleMapCameraRig();
    const screenSize = { width: 400, height: 800 };
    const start: CastleMapCameraState = { ...rig.overview, targetX: 0, targetZ: -7, yaw: 0, distance: 36 };
    const startPoint = { x: 200, y: 420 };
    const currentPoint = { x: 260, y: 420 };
    const startMapPoint = projectScreenPointToMapPlane(start, startPoint, screenSize);
    const panned = panCastleMapCameraByScreenPoints(start, startPoint, currentPoint, screenSize, rig);
    const preservedMapPoint = projectScreenPointToMapPlane(panned, currentPoint, screenSize);

    expect(Math.abs(panned.targetX - start.targetX)).toBeGreaterThan(1);
    expect(preservedMapPoint.x).toBeCloseTo(startMapPoint.x, 6);
    expect(preservedMapPoint.z).toBeCloseTo(startMapPoint.z, 6);
  });

  it("pans from a stored grabbed map point instead of a post-activation screen start", () => {
    const rig = getCastleMapCameraRig();
    const screenSize = { width: 393, height: 642 };
    const start: CastleMapCameraState = { ...rig.overview, targetX: 0, targetZ: -7, yaw: -36, pitch: 56, distance: 36 };
    const trueTouchDown = { x: 166, y: 305 };
    const activatedStart = { x: 184, y: 305 };
    const currentPoint = { x: 246, y: 338 };
    const grabbedMapPoint = projectScreenPointToMapPlane(start, trueTouchDown, screenSize);
    const grabbedPan = panCastleMapCameraByGrabbedPoint(start, grabbedMapPoint, currentPoint, screenSize, rig);
    const postActivationPan = panCastleMapCameraByScreenPoints(start, activatedStart, currentPoint, screenSize, rig);
    const preservedMapPoint = projectScreenPointToMapPlane(grabbedPan, currentPoint, screenSize);

    expect(grabbedPan.targetX).not.toBeCloseTo(postActivationPan.targetX, 3);
    expect(preservedMapPoint.x).toBeCloseTo(grabbedMapPoint.x, 6);
    expect(preservedMapPoint.z).toBeCloseTo(grabbedMapPoint.z, 6);
  });

  it("pans left, right, up, and down without inversion", () => {
    const rig = getCastleMapCameraRig();
    const screenSize = { width: 400, height: 800 };
    const start: CastleMapCameraState = { ...rig.overview, targetX: 0, targetZ: -7, yaw: 0, distance: 36 };
    const center = { x: 200, y: 420 };
    const rightDrag = panCastleMapCameraByScreenPoints(start, center, { x: 260, y: 420 }, screenSize, rig);
    const leftDrag = panCastleMapCameraByScreenPoints(start, center, { x: 140, y: 420 }, screenSize, rig);
    const downDrag = panCastleMapCameraByScreenPoints(start, center, { x: 200, y: 480 }, screenSize, rig);
    const upDrag = panCastleMapCameraByScreenPoints(start, center, { x: 200, y: 360 }, screenSize, rig);

    expect((rightDrag.targetX - start.targetX) * (leftDrag.targetX - start.targetX)).toBeLessThan(0);
    expect((downDrag.targetZ - start.targetZ) * (upDrag.targetZ - start.targetZ)).toBeLessThan(0);
  });

  it("uses yaw when converting screen pan to map-plane movement", () => {
    const rig = getCastleMapCameraRig();
    const start: CastleMapCameraState = { ...rig.overview, targetX: 0, targetZ: 0, yaw: 90, distance: 40 };
    const rightDrag = panCastleMapCamera(start, 80, 0, rig);

    expect(Math.abs(rightDrag.targetX - start.targetX)).toBeLessThan(0.001);
    expect(rightDrag.targetZ).toBeGreaterThan(start.targetZ);
  });

  it("uses yaw for projection-based one-finger pan", () => {
    const rig = getCastleMapCameraRig();
    const screenSize = { width: 400, height: 800 };
    const start: CastleMapCameraState = { ...rig.overview, targetX: 0, targetZ: -7, yaw: 90, distance: 36 };
    const panned = panCastleMapCameraByScreenPoints(start, { x: 200, y: 420 }, { x: 260, y: 420 }, screenSize, rig);

    expect(Math.abs(panned.targetX - start.targetX)).toBeLessThan(0.001);
    expect(Math.abs(panned.targetZ - start.targetZ)).toBeGreaterThan(1);
  });

  it("keeps projection-based pan stable at different pitch values", () => {
    const rig = getCastleMapCameraRig();
    const screenSize = { width: 400, height: 800 };
    const lowPitch: CastleMapCameraState = { ...rig.overview, targetX: 0, targetZ: -7, pitch: 34, distance: 36 };
    const highPitch: CastleMapCameraState = { ...rig.overview, targetX: 0, targetZ: -7, pitch: 64, distance: 36 };
    const startPoint = { x: 205, y: 425 };
    const currentPoint = { x: 245, y: 470 };
    for (const camera of [lowPitch, highPitch]) {
      const grabbedMapPoint = projectScreenPointToMapPlane(camera, startPoint, screenSize);
      const panned = panCastleMapCameraByGrabbedPoint(camera, grabbedMapPoint, currentPoint, screenSize, rig);
      const preservedMapPoint = projectScreenPointToMapPlane(panned, currentPoint, screenSize);

      expect(preservedMapPoint.x).toBeCloseTo(grabbedMapPoint.x, 6);
      expect(preservedMapPoint.z).toBeCloseTo(grabbedMapPoint.z, 6);
    }
  });

  it("does not change yaw, pitch, or distance during one-finger pan", () => {
    const rig = getCastleMapCameraRig();
    const screenSize = { width: 400, height: 800 };
    const start: CastleMapCameraState = { ...rig.overview, targetX: 0, targetZ: -7, yaw: 32, pitch: 48, distance: 29 };
    const grabbedMapPoint = projectScreenPointToMapPlane(start, { x: 180, y: 410 }, screenSize);
    const panned = panCastleMapCameraByGrabbedPoint(start, grabbedMapPoint, { x: 270, y: 465 }, screenSize, rig);

    expect(panned.yaw).toBe(start.yaw);
    expect(panned.pitch).toBe(start.pitch);
    expect(panned.distance).toBe(start.distance);
  });

  it("clamps one-finger pan only after calculating the grabbed-point target", () => {
    const rig = getCastleMapCameraRig();
    const screenSize = { width: 400, height: 800 };
    const start: CastleMapCameraState = { ...rig.overview, targetX: rig.panBounds.maxTargetX - 0.1, targetZ: -7, yaw: 0, pitch: 56, distance: 36 };
    const grabbedMapPoint = projectScreenPointToMapPlane(start, { x: 200, y: 420 }, screenSize);
    const panned = panCastleMapCameraByGrabbedPoint(start, grabbedMapPoint, { x: 380, y: 420 }, screenSize, rig);

    expect(panned.targetX).toBe(rig.panBounds.maxTargetX);
    expect(panned.targetZ).toBeGreaterThanOrEqual(rig.panBounds.minTargetZ);
    expect(panned.targetZ).toBeLessThanOrEqual(rig.panBounds.maxTargetZ);
    expect(panned.yaw).toBe(start.yaw);
    expect(panned.pitch).toBe(start.pitch);
    expect(panned.distance).toBe(start.distance);
  });

  it("pans from grabbed raycast world points without changing yaw, pitch, or distance", () => {
    const rig = getCastleMapCameraRig();
    const start: CastleMapCameraState = { ...rig.overview, targetX: 0, targetZ: -7, yaw: 32, pitch: 48, distance: 29 };
    const grabbedPoint = { x: -8, y: 2.41, z: -16 };
    const currentPoint = { x: -10.5, y: 2.41, z: -14.25 };
    const panned = panCastleMapCameraByWorldPoints(start, grabbedPoint, currentPoint, rig);

    expect(panned.targetX).toBeCloseTo(start.targetX + 2.5, 6);
    expect(panned.targetZ).toBeCloseTo(start.targetZ - 1.75, 6);
    expect(panned.yaw).toBe(start.yaw);
    expect(panned.pitch).toBe(start.pitch);
    expect(panned.distance).toBe(start.distance);
  });

  it("clamps raycast world-point pan after calculating the grabbed-hit target", () => {
    const rig = getCastleMapCameraRig();
    const start: CastleMapCameraState = { ...rig.overview, targetX: rig.panBounds.maxTargetX - 0.1, targetZ: -7, yaw: 0, pitch: 56, distance: 36 };
    const panned = panCastleMapCameraByWorldPoints(start, { x: 6, y: 0.21, z: -2 }, { x: 1, y: 0.21, z: -2 }, rig);

    expect(panned.targetX).toBe(rig.panBounds.maxTargetX);
    expect(panned.targetZ).toBe(start.targetZ);
    expect(panned.yaw).toBe(start.yaw);
    expect(panned.pitch).toBe(start.pitch);
    expect(panned.distance).toBe(start.distance);
  });

  it("makes pinch zoom deltas visibly change distance", () => {
    const rig = getCastleMapCameraRig();
    const start = rig.overview;
    const zoomIn = zoomCastleMapCamera(start, 1.25, rig);
    const zoomOut = zoomCastleMapCamera(start, 0.82, rig);

    expect(start.distance - zoomIn.distance).toBeGreaterThan(6);
    expect(zoomOut.distance).toBeGreaterThan(start.distance);
  });

  it("pinch zoom preserves the map point under the two-finger midpoint", () => {
    const rig = getCastleMapCameraRig();
    const screenSize = { width: 400, height: 800 };
    const start = rig.overview;
    const focal = createCastleMapFocalReference(start, { x: 285, y: 430 }, screenSize);
    const zoomed = zoomCastleMapCameraAtPoint(start, 1.35, focal, rig);
    const preservedMapPoint = projectScreenPointToMapPlane(zoomed, focal.screenPoint, screenSize);
    const centerZoom = zoomCastleMapCamera(start, 1.35, rig);

    expect(start.distance - zoomed.distance).toBeGreaterThan(6);
    expect(zoomed.targetX).not.toBeCloseTo(centerZoom.targetX, 6);
    expect(preservedMapPoint.x).toBeCloseTo(focal.mapPoint.x, 6);
    expect(preservedMapPoint.z).toBeCloseTo(focal.mapPoint.z, 6);
  });

  it("rotates heading from two-finger rotation", () => {
    const rig = getCastleMapCameraRig();
    const start = rig.overview;
    const rotated = rotateCastleMapCamera(start, Math.PI / 5, rig);

    expect(rotated.yaw - start.yaw).toBeGreaterThan(30);
  });

  it("chooses the stationary finger as the rotation pivot when one finger anchors", () => {
    const choice = chooseCastleMapRotationPivot(
      [
        { id: 1, x: 120, y: 320 },
        { id: 2, x: 280, y: 320 },
      ],
      [
        { id: 1, x: 122, y: 321 },
        { id: 2, x: 330, y: 390 },
      ],
      { x: 225, y: 355 }
    );

    expect(choice.kind).toBe("stationary_touch");
    expect(choice.point.x).toBe(122);
    expect(choice.point.y).toBe(321);
  });

  it("chooses the midpoint as the rotation pivot when neither finger anchors", () => {
    const choice = chooseCastleMapRotationPivot(
      [
        { id: 1, x: 120, y: 320 },
        { id: 2, x: 280, y: 320 },
      ],
      [
        { id: 1, x: 90, y: 290 },
        { id: 2, x: 330, y: 390 },
      ],
      { x: 225, y: 355 }
    );

    expect(choice.kind).toBe("midpoint");
    expect(choice.point.x).toBe(210);
    expect(choice.point.y).toBe(340);
  });

  it("rotates around a pivot while preserving that pivot focus", () => {
    const rig = getCastleMapCameraRig();
    const screenSize = { width: 400, height: 800 };
    const start = rig.overview;
    const pivot = createCastleMapFocalReference(start, { x: 130, y: 430 }, screenSize);
    const rotated = rotateCastleMapCameraAroundPoint(start, Math.PI / 5, pivot, rig);
    const preservedMapPoint = projectScreenPointToMapPlane(rotated, pivot.screenPoint, screenSize);
    const centerRotate = rotateCastleMapCamera(start, Math.PI / 5, rig);

    expect(rotated.yaw - start.yaw).toBeGreaterThan(30);
    expect(rotated.targetX).not.toBeCloseTo(centerRotate.targetX, 6);
    expect(preservedMapPoint.x).toBeCloseTo(pivot.mapPoint.x, 6);
    expect(preservedMapPoint.z).toBeCloseTo(pivot.mapPoint.z, 6);
  });

  it("tilts pitch visibly from two-finger vertical movement without escaping the tightened band", () => {
    const rig = getCastleMapCameraRig();
    const start = rig.overview;
    const tiltUp = tiltCastleMapCamera(start, -80, rig);
    const tiltDown = tiltCastleMapCamera(start, 80, rig);

    expect(tiltUp.pitch).toBeGreaterThan(start.pitch);
    expect(tiltDown.pitch).toBeLessThan(start.pitch);
    expect(tiltUp.pitch).toBeLessThanOrEqual(rig.maxPitch);
    expect(tiltDown.pitch).toBeGreaterThanOrEqual(rig.minPitch);
  });

  it("clamps two-finger tilt to the tightened pitch band", () => {
    const rig = getCastleMapCameraRig();
    const start = rig.overview;
    const tiltUp = tiltCastleMapCamera(start, -1000, rig);
    const tiltDown = tiltCastleMapCamera(start, 1000, rig);

    expect(tiltUp.pitch).toBe(rig.maxPitch);
    expect(tiltDown.pitch).toBe(rig.minPitch);
  });

  it("clamps pan, zoom, and tilt to the camera rig bounds", () => {
    const rig = getCastleMapCameraRig();
    const start = rig.overview;
    const next = applyCastleMapGestureTransform(start, {
      panDx: 100000,
      panDy: 100000,
      zoomScale: 100,
      tiltDy: 100000,
    }, rig);

    expect(next.targetX).toBeGreaterThanOrEqual(rig.panBounds.minTargetX);
    expect(next.targetX).toBeLessThanOrEqual(rig.panBounds.maxTargetX);
    expect(next.targetZ).toBeGreaterThanOrEqual(rig.panBounds.minTargetZ);
    expect(next.targetZ).toBeLessThanOrEqual(rig.panBounds.maxTargetZ);
    expect(next.distance).toBeGreaterThanOrEqual(rig.minDistance);
    expect(next.distance).toBeLessThanOrEqual(rig.maxDistance);
    expect(next.pitch).toBeGreaterThanOrEqual(rig.minPitch);
    expect(next.pitch).toBeLessThanOrEqual(rig.maxPitch);
  });
});
