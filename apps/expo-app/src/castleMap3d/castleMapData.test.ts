import { getCastleSceneBounds } from "./castleMapCamera";
import { CASTLE_ROOM_IDS, castleMapDefinition, type CastleFootprint } from "./castleMapData";

const lockedAdjacency = [
  ["main_hall", "archive"],
  ["main_hall", "workshop"],
  ["main_hall", "threshold"],
  ["threshold", "bedroom"],
  ["bedroom", "trophy_room"],
  ["archive", "study"],
  ["archive", "trophy_room"],
  ["study", "observatory"],
  ["workshop", "potions_room"],
] as const;

function getFootprintBounds(footprint: CastleFootprint) {
  return {
    minX: footprint.x - footprint.width / 2,
    maxX: footprint.x + footprint.width / 2,
    minZ: footprint.z - footprint.depth / 2,
    maxZ: footprint.z + footprint.depth / 2,
  };
}

function getRoom(id: string) {
  const room = castleMapDefinition.rooms.find((item) => item.id === id);
  expect(room).toBeDefined();
  return room!;
}

function footprintsOverlap(left: CastleFootprint, right: CastleFootprint) {
  const a = getFootprintBounds(left);
  const b = getFootprintBounds(right);
  return a.minX < b.maxX && a.maxX > b.minX && a.minZ < b.maxZ && a.maxZ > b.minZ;
}

describe("castle 3D map blockout data", () => {
  it("contains exactly the locked room set", () => {
    expect(castleMapDefinition.rooms.map((room) => room.id).sort()).toEqual([...CASTLE_ROOM_IDS].sort());
  });

  it("preserves the locked active adjacency", () => {
    const actual = castleMapDefinition.connections
      .filter((connection) => connection.active)
      .map((connection) => [connection.from, connection.to].sort().join(":"))
      .sort();
    const expected = lockedAdjacency.map(([from, to]) => [from, to].sort().join(":")).sort();
    expect(actual).toEqual(expected);
  });

  it("requires every active connection to have matching active door openings on both rooms", () => {
    for (const connection of castleMapDefinition.connections.filter((item) => item.active)) {
      const fromRoom = castleMapDefinition.rooms.find((room) => room.id === connection.from);
      const toRoom = castleMapDefinition.rooms.find((room) => room.id === connection.to);
      expect(fromRoom).toBeDefined();
      expect(toRoom).toBeDefined();
      const fromDoor = fromRoom?.doors.find((door) => door.id === connection.fromDoorId);
      const toDoor = toRoom?.doors.find((door) => door.id === connection.toDoorId);
      expect(fromDoor).toMatchObject({ kind: "active", connectionId: connection.id });
      expect(toDoor).toMatchObject({ kind: "active", connectionId: connection.id });
    }
  });

  it("keeps same-level links as corridors and cross-level links as visible stair runs", () => {
    for (const connection of castleMapDefinition.connections.filter((item) => item.active)) {
      const crossLevel = connection.levelFrom !== connection.levelTo;
      if (crossLevel) {
        expect(connection.kind).toBe("stairs");
        expect(castleMapDefinition.stairs.some((stair) => stair.connectionId === connection.id && stair.steps >= 2)).toBe(true);
      } else {
        expect(connection.kind).toBe("corridor");
        expect(castleMapDefinition.surfaces.some((surface) => surface.connectionId === connection.id)).toBe(true);
      }
    }
  });

  it("does not mark props as active-path blockers or final collision truth", () => {
    expect(castleMapDefinition.obstacles.length).toBeGreaterThan(0);
    for (const obstacle of castleMapDefinition.obstacles) {
      expect(obstacle.activePathBlocking).toBe(false);
    }
    expect(castleMapDefinition.futureNavigationNotes.join(" ")).toContain("not final collision truth");
  });

  it("moves observatory and potions_room above their branch rooms without changing level", () => {
    const study = getRoom("study");
    const observatory = getRoom("observatory");
    const workshop = getRoom("workshop");
    const potionsRoom = getRoom("potions_room");

    expect(observatory.level).toBe(0);
    expect(potionsRoom.level).toBe(0);
    expect(getFootprintBounds(observatory.footprint).maxZ).toBeLessThan(getFootprintBounds(study.footprint).minZ);
    expect(getFootprintBounds(potionsRoom.footprint).maxZ).toBeLessThan(getFootprintBounds(workshop.footprint).minZ);

    const studyObservatorySurface = castleMapDefinition.surfaces.find((surface) => surface.connectionId === "study_observatory");
    const workshopPotionsSurface = castleMapDefinition.surfaces.find((surface) => surface.connectionId === "workshop_potions_room");
    expect(studyObservatorySurface).toBeDefined();
    expect(workshopPotionsSurface).toBeDefined();
    expect(studyObservatorySurface!.footprint.depth).toBeGreaterThan(studyObservatorySurface!.footprint.width);
    expect(workshopPotionsSurface!.footprint.depth).toBeGreaterThan(workshopPotionsSurface!.footprint.width);
  });

  it("keeps same-level room footprints non-overlapping", () => {
    const sameLevelRooms = castleMapDefinition.rooms.filter((room) => room.level === 0);
    for (let index = 0; index < sameLevelRooms.length; index += 1) {
      for (let otherIndex = index + 1; otherIndex < sameLevelRooms.length; otherIndex += 1) {
        expect(footprintsOverlap(sameLevelRooms[index].footprint, sameLevelRooms[otherIndex].footprint)).toBe(false);
      }
    }
  });

  it("keeps scene bounds enclosing every room footprint", () => {
    const sceneBounds = getCastleSceneBounds(castleMapDefinition);
    for (const room of castleMapDefinition.rooms) {
      const bounds = getFootprintBounds(room.footprint);
      expect(bounds.minX).toBeGreaterThanOrEqual(sceneBounds.minX);
      expect(bounds.maxX).toBeLessThanOrEqual(sceneBounds.maxX);
      expect(bounds.minZ).toBeGreaterThanOrEqual(sceneBounds.minZ);
      expect(bounds.maxZ).toBeLessThanOrEqual(sceneBounds.maxZ);
    }
  });
});
