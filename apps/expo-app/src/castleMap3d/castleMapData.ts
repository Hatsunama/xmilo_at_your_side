import { castleMapTheme } from "./castleMapTheme";

const { roomColors, surfaceColors, stairColors, obstacleColors } = castleMapTheme;

export const CASTLE_ROOM_IDS = [
  "main_hall",
  "archive",
  "trophy_room",
  "study",
  "workshop",
  "observatory",
  "potions_room",
  "threshold",
  "bedroom",
] as const;

export type CastleRoomId = (typeof CASTLE_ROOM_IDS)[number];
export type CastleSide = "north" | "south" | "east" | "west";
export type CastleDoorKind = "active" | "future";
export type CastleConnectionKind = "corridor" | "stairs";

export type CastleFootprint = {
  x: number;
  z: number;
  width: number;
  depth: number;
};

export type CastleDoorOpening = {
  id: string;
  side: CastleSide;
  offset: number;
  width: number;
  kind: CastleDoorKind;
  connectionId?: string;
  label: string;
};

export type CastleRoomDefinition = {
  id: CastleRoomId;
  displayName: string;
  level: number;
  footprint: CastleFootprint;
  wallHeight: number;
  wallThickness: number;
  floorColor: string;
  accentColor: string;
  doors: CastleDoorOpening[];
  navigationNote: string;
};

export type CastleConnectionDefinition = {
  id: string;
  from: CastleRoomId;
  to: CastleRoomId;
  kind: CastleConnectionKind;
  active: boolean;
  fromDoorId: string;
  toDoorId: string;
  levelFrom: number;
  levelTo: number;
  notes: string;
};

export type CastleSurfaceDefinition = {
  id: string;
  kind: "floor" | "landing";
  level: number;
  footprint: CastleFootprint;
  connectionId?: string;
  color: string;
};

export type CastleStairDefinition = {
  id: string;
  connectionId: string;
  from: [number, number, number];
  to: [number, number, number];
  width: number;
  steps: number;
  color: string;
};

export type CastleObstacleDefinition = {
  id: string;
  roomId: CastleRoomId;
  kind: "display_case" | "table" | "shelf";
  footprint: CastleFootprint;
  level: number;
  height: number;
  color: string;
  activePathBlocking: false;
};

export type CastleCameraBounds = {
  minTargetX: number;
  maxTargetX: number;
  minTargetZ: number;
  maxTargetZ: number;
  minDistance: number;
  maxDistance: number;
  minPitch: number;
  maxPitch: number;
};

export type CastleCameraDefinition = {
  target: [number, number, number];
  yaw: number;
  pitch: number;
  distance: number;
  bounds: CastleCameraBounds;
};

export type CastleMapDefinition = {
  version: string;
  units: string;
  rooms: CastleRoomDefinition[];
  connections: CastleConnectionDefinition[];
  surfaces: CastleSurfaceDefinition[];
  stairs: CastleStairDefinition[];
  obstacles: CastleObstacleDefinition[];
  initialCamera: CastleCameraDefinition;
  futureNavigationNotes: string[];
};

export const CASTLE_LEVEL_HEIGHT = 2.2;

export const castleRooms: CastleRoomDefinition[] = [
  {
    id: "main_hall",
    displayName: "Main Hall",
    level: 0,
    footprint: { x: 0, z: 0, width: 8, depth: 7 },
    wallHeight: 1.35,
    wallThickness: 0.28,
    floorColor: roomColors.main_hall.floorColor,
    accentColor: roomColors.main_hall.accentColor,
    navigationNote: "Central ground-floor hub with three active ground-floor openings.",
    doors: [
      { id: "main_hall_archive", side: "west", offset: 0, width: 1.6, kind: "active", connectionId: "main_hall_archive", label: "Archive corridor" },
      { id: "main_hall_workshop", side: "east", offset: 0, width: 1.6, kind: "active", connectionId: "main_hall_workshop", label: "Workshop corridor" },
      { id: "main_hall_threshold", side: "north", offset: 0, width: 1.6, kind: "active", connectionId: "main_hall_threshold", label: "Threshold passage" },
    ],
  },
  {
    id: "archive",
    displayName: "Archive",
    level: 0,
    footprint: { x: -10, z: 0, width: 6, depth: 6 },
    wallHeight: 1.25,
    wallThickness: 0.26,
    floorColor: roomColors.archive.floorColor,
    accentColor: roomColors.archive.accentColor,
    navigationNote: "Ground-floor west room with a visible stair tower to the upper trophy room.",
    doors: [
      { id: "archive_main_hall", side: "east", offset: 0, width: 1.6, kind: "active", connectionId: "main_hall_archive", label: "Main Hall corridor" },
      { id: "archive_study", side: "west", offset: 0, width: 1.4, kind: "active", connectionId: "archive_study", label: "Study corridor" },
      { id: "archive_trophy_stair", side: "north", offset: 1.2, width: 1.4, kind: "active", connectionId: "archive_trophy_room", label: "Trophy stair" },
    ],
  },
  {
    id: "study",
    displayName: "Study",
    level: 0,
    footprint: { x: -17, z: 0, width: 5.5, depth: 5 },
    wallHeight: 1.25,
    wallThickness: 0.24,
    floorColor: roomColors.study.floorColor,
    accentColor: roomColors.study.accentColor,
    navigationNote: "Ground-floor west wing room linking Archive to the north Observatory branch.",
    doors: [
      { id: "study_archive", side: "east", offset: 0, width: 1.3, kind: "active", connectionId: "archive_study", label: "Archive corridor" },
      { id: "study_observatory", side: "north", offset: 0, width: 1.3, kind: "active", connectionId: "study_observatory", label: "Observatory corridor" },
    ],
  },
  {
    id: "observatory",
    displayName: "Observatory",
    level: 0,
    footprint: { x: -17, z: -7, width: 5, depth: 5 },
    wallHeight: 1.4,
    wallThickness: 0.24,
    floorColor: roomColors.observatory.floorColor,
    accentColor: roomColors.observatory.accentColor,
    navigationNote: "Ground-floor north branch endpoint above Study, with no hidden exits.",
    doors: [
      { id: "observatory_study", side: "south", offset: 0, width: 1.3, kind: "active", connectionId: "study_observatory", label: "Study corridor" },
    ],
  },
  {
    id: "workshop",
    displayName: "Workshop",
    level: 0,
    footprint: { x: 10, z: 0, width: 6, depth: 6 },
    wallHeight: 1.25,
    wallThickness: 0.26,
    floorColor: roomColors.workshop.floorColor,
    accentColor: roomColors.workshop.accentColor,
    navigationNote: "Ground-floor east room linking Main Hall to the north Potions branch.",
    doors: [
      { id: "workshop_main_hall", side: "west", offset: 0, width: 1.6, kind: "active", connectionId: "main_hall_workshop", label: "Main Hall corridor" },
      { id: "workshop_potions", side: "north", offset: 0, width: 1.3, kind: "active", connectionId: "workshop_potions_room", label: "Potions corridor" },
    ],
  },
  {
    id: "potions_room",
    displayName: "Potions Room",
    level: 0,
    footprint: { x: 10, z: -7, width: 5, depth: 5 },
    wallHeight: 1.25,
    wallThickness: 0.24,
    floorColor: roomColors.potions_room.floorColor,
    accentColor: roomColors.potions_room.accentColor,
    navigationNote: "Ground-floor north branch endpoint above Workshop.",
    doors: [
      { id: "potions_workshop", side: "south", offset: 0, width: 1.3, kind: "active", connectionId: "workshop_potions_room", label: "Workshop corridor" },
    ],
  },
  {
    id: "threshold",
    displayName: "Threshold",
    level: 0,
    footprint: { x: 0, z: -8.5, width: 5, depth: 4 },
    wallHeight: 1.2,
    wallThickness: 0.24,
    floorColor: roomColors.threshold.floorColor,
    accentColor: roomColors.threshold.accentColor,
    navigationNote: "Ground-floor landing room; the north opening starts the visible stair to Bedroom.",
    doors: [
      { id: "threshold_main_hall", side: "south", offset: 0, width: 1.5, kind: "active", connectionId: "main_hall_threshold", label: "Main Hall passage" },
      { id: "threshold_bedroom_stair", side: "north", offset: 0, width: 1.5, kind: "active", connectionId: "threshold_bedroom", label: "Bedroom stair" },
    ],
  },
  {
    id: "bedroom",
    displayName: "Bedroom",
    level: 1,
    footprint: { x: 0, z: -16, width: 6, depth: 5 },
    wallHeight: 1.25,
    wallThickness: 0.24,
    floorColor: roomColors.bedroom.floorColor,
    accentColor: roomColors.bedroom.accentColor,
    navigationNote: "Upper-floor room reached by a visible stair from Threshold and a same-level corridor to Trophy Room.",
    doors: [
      { id: "bedroom_threshold_stair", side: "south", offset: 0, width: 1.5, kind: "active", connectionId: "threshold_bedroom", label: "Threshold stair" },
      { id: "bedroom_trophy", side: "west", offset: 0, width: 1.4, kind: "active", connectionId: "bedroom_trophy_room", label: "Trophy corridor" },
    ],
  },
  {
    id: "trophy_room",
    displayName: "Trophy Room",
    level: 1,
    footprint: { x: -8.5, z: -16, width: 6, depth: 5 },
    wallHeight: 1.25,
    wallThickness: 0.24,
    floorColor: roomColors.trophy_room.floorColor,
    accentColor: roomColors.trophy_room.accentColor,
    navigationNote: "Upper-floor room reached by Bedroom corridor and Archive stair; center display case is not an active-path claim.",
    doors: [
      { id: "trophy_bedroom", side: "east", offset: 0, width: 1.4, kind: "active", connectionId: "bedroom_trophy_room", label: "Bedroom corridor" },
      { id: "trophy_archive_stair", side: "south", offset: 0.8, width: 1.4, kind: "active", connectionId: "archive_trophy_room", label: "Archive stair" },
    ],
  },
];

export const castleConnections: CastleConnectionDefinition[] = [
  { id: "main_hall_archive", from: "main_hall", to: "archive", kind: "corridor", active: true, fromDoorId: "main_hall_archive", toDoorId: "archive_main_hall", levelFrom: 0, levelTo: 0, notes: "Ground-floor west corridor; both rooms have matching open door segments." },
  { id: "main_hall_workshop", from: "main_hall", to: "workshop", kind: "corridor", active: true, fromDoorId: "main_hall_workshop", toDoorId: "workshop_main_hall", levelFrom: 0, levelTo: 0, notes: "Ground-floor east corridor; no wall or prop blocks the opening." },
  { id: "main_hall_threshold", from: "main_hall", to: "threshold", kind: "corridor", active: true, fromDoorId: "main_hall_threshold", toDoorId: "threshold_main_hall", levelFrom: 0, levelTo: 0, notes: "Ground-floor north passage to the stair landing room." },
  { id: "threshold_bedroom", from: "threshold", to: "bedroom", kind: "stairs", active: true, fromDoorId: "threshold_bedroom_stair", toDoorId: "bedroom_threshold_stair", levelFrom: 0, levelTo: 1, notes: "Visible stair run connects the lower Threshold landing to the upper Bedroom landing." },
  { id: "bedroom_trophy_room", from: "bedroom", to: "trophy_room", kind: "corridor", active: true, fromDoorId: "bedroom_trophy", toDoorId: "trophy_bedroom", levelFrom: 1, levelTo: 1, notes: "Same-level upper corridor, not a hidden teleport." },
  { id: "archive_study", from: "archive", to: "study", kind: "corridor", active: true, fromDoorId: "archive_study", toDoorId: "study_archive", levelFrom: 0, levelTo: 0, notes: "Ground-floor west wing corridor." },
  { id: "archive_trophy_room", from: "archive", to: "trophy_room", kind: "stairs", active: true, fromDoorId: "archive_trophy_stair", toDoorId: "trophy_archive_stair", levelFrom: 0, levelTo: 1, notes: "Visible stair tower rises from Archive to upper Trophy Room landing." },
  { id: "study_observatory", from: "study", to: "observatory", kind: "corridor", active: true, fromDoorId: "study_observatory", toDoorId: "observatory_study", levelFrom: 0, levelTo: 0, notes: "Ground-floor north branch corridor." },
  { id: "workshop_potions_room", from: "workshop", to: "potions_room", kind: "corridor", active: true, fromDoorId: "workshop_potions", toDoorId: "potions_workshop", levelFrom: 0, levelTo: 0, notes: "Ground-floor north branch corridor." },
];

export const castleSurfaces: CastleSurfaceDefinition[] = [
  { id: "corridor_main_archive", kind: "floor", level: 0, footprint: { x: -5.5, z: 0, width: 3, depth: 1.7 }, connectionId: "main_hall_archive", color: surfaceColors.corridor_main_archive },
  { id: "corridor_main_workshop", kind: "floor", level: 0, footprint: { x: 5.5, z: 0, width: 3, depth: 1.7 }, connectionId: "main_hall_workshop", color: surfaceColors.corridor_main_workshop },
  { id: "corridor_main_threshold", kind: "floor", level: 0, footprint: { x: 0, z: -5, width: 1.7, depth: 3 }, connectionId: "main_hall_threshold", color: surfaceColors.corridor_main_threshold },
  { id: "corridor_archive_study", kind: "floor", level: 0, footprint: { x: -13.25, z: 0, width: 1.7, depth: 1.45 }, connectionId: "archive_study", color: surfaceColors.corridor_archive_study },
  { id: "corridor_study_observatory", kind: "floor", level: 0, footprint: { x: -17, z: -3.5, width: 1.3, depth: 2 }, connectionId: "study_observatory", color: surfaceColors.corridor_study_observatory },
  { id: "corridor_workshop_potions", kind: "floor", level: 0, footprint: { x: 10, z: -3.75, width: 1.3, depth: 1.5 }, connectionId: "workshop_potions_room", color: surfaceColors.corridor_workshop_potions },
  { id: "landing_threshold_bedroom_lower", kind: "landing", level: 0, footprint: { x: 0, z: -11.45, width: 1.8, depth: 1.5 }, connectionId: "threshold_bedroom", color: surfaceColors.landing_threshold_bedroom_lower },
  { id: "landing_threshold_bedroom_upper", kind: "landing", level: 1, footprint: { x: 0, z: -12.75, width: 1.8, depth: 1.5 }, connectionId: "threshold_bedroom", color: surfaceColors.landing_threshold_bedroom_upper },
  { id: "corridor_bedroom_trophy", kind: "floor", level: 1, footprint: { x: -4.25, z: -16, width: 2.5, depth: 1.45 }, connectionId: "bedroom_trophy_room", color: surfaceColors.corridor_bedroom_trophy },
  { id: "landing_archive_trophy_lower", kind: "landing", level: 0, footprint: { x: -8.8, z: -4, width: 1.8, depth: 1.5 }, connectionId: "archive_trophy_room", color: surfaceColors.landing_archive_trophy_lower },
  { id: "landing_archive_trophy_upper", kind: "landing", level: 1, footprint: { x: -8.8, z: -12.75, width: 1.8, depth: 1.5 }, connectionId: "archive_trophy_room", color: surfaceColors.landing_archive_trophy_upper },
];

export const castleStairs: CastleStairDefinition[] = [
  { id: "stair_threshold_bedroom", connectionId: "threshold_bedroom", from: [0, 0, -11.6], to: [0, CASTLE_LEVEL_HEIGHT, -13.1], width: 1.55, steps: 7, color: stairColors.stair_threshold_bedroom },
  { id: "stair_archive_trophy", connectionId: "archive_trophy_room", from: [-8.8, 0, -4.7], to: [-8.8, CASTLE_LEVEL_HEIGHT, -13], width: 1.55, steps: 9, color: stairColors.stair_archive_trophy },
];

export const castleObstacles: CastleObstacleDefinition[] = [
  { id: "trophy_display_case", roomId: "trophy_room", kind: "display_case", footprint: { x: -8.5, z: -16, width: 1.55, depth: 1.05 }, level: 1, height: 0.75, color: obstacleColors.trophy_display_case, activePathBlocking: false },
  { id: "study_table", roomId: "study", kind: "table", footprint: { x: -17, z: 0.8, width: 1.6, depth: 0.9 }, level: 0, height: 0.55, color: obstacleColors.study_table, activePathBlocking: false },
  { id: "workshop_bench", roomId: "workshop", kind: "table", footprint: { x: 10.8, z: 1, width: 1.8, depth: 0.8 }, level: 0, height: 0.55, color: obstacleColors.workshop_bench, activePathBlocking: false },
];

export const castleCamera: CastleCameraDefinition = {
  target: [-3, 0, -6],
  yaw: -38,
  pitch: 50,
  distance: 25,
  bounds: {
    minTargetX: -26,
    maxTargetX: 18,
    minTargetZ: -19,
    maxTargetZ: 4,
    minDistance: 10,
    maxDistance: 38,
    minPitch: 22,
    maxPitch: 72,
  },
};

export const castleMapDefinition: CastleMapDefinition = {
  version: "phase20a79_castle_theme_v1",
  units: "prototype_world_units",
  rooms: castleRooms,
  connections: castleConnections,
  surfaces: castleSurfaces,
  stairs: castleStairs,
  obstacles: castleObstacles,
  initialCamera: castleCamera,
  futureNavigationNotes: [
    "Room footprints are navmesh-ready blockout surfaces, not final collision truth.",
    "Door openings and stair definitions are explicit world-space topology, not route overlay proof.",
    "Obstacles are visible props only and do not claim final collision coverage.",
  ],
} as const;
