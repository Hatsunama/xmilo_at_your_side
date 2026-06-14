import {
  castleMapDefinition,
  type CastleCameraBounds,
  type CastleFootprint,
  type CastleMapDefinition,
} from "./castleMapData";

export const CASTLE_MAP_CAMERA_FOV = 48;

export type CastleMapCameraState = {
  targetX: number;
  targetZ: number;
  yaw: number;
  pitch: number;
  distance: number;
};

export type CastleSceneBounds = {
  minX: number;
  maxX: number;
  minZ: number;
  maxZ: number;
  width: number;
  depth: number;
  centerX: number;
  centerZ: number;
};

export type CastleMapCameraRig = {
  sceneBounds: CastleSceneBounds;
  overview: CastleMapCameraState;
  panBounds: Pick<CastleCameraBounds, "minTargetX" | "maxTargetX" | "minTargetZ" | "maxTargetZ">;
  minDistance: number;
  maxDistance: number;
  minPitch: number;
  maxPitch: number;
};

export type CastleMapGestureTransform = {
  panDx?: number;
  panDy?: number;
  zoomScale?: number;
  rotationRadians?: number;
  tiltDy?: number;
  zoomFocal?: CastleMapFocalReference;
  rotationPivot?: CastleMapFocalReference;
};

type MutableBounds = {
  minX: number;
  maxX: number;
  minZ: number;
  maxZ: number;
};

export type CastleMapScreenPoint = {
  x: number;
  y: number;
};

export type CastleMapScreenSize = {
  width: number;
  height: number;
};

export type CastleMapTouchPoint = CastleMapScreenPoint & {
  id: number;
};

export type CastleMapFocalReference = {
  screenPoint: CastleMapScreenPoint;
  mapPoint: CastleMapGroundPoint;
  screenSize: CastleMapScreenSize;
};

export type CastleMapGroundPoint = {
  x: number;
  z: number;
};

export type CastleMapWorldPoint = CastleMapGroundPoint & {
  y: number;
};

export type CastleMapRotationPivotChoice = {
  kind: "stationary_touch" | "midpoint";
  point: CastleMapScreenPoint;
};

export function getCastleSceneBounds(definition: CastleMapDefinition = castleMapDefinition): CastleSceneBounds {
  const bounds: MutableBounds = {
    minX: Number.POSITIVE_INFINITY,
    maxX: Number.NEGATIVE_INFINITY,
    minZ: Number.POSITIVE_INFINITY,
    maxZ: Number.NEGATIVE_INFINITY,
  };

  for (const room of definition.rooms) includeFootprint(bounds, room.footprint);
  for (const surface of definition.surfaces) includeFootprint(bounds, surface.footprint);
  for (const obstacle of definition.obstacles) includeFootprint(bounds, obstacle.footprint);
  for (const stair of definition.stairs) {
    includePoint(bounds, stair.from[0], stair.from[2]);
    includePoint(bounds, stair.to[0], stair.to[2]);
  }

  return finalizeBounds(bounds);
}

export function getCastleMapCameraRig(definition: CastleMapDefinition = castleMapDefinition): CastleMapCameraRig {
  const sceneBounds = getCastleSceneBounds(definition);
  const maxSpan = Math.max(sceneBounds.width, sceneBounds.depth);
  const panPadding = Math.max(4, maxSpan * 0.12);
  const overviewDistance = Math.ceil(requiredDistanceToFrameBounds(sceneBounds));

  return {
    sceneBounds,
    overview: {
      targetX: sceneBounds.centerX,
      targetZ: sceneBounds.centerZ,
      yaw: -36,
      pitch: 56,
      distance: overviewDistance,
    },
    panBounds: {
      minTargetX: sceneBounds.minX - panPadding,
      maxTargetX: sceneBounds.maxX + panPadding,
      minTargetZ: sceneBounds.minZ - panPadding,
      maxTargetZ: sceneBounds.maxZ + panPadding,
    },
    minDistance: Math.max(8, Math.round(maxSpan * 0.18)),
    maxDistance: Math.ceil(overviewDistance * 1.16),
    minPitch: 34,
    maxPitch: 66,
  };
}

export function getCastleOverviewCamera(definition: CastleMapDefinition = castleMapDefinition): CastleMapCameraState {
  return getCastleMapCameraRig(definition).overview;
}

export function clampCastleMapCamera(camera: CastleMapCameraState, rig = getCastleMapCameraRig()): CastleMapCameraState {
  return {
    targetX: clamp(camera.targetX, rig.panBounds.minTargetX, rig.panBounds.maxTargetX),
    targetZ: clamp(camera.targetZ, rig.panBounds.minTargetZ, rig.panBounds.maxTargetZ),
    yaw: camera.yaw,
    pitch: clamp(camera.pitch, rig.minPitch, rig.maxPitch),
    distance: clamp(camera.distance, rig.minDistance, rig.maxDistance),
  };
}

export function recenterCastleMapCamera(rig = getCastleMapCameraRig()): CastleMapCameraState {
  return rig.overview;
}

export function panCastleMapCamera(camera: CastleMapCameraState, panDx: number, panDy: number, rig = getCastleMapCameraRig()): CastleMapCameraState {
  const scale = camera.distance * 0.0026;
  const yaw = degToRad(camera.yaw);
  const rightX = Math.cos(yaw);
  const rightZ = -Math.sin(yaw);
  const forwardX = Math.sin(yaw);
  const forwardZ = Math.cos(yaw);
  return clampCastleMapCamera(
    {
      ...camera,
      targetX: camera.targetX - panDx * scale * rightX - panDy * scale * forwardX,
      targetZ: camera.targetZ - panDx * scale * rightZ - panDy * scale * forwardZ,
    },
    rig
  );
}

export function panCastleMapCameraByScreenPoints(
  camera: CastleMapCameraState,
  startPoint: CastleMapScreenPoint,
  currentPoint: CastleMapScreenPoint,
  screenSize: CastleMapScreenSize,
  rig = getCastleMapCameraRig()
): CastleMapCameraState {
  const startMapPoint = projectScreenPointToMapPlane(camera, startPoint, screenSize);
  const currentMapPoint = projectScreenPointToMapPlane(camera, currentPoint, screenSize);
  return clampCastleMapCamera(
    {
      ...camera,
      targetX: camera.targetX + startMapPoint.x - currentMapPoint.x,
      targetZ: camera.targetZ + startMapPoint.z - currentMapPoint.z,
    },
    rig
  );
}

export function panCastleMapCameraByGrabbedPoint(
  camera: CastleMapCameraState,
  grabbedMapPoint: CastleMapGroundPoint,
  currentPoint: CastleMapScreenPoint,
  screenSize: CastleMapScreenSize,
  rig = getCastleMapCameraRig()
): CastleMapCameraState {
  const currentMapPoint = projectScreenPointToMapPlane(camera, currentPoint, screenSize);
  return clampCastleMapCamera(
    {
      ...camera,
      targetX: camera.targetX + grabbedMapPoint.x - currentMapPoint.x,
      targetZ: camera.targetZ + grabbedMapPoint.z - currentMapPoint.z,
    },
    rig
  );
}

export function panCastleMapCameraByWorldPoints(
  camera: CastleMapCameraState,
  grabbedPoint: CastleMapWorldPoint,
  currentPoint: CastleMapWorldPoint,
  rig = getCastleMapCameraRig()
): CastleMapCameraState {
  return clampCastleMapCamera(
    {
      ...camera,
      targetX: camera.targetX + grabbedPoint.x - currentPoint.x,
      targetZ: camera.targetZ + grabbedPoint.z - currentPoint.z,
    },
    rig
  );
}

export function zoomCastleMapCamera(camera: CastleMapCameraState, zoomScale: number, rig = getCastleMapCameraRig()): CastleMapCameraState {
  const safeScale = Math.max(0.25, Math.min(4, zoomScale || 1));
  return clampCastleMapCamera(
    {
      ...camera,
      distance: camera.distance / Math.pow(safeScale, 1.18),
    },
    rig
  );
}

export function zoomCastleMapCameraAtPoint(
  camera: CastleMapCameraState,
  zoomScale: number,
  focal: CastleMapFocalReference,
  rig = getCastleMapCameraRig()
): CastleMapCameraState {
  return preserveMapPointAtScreenPoint(zoomCastleMapCamera(camera, zoomScale, rig), focal, rig);
}

export function rotateCastleMapCamera(camera: CastleMapCameraState, rotationRadians: number, rig = getCastleMapCameraRig()): CastleMapCameraState {
  return clampCastleMapCamera(
    {
      ...camera,
      yaw: camera.yaw + radToDeg(rotationRadians) * 0.9,
    },
    rig
  );
}

export function rotateCastleMapCameraAroundPoint(
  camera: CastleMapCameraState,
  rotationRadians: number,
  pivot: CastleMapFocalReference,
  rig = getCastleMapCameraRig()
): CastleMapCameraState {
  return preserveMapPointAtScreenPoint(rotateCastleMapCamera(camera, rotationRadians, rig), pivot, rig);
}

export function tiltCastleMapCamera(camera: CastleMapCameraState, tiltDy: number, rig = getCastleMapCameraRig()): CastleMapCameraState {
  return clampCastleMapCamera(
    {
      ...camera,
      pitch: camera.pitch - tiltDy * 0.22,
    },
    rig
  );
}

export function applyCastleMapGestureTransform(camera: CastleMapCameraState, transform: CastleMapGestureTransform, rig = getCastleMapCameraRig()): CastleMapCameraState {
  let next = camera;
  if (transform.panDx || transform.panDy) {
    next = panCastleMapCamera(next, transform.panDx ?? 0, transform.panDy ?? 0, rig);
  }
  if (transform.zoomScale && transform.zoomScale !== 1) {
    next = transform.zoomFocal
      ? zoomCastleMapCameraAtPoint(next, transform.zoomScale, transform.zoomFocal, rig)
      : zoomCastleMapCamera(next, transform.zoomScale, rig);
  }
  if (transform.rotationRadians) {
    next = transform.rotationPivot
      ? rotateCastleMapCameraAroundPoint(next, transform.rotationRadians, transform.rotationPivot, rig)
      : rotateCastleMapCamera(next, transform.rotationRadians, rig);
  }
  if (transform.tiltDy) {
    next = tiltCastleMapCamera(next, transform.tiltDy, rig);
  }
  return clampCastleMapCamera(next, rig);
}

export function projectScreenPointToMapPlane(
  camera: CastleMapCameraState,
  screenPoint: CastleMapScreenPoint,
  screenSize: CastleMapScreenSize,
  fovDegrees = CASTLE_MAP_CAMERA_FOV
): CastleMapGroundPoint {
  if (screenSize.width <= 0 || screenSize.height <= 0) {
    return { x: camera.targetX, z: camera.targetZ };
  }

  const position = cameraPosition(camera);
  const target = { x: camera.targetX, y: 0, z: camera.targetZ };
  const forward = normalize3({
    x: target.x - position.x,
    y: target.y - position.y,
    z: target.z - position.z,
  });
  const right = normalize3(cross3({ x: 0, y: 1, z: 0 }, forward));
  const up = normalize3(cross3(forward, right));
  const aspect = screenSize.width / screenSize.height;
  const tanFov = Math.tan(degToRad(fovDegrees) / 2);
  const ndcX = (screenPoint.x / screenSize.width) * 2 - 1;
  const ndcY = 1 - (screenPoint.y / screenSize.height) * 2;
  const ray = normalize3({
    x: forward.x + right.x * ndcX * tanFov * aspect + up.x * ndcY * tanFov,
    y: forward.y + right.y * ndcX * tanFov * aspect + up.y * ndcY * tanFov,
    z: forward.z + right.z * ndcX * tanFov * aspect + up.z * ndcY * tanFov,
  });

  if (Math.abs(ray.y) < 0.0001) {
    return { x: camera.targetX, z: camera.targetZ };
  }

  const t = -position.y / ray.y;
  if (!Number.isFinite(t) || t < 0) {
    return { x: camera.targetX, z: camera.targetZ };
  }

  return {
    x: position.x + ray.x * t,
    z: position.z + ray.z * t,
  };
}

export function createCastleMapFocalReference(
  camera: CastleMapCameraState,
  screenPoint: CastleMapScreenPoint,
  screenSize: CastleMapScreenSize
): CastleMapFocalReference {
  return {
    screenPoint,
    screenSize,
    mapPoint: projectScreenPointToMapPlane(camera, screenPoint, screenSize),
  };
}

export function preserveMapPointAtScreenPoint(
  camera: CastleMapCameraState,
  focal: CastleMapFocalReference,
  rig = getCastleMapCameraRig()
): CastleMapCameraState {
  const currentMapPoint = projectScreenPointToMapPlane(camera, focal.screenPoint, focal.screenSize);
  return clampCastleMapCamera(
    {
      ...camera,
      targetX: camera.targetX + focal.mapPoint.x - currentMapPoint.x,
      targetZ: camera.targetZ + focal.mapPoint.z - currentMapPoint.z,
    },
    rig
  );
}

export function midpointOfScreenPoints(first: CastleMapScreenPoint, second: CastleMapScreenPoint): CastleMapScreenPoint {
  return {
    x: (first.x + second.x) / 2,
    y: (first.y + second.y) / 2,
  };
}

export function chooseCastleMapRotationPivot(
  startTouches: CastleMapTouchPoint[] | null | undefined,
  currentTouches: CastleMapTouchPoint[] | null | undefined,
  fallbackPoint: CastleMapScreenPoint,
  stationaryThreshold = 8,
  movingThreshold = 18
): CastleMapRotationPivotChoice {
  if (!startTouches || !currentTouches || startTouches.length < 2 || currentTouches.length < 2) {
    return { kind: "midpoint", point: fallbackPoint };
  }

  const matched = currentTouches
    .slice(0, 2)
    .map((current) => {
      const start = startTouches.find((touch) => touch.id === current.id);
      if (!start) return null;
      return {
        current,
        distance: Math.hypot(current.x - start.x, current.y - start.y),
      };
    })
    .filter((item): item is { current: CastleMapTouchPoint; distance: number } => item !== null);

  if (matched.length === 2) {
    const [first, second] = matched;
    if (first.distance <= stationaryThreshold && second.distance >= movingThreshold) {
      return { kind: "stationary_touch", point: first.current };
    }
    if (second.distance <= stationaryThreshold && first.distance >= movingThreshold) {
      return { kind: "stationary_touch", point: second.current };
    }
    return { kind: "midpoint", point: midpointOfScreenPoints(first.current, second.current) };
  }

  return { kind: "midpoint", point: midpointOfScreenPoints(currentTouches[0], currentTouches[1]) };
}

export function requiredDistanceToFrameBounds(bounds: CastleSceneBounds, fovDegrees = CASTLE_MAP_CAMERA_FOV) {
  const halfDiagonal = Math.hypot(bounds.width, bounds.depth) / 2;
  const fovRadians = (fovDegrees * Math.PI) / 180;
  return Math.ceil((halfDiagonal / Math.tan(fovRadians / 2)) * 1.18);
}

export function footprintContainedByBounds(footprint: CastleFootprint, bounds: CastleSceneBounds) {
  const footprintBounds = footprintToBounds(footprint);
  return (
    footprintBounds.minX >= bounds.minX &&
    footprintBounds.maxX <= bounds.maxX &&
    footprintBounds.minZ >= bounds.minZ &&
    footprintBounds.maxZ <= bounds.maxZ
  );
}

function includeFootprint(bounds: MutableBounds, footprint: CastleFootprint) {
  const next = footprintToBounds(footprint);
  bounds.minX = Math.min(bounds.minX, next.minX);
  bounds.maxX = Math.max(bounds.maxX, next.maxX);
  bounds.minZ = Math.min(bounds.minZ, next.minZ);
  bounds.maxZ = Math.max(bounds.maxZ, next.maxZ);
}

function includePoint(bounds: MutableBounds, x: number, z: number) {
  bounds.minX = Math.min(bounds.minX, x);
  bounds.maxX = Math.max(bounds.maxX, x);
  bounds.minZ = Math.min(bounds.minZ, z);
  bounds.maxZ = Math.max(bounds.maxZ, z);
}

function footprintToBounds(footprint: CastleFootprint): MutableBounds {
  return {
    minX: footprint.x - footprint.width / 2,
    maxX: footprint.x + footprint.width / 2,
    minZ: footprint.z - footprint.depth / 2,
    maxZ: footprint.z + footprint.depth / 2,
  };
}

function finalizeBounds(bounds: MutableBounds): CastleSceneBounds {
  const width = bounds.maxX - bounds.minX;
  const depth = bounds.maxZ - bounds.minZ;
  return {
    ...bounds,
    width,
    depth,
    centerX: bounds.minX + width / 2,
    centerZ: bounds.minZ + depth / 2,
  };
}

function clamp(value: number, min: number, max: number) {
  return Math.min(max, Math.max(min, value));
}

function degToRad(value: number) {
  return (value * Math.PI) / 180;
}

function radToDeg(value: number) {
  return value * (180 / Math.PI);
}

function cameraPosition(camera: CastleMapCameraState) {
  const yaw = degToRad(camera.yaw);
  const pitch = degToRad(camera.pitch);
  const horizontalDistance = Math.cos(pitch) * camera.distance;
  return {
    x: camera.targetX + Math.sin(yaw) * horizontalDistance,
    y: Math.sin(pitch) * camera.distance,
    z: camera.targetZ + Math.cos(yaw) * horizontalDistance,
  };
}

function normalize3(vector: { x: number; y: number; z: number }) {
  const length = Math.hypot(vector.x, vector.y, vector.z);
  if (length <= 0.000001) return { x: 0, y: 0, z: 0 };
  return {
    x: vector.x / length,
    y: vector.y / length,
    z: vector.z / length,
  };
}

function cross3(left: { x: number; y: number; z: number }, right: { x: number; y: number; z: number }) {
  return {
    x: left.y * right.z - left.z * right.y,
    y: left.z * right.x - left.x * right.z,
    z: left.x * right.y - left.y * right.x,
  };
}
