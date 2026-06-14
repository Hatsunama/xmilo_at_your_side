import { CASTLE_ROOM_IDS, castleMapDefinition } from "./castleMapData";
import { castleMapTheme } from "./castleMapTheme";

describe("castle map theme", () => {
  it("covers every room and every visible scene palette slot", () => {
    expect(Object.keys(castleMapTheme.roomColors).sort()).toEqual([...CASTLE_ROOM_IDS].sort());
    expect(Object.keys(castleMapTheme.surfaceColors).sort()).toEqual(castleMapDefinition.surfaces.map((surface) => surface.id).sort());
    expect(Object.keys(castleMapTheme.stairColors).sort()).toEqual(castleMapDefinition.stairs.map((stair) => stair.id).sort());
    expect(Object.keys(castleMapTheme.obstacleColors).sort()).toEqual(castleMapDefinition.obstacles.map((obstacle) => obstacle.id).sort());
  });

  it("keeps the castle lighting and material split bounded and decorative", () => {
    expect(castleMapTheme.scene.backgroundColor).toBe("#07101D");
    expect(castleMapTheme.scene.groundColor).not.toBe(castleMapTheme.scene.backgroundColor);
    expect(castleMapTheme.lighting.ambientIntensity).toBeGreaterThan(0);
    expect(castleMapTheme.lighting.ambientIntensity).toBeLessThan(castleMapTheme.lighting.keyIntensity);
    expect(castleMapTheme.lighting.keyIntensity).toBeGreaterThan(castleMapTheme.lighting.fillIntensity);
    expect(castleMapTheme.materials.ground.roughness).toBeGreaterThan(castleMapTheme.materials.accent.roughness);
    expect(castleMapTheme.materials.wall.roughness).toBeGreaterThan(castleMapTheme.materials.accent.roughness);
    expect(castleMapTheme.materials.doorActive.emissive).toBe("#E7C86F");
    expect("emissive" in castleMapTheme.materials.doorFuture).toBe(false);
  });
});
