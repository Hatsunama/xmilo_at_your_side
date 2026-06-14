import type { CastleRoomId } from "./castleMapData";

export type CastleMaterialPreset = {
  roughness: number;
  metalness: number;
  emissive?: string;
  emissiveIntensity?: number;
};

export type CastleRoomPalette = {
  floorColor: string;
  accentColor: string;
};

export const castleMapTheme = {
  scene: {
    backgroundColor: "#07101D",
    groundColor: "#18212C",
  },
  lighting: {
    ambientColor: "#D7E2F1",
    ambientIntensity: 0.42,
    keyColor: "#FFE4B0",
    keyIntensity: 1.45,
    keyPosition: [7.5, 13.5, 6.5] as [number, number, number],
    fillColor: "#8EB4E8",
    fillIntensity: 0.38,
    fillPosition: [-10, 8, -9] as [number, number, number],
    rimColor: "#D19B67",
    rimIntensity: 0.28,
    rimPosition: [-11, 7, 9] as [number, number, number],
  },
  materials: {
    ground: {
      roughness: 0.97,
      metalness: 0.01,
    },
    floor: {
      roughness: 0.9,
      metalness: 0.02,
    },
    accent: {
      roughness: 0.78,
      metalness: 0.04,
    },
    wall: {
      roughness: 0.95,
      metalness: 0.01,
    },
    wallTop: {
      roughness: 0.84,
      metalness: 0.03,
    },
    stair: {
      roughness: 0.9,
      metalness: 0.02,
    },
    obstacle: {
      roughness: 0.84,
      metalness: 0.03,
    },
    doorActive: {
      roughness: 0.62,
      metalness: 0.16,
      emissive: "#E7C86F",
      emissiveIntensity: 0.14,
    },
    doorFuture: {
      roughness: 0.87,
      metalness: 0.04,
    },
  },
  palette: {
    wallColor: "#73675B",
    wallTopColor: "#8B7D6D",
    activeConnectionColor: "#E7C86F",
    futureConnectionColor: "#786B5E",
  },
  legend: {
    groundFloor: "#6A645B",
    upperFloor: "#5D4E68",
    activeLink: "#E7C86F",
    stair: "#8092A7",
  },
  roomColors: {
    main_hall: {
      floorColor: "#6A645B",
      accentColor: "#C3A687",
    },
    archive: {
      floorColor: "#5C4F47",
      accentColor: "#B79268",
    },
    study: {
      floorColor: "#4F5B60",
      accentColor: "#A68B6D",
    },
    observatory: {
      floorColor: "#405368",
      accentColor: "#9AB7D6",
    },
    workshop: {
      floorColor: "#5D5448",
      accentColor: "#C28E5B",
    },
    potions_room: {
      floorColor: "#41564E",
      accentColor: "#7FC3A0",
    },
    threshold: {
      floorColor: "#59626D",
      accentColor: "#AEBED1",
    },
    bedroom: {
      floorColor: "#5D4E68",
      accentColor: "#C7A4FF",
    },
    trophy_room: {
      floorColor: "#685847",
      accentColor: "#DAB15A",
    },
  } satisfies Record<CastleRoomId, CastleRoomPalette>,
  surfaceColors: {
    corridor_main_archive: "#4F5964",
    corridor_main_workshop: "#4D5860",
    corridor_main_threshold: "#57616E",
    corridor_archive_study: "#5B5147",
    corridor_study_observatory: "#54616C",
    corridor_workshop_potions: "#4D6158",
    landing_threshold_bedroom_lower: "#5E6977",
    landing_threshold_bedroom_upper: "#645A72",
    corridor_bedroom_trophy: "#5B4F66",
    landing_archive_trophy_lower: "#555048",
    landing_archive_trophy_upper: "#6D5D4B",
  },
  stairColors: {
    stair_threshold_bedroom: "#8092A7",
    stair_archive_trophy: "#8C785C",
  },
  obstacleColors: {
    trophy_display_case: "#C99A49",
    study_table: "#70513C",
    workshop_bench: "#826145",
  },
} as const;

