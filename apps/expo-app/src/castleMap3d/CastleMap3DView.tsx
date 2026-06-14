import { Canvas, useFrame, useThree } from "@react-three/fiber/native";
import React, { Component, useCallback, useEffect, useMemo, useRef, useState, type ReactNode } from "react";
import { Pressable, StyleSheet, Text, View, useWindowDimensions } from "react-native";
import { Gesture, GestureDetector } from "react-native-gesture-handler";
import * as THREE from "three";

import {
  CASTLE_LEVEL_HEIGHT,
  castleMapDefinition,
  type CastleDoorOpening,
  type CastleFootprint,
  type CastleRoomDefinition,
  type CastleSide,
} from "./castleMapData";
import { castleMapTheme, type CastleMaterialPreset } from "./castleMapTheme";
import { getAppSetting, initArchiveDb, setAppSetting } from "../lib/archiveDb";
import {
  CASTLE_MAP_CAMERA_FOV,
  applyCastleMapGestureTransform,
  chooseCastleMapRotationPivot,
  clampCastleMapCamera,
  createCastleMapFocalReference,
  getCastleMapCameraRig,
  getCastleOverviewCamera,
  panCastleMapCameraByWorldPoints,
  recenterCastleMapCamera,
  type CastleMapCameraState,
  type CastleMapFocalReference,
  type CastleMapScreenPoint,
  type CastleMapGestureTransform,
  type CastleMapTouchPoint,
  type CastleMapWorldPoint,
} from "./castleMapCamera";

type MultiGestureState = {
  camera: CastleMapCameraState;
  activeCount: number;
  startTouches: CastleMapTouchPoint[] | null;
  currentTouches: CastleMapTouchPoint[] | null;
  zoomFocal: CastleMapFocalReference | null;
  rotationPivot: CastleMapFocalReference | null;
  rotationPivotKind: "stationary_touch" | "midpoint" | null;
  transform: CastleMapGestureTransform & Required<Pick<CastleMapGestureTransform, "zoomScale" | "rotationRadians" | "tiltDy">>;
};

type OneFingerPanState = {
  camera: CastleMapCameraState;
  raycastCamera: THREE.Camera;
  grabbedHitPoint: CastleMapWorldPoint;
};

type CastleMapPanRaycastHit = {
  camera: THREE.Camera;
  point: CastleMapWorldPoint;
};

type CastleMapPanRaycaster = {
  capturePanStart(point: CastleMapScreenPoint): CastleMapPanRaycastHit | null;
  raycastPanPoint(point: CastleMapScreenPoint, camera: THREE.Camera): CastleMapWorldPoint | null;
};

type WallSegment = {
  key: string;
  footprint: CastleFootprint;
};

type PanSurfaceDefinition = {
  id: string;
  x: number;
  y: number;
  z: number;
  width: number;
  depth: number;
};

type MapErrorBoundaryProps = {
  children: ReactNode;
};

type MapErrorBoundaryState = {
  error: Error | null;
};

const { scene, lighting, materials, palette, legend } = castleMapTheme;
const LEGEND_DISMISSED_KEY = "castle_map_legend_dismissed_phase20";

type CastleMapLegendOverlayState = {
  legendReady: boolean;
  legendDismissed: boolean;
  suppressLegendOverlay?: boolean;
};

export function shouldShowCastleMapLegendOverlay({
  legendReady,
  legendDismissed,
  suppressLegendOverlay = false,
}: CastleMapLegendOverlayState) {
  return legendReady && !legendDismissed && !suppressLegendOverlay;
}

type CastleMapLegendPanelProps = {
  suppressLegendOverlay?: boolean;
};

export function CastleMapLegendPanel({ suppressLegendOverlay = false }: CastleMapLegendPanelProps) {
  const { width, height } = useWindowDimensions();
  const compact = width < 390 || height < 720;
  const [legendReady, setLegendReady] = useState(false);
  const [legendDismissed, setLegendDismissed] = useState(false);

  useEffect(() => {
    let disposed = false;

    async function loadLegendPreference() {
      try {
        await initArchiveDb();
        const value = await getAppSetting(LEGEND_DISMISSED_KEY);
        if (disposed) return;
        setLegendDismissed(value === "true");
      } catch {
        if (!disposed) {
          setLegendDismissed(false);
        }
      } finally {
        if (!disposed) {
          setLegendReady(true);
        }
      }
    }

    void loadLegendPreference();
    return () => {
      disposed = true;
    };
  }, []);

  const dismissLegend = useCallback(async () => {
    setLegendDismissed(true);
    try {
      await initArchiveDb();
      await setAppSetting(LEGEND_DISMISSED_KEY, "true");
    } catch {
    }
  }, []);

  const showLegendOverlay = shouldShowCastleMapLegendOverlay({
    legendReady,
    legendDismissed,
    suppressLegendOverlay,
  });

  if (!showLegendOverlay) {
    return null;
  }

  return (
    <View style={[styles.legendPanel, compact ? styles.legendPanelCompact : null]} pointerEvents="box-none">
      <Pressable style={styles.legendDismissButton} onPress={dismissLegend} accessibilityRole="button" accessibilityLabel="Dismiss castle map legend">
        <Text style={styles.legendDismissText}>X</Text>
      </Pressable>
      <Text style={styles.controlHint}>
        Drag with one finger to slide. Pinch to zoom. Twist two fingers to rotate. Slide two fingers vertically to tilt. Recenter returns to full-map overview.
      </Text>
      <View style={styles.legendRow}>
        <LegendChip label="Ground floor" color={legend.groundFloor} />
        <LegendChip label="Upper floor" color={legend.upperFloor} />
        <LegendChip label="Active link" color={legend.activeLink} />
        <LegendChip label="Stair" color={legend.stair} />
      </View>
    </View>
  );
}

class MapErrorBoundary extends Component<MapErrorBoundaryProps, MapErrorBoundaryState> {
  state: MapErrorBoundaryState = { error: null };

  static getDerivedStateFromError(error: Error): MapErrorBoundaryState {
    return { error };
  }

  render() {
    if (this.state.error) {
      return (
        <View style={styles.rendererFallback}>
          <Text style={styles.rendererFallbackTitle}>3D map renderer unavailable</Text>
          <Text style={styles.rendererFallbackBody}>
            The castle map route loaded, but this device did not initialize the mobile 3D canvas.
          </Text>
        </View>
      );
    }
    return this.props.children;
  }
}

export function CastleMap3DView({
  variant = "lair",
}: {
  variant?: "lair";
}) {
  const { width, height } = useWindowDimensions();
  const cameraRig = useMemo(() => getCastleMapCameraRig(), []);
  const [cameraState, setCameraState] = useState<CastleMapCameraState>(() => getCastleOverviewCamera());
  const cameraRef = useRef(cameraState);
  const panRaycasterRef = useRef<CastleMapPanRaycaster | null>(null);
  const oneFingerPanStartRef = useRef<OneFingerPanState | null>(null);
  const multiGestureRef = useRef<MultiGestureState | null>(null);
  const compact = width < 390 || height < 720;
  const isLair = variant === "lair";
  const [mapSurfaceSize, setMapSurfaceSize] = useState(() => ({ width, height }));
  const screenSize = useMemo(() => mapSurfaceSize, [mapSurfaceSize]);

  useEffect(() => {
    cameraRef.current = cameraState;
  }, [cameraState]);

  const beginOneFingerPan = useCallback((point: CastleMapScreenPoint) => {
    if (oneFingerPanStartRef.current) return;
    const panHit = panRaycasterRef.current?.capturePanStart(point);
    if (!panHit) return;
    multiGestureRef.current = null;
    oneFingerPanStartRef.current = {
      camera: cameraRef.current,
      raycastCamera: panHit.camera,
      grabbedHitPoint: panHit.point,
    };
  }, []);

  const updateOneFingerPan = useCallback((point: CastleMapScreenPoint) => {
    const start = oneFingerPanStartRef.current;
    if (!start) return;
    const currentHitPoint = panRaycasterRef.current?.raycastPanPoint(point, start.raycastCamera);
    if (!currentHitPoint) return;
    setCameraState(panCastleMapCameraByWorldPoints(start.camera, start.grabbedHitPoint, currentHitPoint, cameraRig));
  }, [cameraRig]);

  const updateMapSurfaceSize = useCallback((nextWidth: number, nextHeight: number) => {
    if (nextWidth <= 0 || nextHeight <= 0) return;
    setMapSurfaceSize((current) => {
      if (Math.abs(current.width - nextWidth) < 0.5 && Math.abs(current.height - nextHeight) < 0.5) return current;
      return { width: nextWidth, height: nextHeight };
    });
  }, []);

  const endOneFingerPan = useCallback(() => {
    oneFingerPanStartRef.current = null;
  }, []);

  const ensureMultiGesture = useCallback((touches?: CastleMapTouchPoint[]) => {
    if (multiGestureRef.current) return multiGestureRef.current;
    oneFingerPanStartRef.current = null;
    multiGestureRef.current = {
      camera: cameraRef.current,
      activeCount: 0,
      startTouches: touches && touches.length >= 2 ? touches : null,
      currentTouches: touches && touches.length >= 2 ? touches : null,
      zoomFocal: null,
      rotationPivot: null,
      rotationPivotKind: null,
      transform: {
        zoomScale: 1,
        rotationRadians: 0,
        tiltDy: 0,
      },
    };
    return multiGestureRef.current;
  }, []);

  const beginMultiGesture = useCallback(() => {
    const gesture = ensureMultiGesture();
    gesture.activeCount += 1;
  }, [ensureMultiGesture]);

  const syncMultiTouches = useCallback((touches: CastleMapTouchPoint[]) => {
    if (touches.length < 2) return;
    const gesture = ensureMultiGesture(touches);
    if (!gesture.startTouches) gesture.startTouches = touches;
    gesture.currentTouches = touches;
  }, [ensureMultiGesture]);

  const updateMultiGesture = useCallback((transform: Partial<MultiGestureState["transform"]> & Pick<CastleMapGestureTransform, "zoomFocal" | "rotationPivot">) => {
    const gesture = ensureMultiGesture();
    gesture.transform = {
      ...gesture.transform,
      ...transform,
    };
    setCameraState(applyCastleMapGestureTransform(gesture.camera, gesture.transform, cameraRig));
  }, [cameraRig, ensureMultiGesture]);

  const updateZoomGesture = useCallback((zoomScale: number, focalPoint: CastleMapScreenPoint) => {
    const gesture = ensureMultiGesture();
    if (!gesture.zoomFocal) {
      gesture.zoomFocal = createCastleMapFocalReference(gesture.camera, focalPoint, screenSize);
    }
    updateMultiGesture({
      zoomScale,
      zoomFocal: gesture.zoomFocal,
    });
  }, [ensureMultiGesture, screenSize, updateMultiGesture]);

  const updateRotationGesture = useCallback((rotationRadians: number, fallbackPoint: CastleMapScreenPoint) => {
    const gesture = ensureMultiGesture();
    const pivotChoice = chooseCastleMapRotationPivot(gesture.startTouches, gesture.currentTouches, fallbackPoint);
    if (!gesture.rotationPivot || gesture.rotationPivotKind !== pivotChoice.kind) {
      gesture.rotationPivot = createCastleMapFocalReference(gesture.camera, pivotChoice.point, screenSize);
      gesture.rotationPivotKind = pivotChoice.kind;
    }
    updateMultiGesture({
      rotationRadians,
      rotationPivot: gesture.rotationPivot,
    });
  }, [ensureMultiGesture, screenSize, updateMultiGesture]);

  const updateTiltGesture = useCallback((tiltDy: number) => {
    updateMultiGesture({ tiltDy });
  }, [updateMultiGesture]);

  const readGestureTouchPoints = useCallback((event: { allTouches?: Array<{ id: number; x: number; y: number }> }) => {
    return (event.allTouches ?? []).map((touch) => ({
      id: touch.id,
      x: touch.x,
      y: touch.y,
    }));
  }, []);

  const endMultiGesture = useCallback(() => {
    const gesture = multiGestureRef.current;
    if (!gesture) return;
    gesture.activeCount -= 1;
    if (gesture.activeCount <= 0) {
      multiGestureRef.current = null;
    }
  }, []);

  const mapGesture = useMemo(() => {
    const oneFingerPan = Gesture.Pan()
      .minPointers(1)
      .maxPointers(1)
      .onTouchesDown((event) => {
        const [touch] = readGestureTouchPoints(event);
        if (touch) beginOneFingerPan(touch);
      })
      .onStart((event) => beginOneFingerPan({ x: event.x, y: event.y }))
      .onUpdate((event) => updateOneFingerPan({ x: event.x, y: event.y }))
      .onFinalize(() => endOneFingerPan())
      .runOnJS(true);

    const pinchZoom = Gesture.Pinch()
      .onTouchesDown((event) => syncMultiTouches(readGestureTouchPoints(event)))
      .onTouchesMove((event) => syncMultiTouches(readGestureTouchPoints(event)))
      .onBegin(() => beginMultiGesture())
      .onUpdate((event) => updateZoomGesture(event.scale, { x: event.focalX, y: event.focalY }))
      .onFinalize(() => endMultiGesture())
      .runOnJS(true);

    const rotateHeading = Gesture.Rotation()
      .onTouchesDown((event) => syncMultiTouches(readGestureTouchPoints(event)))
      .onTouchesMove((event) => syncMultiTouches(readGestureTouchPoints(event)))
      .onBegin(() => beginMultiGesture())
      .onUpdate((event) => updateRotationGesture(event.rotation, { x: event.anchorX, y: event.anchorY }))
      .onFinalize(() => endMultiGesture())
      .runOnJS(true);

    const twoFingerTilt = Gesture.Pan()
      .minPointers(2)
      .onTouchesDown((event) => syncMultiTouches(readGestureTouchPoints(event)))
      .onTouchesMove((event) => syncMultiTouches(readGestureTouchPoints(event)))
      .onBegin(() => beginMultiGesture())
      .onUpdate((event) => updateTiltGesture(event.translationY))
      .onFinalize(() => endMultiGesture())
      .runOnJS(true);

    return Gesture.Simultaneous(oneFingerPan, pinchZoom, rotateHeading, twoFingerTilt);
  }, [
    beginMultiGesture,
    beginOneFingerPan,
    endMultiGesture,
    endOneFingerPan,
    readGestureTouchPoints,
    syncMultiTouches,
    updateOneFingerPan,
    updateRotationGesture,
    updateTiltGesture,
    updateZoomGesture,
  ]);

  function resetCamera() {
    oneFingerPanStartRef.current = null;
    multiGestureRef.current = null;
    setCameraState(recenterCastleMapCamera(cameraRig));
  }

  return (
    <View style={styles.container}>
      <MapErrorBoundary>
        <Canvas style={styles.canvas} camera={{ fov: CASTLE_MAP_CAMERA_FOV, near: 0.1, far: 220 }}>
          <color attach="background" args={[scene.backgroundColor]} />
          <ambientLight color={lighting.ambientColor} intensity={lighting.ambientIntensity} />
          <directionalLight color={lighting.keyColor} position={lighting.keyPosition} intensity={lighting.keyIntensity} />
          <directionalLight color={lighting.fillColor} position={lighting.fillPosition} intensity={lighting.fillIntensity} />
          <directionalLight color={lighting.rimColor} position={lighting.rimPosition} intensity={lighting.rimIntensity} />
          <CameraRig cameraState={cameraState} />
          <CastleWorld panRaycasterRef={panRaycasterRef} />
        </Canvas>
      </MapErrorBoundary>
      <GestureDetector gesture={mapGesture}>
        <View
          style={styles.gestureLayer}
          onLayout={(event) => updateMapSurfaceSize(event.nativeEvent.layout.width, event.nativeEvent.layout.height)}
        />
      </GestureDetector>
      <View style={[styles.topPanel, compact ? styles.topPanelCompact : null, isLair ? styles.topPanelLair : null]} pointerEvents="box-none">
        <View style={[styles.pillRow, isLair ? styles.pillRowLair : null]}>
          <ControlButton label="Recenter" onPress={resetCamera} />
        </View>
      </View>
    </View>
  );
}

function CastleWorld({ panRaycasterRef }: { panRaycasterRef: React.MutableRefObject<CastleMapPanRaycaster | null> }) {
  return (
    <group>
      <mesh position={[-3, -0.08, -7]}>
        <boxGeometry args={[52, 0.08, 34]} />
        <meshStandardMaterial color={scene.groundColor} roughness={materials.ground.roughness} metalness={materials.ground.metalness} />
      </mesh>
      {castleMapDefinition.surfaces.map((surface) => (
        <Block key={surface.id} footprint={surface.footprint} level={surface.level} height={0.12} color={surface.color} material={materials.floor} />
      ))}
      {castleMapDefinition.rooms.map((room) => (
        <RoomBlock key={room.id} room={room} />
      ))}
      {castleMapDefinition.stairs.map((stair) => (
        <StairRun key={stair.id} stair={stair} />
      ))}
      {castleMapDefinition.obstacles.map((obstacle) => (
        <Block key={obstacle.id} footprint={obstacle.footprint} level={obstacle.level} height={obstacle.height} color={obstacle.color} yOffset={0.16} material={materials.obstacle} />
      ))}
      <PanRaycastSurfaces panRaycasterRef={panRaycasterRef} />
    </group>
  );
}

function PanRaycastSurfaces({ panRaycasterRef }: { panRaycasterRef: React.MutableRefObject<CastleMapPanRaycaster | null> }) {
  const { camera, size } = useThree();
  const raycaster = useMemo(() => new THREE.Raycaster(), []);
  const panSurfaceRefs = useRef(new Map<string, THREE.Mesh>());
  const panSurfaces = useMemo(() => createPanSurfaceDefinitions(), []);

  const setPanSurfaceRef = useCallback((id: string, node: THREE.Mesh | null) => {
    if (node) {
      panSurfaceRefs.current.set(id, node);
    } else {
      panSurfaceRefs.current.delete(id);
    }
  }, []);

  const raycastWithCamera = useCallback((point: CastleMapScreenPoint, sourceCamera: THREE.Camera): CastleMapWorldPoint | null => {
    if (size.width <= 0 || size.height <= 0) return null;
    const objects = Array.from(panSurfaceRefs.current.values());
    if (objects.length === 0) return null;
    raycaster.setFromCamera(new THREE.Vector2((point.x / size.width) * 2 - 1, -(point.y / size.height) * 2 + 1), sourceCamera);
    const [hit] = raycaster.intersectObjects(objects, false);
    if (!hit) return null;
    return {
      x: hit.point.x,
      y: hit.point.y,
      z: hit.point.z,
    };
  }, [raycaster, size.height, size.width]);

  useEffect(() => {
    const panRaycaster: CastleMapPanRaycaster = {
      capturePanStart(point) {
        camera.updateMatrixWorld(true);
        const cameraSnapshot = camera.clone();
        if (cameraSnapshot instanceof THREE.PerspectiveCamera) cameraSnapshot.updateProjectionMatrix();
        cameraSnapshot.updateMatrixWorld(true);
        const hitPoint = raycastWithCamera(point, cameraSnapshot);
        return hitPoint ? { camera: cameraSnapshot, point: hitPoint } : null;
      },
      raycastPanPoint(point, sourceCamera) {
        return raycastWithCamera(point, sourceCamera);
      },
    };
    panRaycasterRef.current = panRaycaster;
    return () => {
      if (panRaycasterRef.current === panRaycaster) panRaycasterRef.current = null;
    };
  }, [camera, panRaycasterRef, raycastWithCamera]);

  return (
    <group>
      {panSurfaces.map((surface) => (
        <mesh
          key={surface.id}
          ref={(node) => setPanSurfaceRef(surface.id, node)}
          position={[surface.x, surface.y, surface.z]}
          rotation={[-Math.PI / 2, 0, 0]}
          userData={{ castlePanSurface: true, panSurfaceId: surface.id }}
        >
          <planeGeometry args={[surface.width, surface.depth]} />
          <meshBasicMaterial transparent opacity={0} depthWrite={false} side={THREE.DoubleSide} />
        </mesh>
      ))}
    </group>
  );
}

function RoomBlock({ room }: { room: CastleRoomDefinition }) {
  const wallSegments = useMemo(() => buildWallSegments(room), [room]);
  return (
    <group>
      <Block footprint={room.footprint} level={room.level} height={0.16} color={room.floorColor} material={materials.floor} />
      <Block footprint={{ x: room.footprint.x, z: room.footprint.z, width: room.footprint.width + 0.16, depth: room.footprint.depth + 0.16 }} level={room.level} height={0.03} color={room.accentColor} yOffset={0.17} material={materials.accent} />
      {wallSegments.map((segment) => (
        <Block key={segment.key} footprint={segment.footprint} level={room.level} height={room.wallHeight} color={palette.wallColor} yOffset={0.16} material={materials.wall} />
      ))}
      {wallSegments.map((segment) => (
        <Block key={`${segment.key}_cap`} footprint={segment.footprint} level={room.level} height={0.04} color={palette.wallTopColor} yOffset={room.wallHeight + 0.18} material={materials.wallTop} />
      ))}
      {room.doors.map((door) => (
        <DoorMarker key={door.id} room={room} door={door} />
      ))}
    </group>
  );
}

function Block({
  footprint,
  level,
  height,
  color,
  material = materials.floor,
  yOffset = 0,
}: {
  footprint: CastleFootprint;
  level: number;
  height: number;
  color: string;
  material?: CastleMaterialPreset;
  yOffset?: number;
}) {
  const y = levelToY(level) + yOffset + height / 2;
  return (
    <mesh position={[footprint.x, y, footprint.z]}>
      <boxGeometry args={[footprint.width, height, footprint.depth]} />
      <meshStandardMaterial
        color={color}
        roughness={material.roughness}
        metalness={material.metalness}
        {...(material.emissive ? { emissive: material.emissive, emissiveIntensity: material.emissiveIntensity ?? 0.1 } : {})}
      />
    </mesh>
  );
}

function DoorMarker({ room, door }: { room: CastleRoomDefinition; door: CastleDoorOpening }) {
  const world = doorWorldFootprint(room, door);
  return (
    <Block
      footprint={world}
      level={room.level}
      height={0.06}
      color={door.kind === "active" ? palette.activeConnectionColor : palette.futureConnectionColor}
      yOffset={0.22}
      material={door.kind === "active" ? materials.doorActive : materials.doorFuture}
    />
  );
}

function StairRun({ stair }: { stair: (typeof castleMapDefinition.stairs)[number] }) {
  const from = new THREE.Vector3(...stair.from);
  const to = new THREE.Vector3(...stair.to);
  const delta = to.clone().sub(from);
  const horizontalLength = Math.hypot(delta.x, delta.z);
  const stepRun = horizontalLength / stair.steps;
  const alongX = Math.abs(delta.x) > Math.abs(delta.z);
  const pieces = Array.from({ length: stair.steps }, (_, index) => {
    const t = (index + 0.5) / stair.steps;
    const position = from.clone().lerp(to, t);
    const stepHeight = 0.14;
    return {
      id: `${stair.id}_${index}`,
      position: [position.x, position.y + stepHeight / 2 + 0.08, position.z] as [number, number, number],
      args: (alongX ? [stepRun, stepHeight, stair.width] : [stair.width, stepHeight, stepRun]) as [number, number, number],
    };
  });
  return (
    <group>
      {pieces.map((piece) => (
        <mesh key={piece.id} position={piece.position}>
          <boxGeometry args={piece.args} />
          <meshStandardMaterial color={stair.color} roughness={materials.stair.roughness} metalness={materials.stair.metalness} />
        </mesh>
      ))}
    </group>
  );
}

function createPanSurfaceDefinitions(): PanSurfaceDefinition[] {
  return [
    {
      id: "ground_base",
      x: -3,
      y: -0.035,
      z: -7,
      width: 52,
      depth: 34,
    },
    ...castleMapDefinition.surfaces.map((surface) => ({
      id: `surface_${surface.id}`,
      x: surface.footprint.x,
      y: levelToY(surface.level) + 0.13,
      z: surface.footprint.z,
      width: surface.footprint.width,
      depth: surface.footprint.depth,
    })),
    ...castleMapDefinition.rooms.map((room) => ({
      id: `room_${room.id}`,
      x: room.footprint.x,
      y: levelToY(room.level) + 0.21,
      z: room.footprint.z,
      width: room.footprint.width + 0.16,
      depth: room.footprint.depth + 0.16,
    })),
    ...castleMapDefinition.stairs.flatMap((stair) => createStairPanSurfaces(stair)),
  ];
}

function createStairPanSurfaces(stair: (typeof castleMapDefinition.stairs)[number]): PanSurfaceDefinition[] {
  const from = new THREE.Vector3(...stair.from);
  const to = new THREE.Vector3(...stair.to);
  const delta = to.clone().sub(from);
  const horizontalLength = Math.hypot(delta.x, delta.z);
  const stepRun = horizontalLength / stair.steps;
  const alongX = Math.abs(delta.x) > Math.abs(delta.z);
  return Array.from({ length: stair.steps }, (_, index) => {
    const t = (index + 0.5) / stair.steps;
    const position = from.clone().lerp(to, t);
    return {
      id: `stair_${stair.id}_${index}`,
      x: position.x,
      y: position.y + 0.16,
      z: position.z,
      width: alongX ? stepRun : stair.width,
      depth: alongX ? stair.width : stepRun,
    };
  });
}

function CameraRig({ cameraState }: { cameraState: CastleMapCameraState }) {
  const { camera } = useThree();
  useFrame(() => {
    const position = cameraPosition(cameraState);
    camera.position.set(position.x, position.y, position.z);
    camera.lookAt(cameraState.targetX, levelToY(0), cameraState.targetZ);
    camera.updateProjectionMatrix();
  });
  return null;
}

function ControlButton({ label, active, onPress }: { label: string; active?: boolean; onPress: () => void }) {
  return (
    <Pressable style={[styles.controlButton, active ? styles.controlButtonActive : null]} onPress={onPress} accessibilityRole="button">
      <Text style={[styles.controlButtonText, active ? styles.controlButtonTextActive : null]}>{label}</Text>
    </Pressable>
  );
}

function LegendChip({ label, color }: { label: string; color: string }) {
  return (
    <View style={styles.legendChip}>
      <View style={[styles.legendSwatch, { backgroundColor: color }]} />
      <Text style={styles.legendText}>{label}</Text>
    </View>
  );
}

function levelToY(level: number) {
  return level * CASTLE_LEVEL_HEIGHT;
}

function clamp(value: number, min: number, max: number) {
  return Math.min(max, Math.max(min, value));
}

function degToRad(value: number) {
  return (value * Math.PI) / 180;
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

function buildWallSegments(room: CastleRoomDefinition): WallSegment[] {
  return (["north", "south", "east", "west"] as const).flatMap((side) => buildWallSideSegments(room, side));
}

function buildWallSideSegments(room: CastleRoomDefinition, side: CastleSide): WallSegment[] {
  const doors = room.doors.filter((door) => door.side === side);
  const { width, depth, x, z } = room.footprint;
  const total = side === "north" || side === "south" ? width : depth;
  const axisStart = -total / 2;
  const axisEnd = total / 2;
  const openings = doors
    .map((door) => ({
      start: clamp(door.offset - door.width / 2, axisStart, axisEnd),
      end: clamp(door.offset + door.width / 2, axisStart, axisEnd),
    }))
    .sort((left, right) => left.start - right.start);
  const spans: Array<{ start: number; end: number }> = [];
  let cursor = axisStart;
  for (const opening of openings) {
    if (opening.start > cursor) spans.push({ start: cursor, end: opening.start });
    cursor = Math.max(cursor, opening.end);
  }
  if (cursor < axisEnd) spans.push({ start: cursor, end: axisEnd });
  return spans
    .filter((span) => span.end - span.start > 0.08)
    .map((span, index) => {
      const center = (span.start + span.end) / 2;
      const length = span.end - span.start;
      if (side === "north" || side === "south") {
        return {
          key: `${room.id}_${side}_${index}`,
          footprint: {
            x: x + center,
            z: z + (side === "north" ? -depth / 2 : depth / 2),
            width: length,
            depth: room.wallThickness,
          },
        };
      }
      return {
        key: `${room.id}_${side}_${index}`,
        footprint: {
          x: x + (side === "west" ? -width / 2 : width / 2),
          z: z + center,
          width: room.wallThickness,
          depth: length,
        },
      };
    });
}

function doorWorldFootprint(room: CastleRoomDefinition, door: CastleDoorOpening): CastleFootprint {
  const { x, z, width, depth } = room.footprint;
  const markerDepth = 0.32;
  if (door.side === "north" || door.side === "south") {
    return {
      x: x + door.offset,
      z: z + (door.side === "north" ? -depth / 2 - markerDepth / 2 : depth / 2 + markerDepth / 2),
      width: door.width,
      depth: markerDepth,
    };
  }
  return {
    x: x + (door.side === "west" ? -width / 2 - markerDepth / 2 : width / 2 + markerDepth / 2),
    z: z + door.offset,
    width: markerDepth,
    depth: door.width,
  };
}

const styles = StyleSheet.create({
  container: { flex: 1, backgroundColor: scene.backgroundColor },
  canvas: { ...StyleSheet.absoluteFillObject },
  rendererFallback: {
    ...StyleSheet.absoluteFillObject,
    backgroundColor: scene.backgroundColor,
    alignItems: "center",
    justifyContent: "center",
    padding: 24,
  },
  rendererFallbackTitle: { color: "#F8FAFC", fontSize: 20, fontWeight: "900", textAlign: "center" },
  rendererFallbackBody: { color: "#CBD5E1", marginTop: 8, lineHeight: 20, textAlign: "center" },
  gestureLayer: { ...StyleSheet.absoluteFillObject },
  topPanel: {
    position: "absolute",
    top: 92,
    left: 14,
    right: 14,
    gap: 10,
  },
  topPanelCompact: {
    top: 88,
  },
  topPanelLair: {
    top: 104,
  },
  pillRow: { flexDirection: "row", flexWrap: "wrap", gap: 8 },
  pillRowLair: { justifyContent: "flex-end" },
  controlButton: {
    minHeight: 38,
    borderRadius: 8,
    borderWidth: 1,
    borderColor: "rgba(148, 163, 184, 0.24)",
    backgroundColor: "rgba(15, 23, 42, 0.86)",
    paddingHorizontal: 12,
    alignItems: "center",
    justifyContent: "center",
  },
  controlButtonActive: {
    borderColor: "#38BDF8",
    backgroundColor: "rgba(14, 76, 117, 0.86)",
  },
  controlButtonText: { color: "#DDE7F5", fontSize: 13, fontWeight: "800" },
  controlButtonTextActive: { color: "#F8FAFC" },
  legendPanel: {
    borderRadius: 8,
    borderWidth: 1,
    borderColor: "rgba(148, 163, 184, 0.2)",
    backgroundColor: "rgba(7, 16, 29, 0.76)",
    padding: 10,
    gap: 8,
  },
  legendPanelCompact: {
    padding: 9,
  },
  legendDismissButton: {
    position: "absolute",
    top: 8,
    right: 8,
    width: 28,
    height: 28,
    borderRadius: 14,
    borderWidth: 1,
    borderColor: "rgba(148, 163, 184, 0.28)",
    backgroundColor: "rgba(15, 23, 42, 0.94)",
    alignItems: "center",
    justifyContent: "center",
    zIndex: 2,
  },
  legendDismissText: { color: "#F8FAFC", fontSize: 13, fontWeight: "900", lineHeight: 13 },
  controlHint: { color: "#C7D2E1", fontSize: 12, lineHeight: 17, fontWeight: "700" },
  legendRow: { flexDirection: "row", flexWrap: "wrap", gap: 7 },
  legendChip: {
    flexDirection: "row",
    alignItems: "center",
    gap: 6,
    paddingHorizontal: 8,
    paddingVertical: 5,
    borderRadius: 999,
    backgroundColor: "rgba(15, 23, 42, 0.82)",
  },
  legendSwatch: { width: 10, height: 10, borderRadius: 5, borderWidth: 1, borderColor: "rgba(255,255,255,0.22)" },
  legendText: { color: "#DDE7F5", fontSize: 11, fontWeight: "800" },
});
