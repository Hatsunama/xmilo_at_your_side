import { existsSync, readFileSync } from "fs";
import path from "path";

const appRoot = path.resolve(__dirname, "..", "..");

function readAppFile(relativePath: string) {
  return readFileSync(path.join(appRoot, relativePath), "utf8");
}

describe("lair 3D castle map integration", () => {
  it("renders CastleMap3DView as the Lair map surface", () => {
    const lairSource = readAppFile("app/lair.tsx");
    expect(lairSource).toContain('import { CastleMap3DView, CastleMapLegendPanel } from "../src/castleMap3d/CastleMap3DView"');
    expect(lairSource).toContain('<CastleMap3DView variant="lair" />');
    expect(lairSource).toContain('<CastleMapLegendPanel suppressLegendOverlay={!firstRunOverlayReady || firstRunOverlayVisible} />');
    expect(lairSource.indexOf("<CastleMapLegendPanel")).toBeLessThan(lairSource.indexOf("styles.chatPanel"));
  });

  it("removes the old native Lair surface dependency from the Lair route", () => {
    const lairSource = readAppFile("app/lair.tsx");
    expect(lairSource).not.toContain("usePersistentCastleSurface");
    expect(lairSource).not.toContain("emitGesturePacket");
    expect(lairSource).not.toContain("Preparing castle link");
  });

  it("does not wrap the app in the old persistent native castle surface provider", () => {
    const layoutSource = readAppFile("app/_layout.tsx");
    expect(layoutSource).not.toContain("PersistentCastleSurfaceProvider");
  });

  it("wraps app routes in the gesture-handler root", () => {
    const layoutSource = readAppFile("app/_layout.tsx");
    expect(layoutSource).toContain('import { GestureHandlerRootView } from "react-native-gesture-handler"');
    expect(layoutSource).toContain("<GestureHandlerRootView");
    expect(layoutSource).toContain("styles.gestureRoot");
  });

  it("keeps the shared 3D component on the Lair route and persists the one-time legend overlay dismissal", () => {
    const viewSource = readAppFile("src/castleMap3d/CastleMap3DView.tsx");
    expect(viewSource).toContain('import { getAppSetting, initArchiveDb, setAppSetting } from "../lib/archiveDb"');
    expect(viewSource).toContain("castle_map_legend_dismissed_phase20");
    expect(viewSource).toContain("export function CastleMapLegendPanel");
    expect(viewSource).toContain("suppressLegendOverlay");
    expect(viewSource).toContain("Dismiss castle map legend");
    const lairSource = readAppFile("app/lair.tsx");
    expect(lairSource).toContain('<CastleMapLegendPanel suppressLegendOverlay={!firstRunOverlayReady || firstRunOverlayVisible} />');
  });

  it("loads the first-run overlay state before allowing the legend to appear", () => {
    const lairSource = readAppFile("app/lair.tsx");
    expect(lairSource).toContain("firstRunOverlayReady");
    expect(lairSource).toContain("setFirstRunOverlayReady(true)");
    expect(lairSource).toContain("!firstRunOverlayReady || firstRunOverlayVisible");
  });

  it("does not keep the deleted legacy wrapper files around", () => {
    expect(existsSync(path.join(appRoot, "src/components/PersistentCastleSurface.tsx"))).toBe(false);
    expect(existsSync(path.join(appRoot, "src/components/CastleModule.tsx"))).toBe(false);
  });
});
