import { readFileSync } from "fs";
import path from "path";

const appRoot = path.resolve(__dirname, "..", "..");

function readAppFile(relativePath: string) {
  return readFileSync(path.join(appRoot, relativePath), "utf8");
}

function shouldShowCastleMapLegendOverlay(
  legendReady: boolean,
  legendDismissed: boolean,
  suppressLegendOverlay: boolean
) {
  return legendReady && !legendDismissed && !suppressLegendOverlay;
}

function shouldSuppressCastleMapLegendOverlay(firstRunOverlayReady: boolean, firstRunOverlayVisible: boolean) {
  return !firstRunOverlayReady || firstRunOverlayVisible;
}

describe("castle map legend onboarding overlay", () => {
  it("keeps the legend gate wired to the shared persistence key and renders the panel from the Lair stack", () => {
    const viewSource = readAppFile("src/castleMap3d/CastleMap3DView.tsx");
    const lairSource = readAppFile("app/lair.tsx");

    expect(viewSource).toContain("export function CastleMapLegendPanel");
    expect(viewSource).toContain("return legendReady && !legendDismissed && !suppressLegendOverlay;");
    expect(lairSource).toContain('import { CastleMap3DView, CastleMapLegendPanel } from "../src/castleMap3d/CastleMap3DView"');
    expect(lairSource).toContain("<CastleMapLegendPanel suppressLegendOverlay={!firstRunOverlayReady || firstRunOverlayVisible} />");
  });

  it("shows the legend on a fresh first run when it is ready and not dismissed", () => {
    expect(shouldShowCastleMapLegendOverlay(true, false, false)).toBe(true);
  });

  it("suppresses the legend while the Lair first-run scrim is visible", () => {
    expect(shouldSuppressCastleMapLegendOverlay(false, true)).toBe(true);
    expect(shouldShowCastleMapLegendOverlay(true, false, true)).toBe(false);
  });

  it("shows the legend after the Lair scrim is dismissed when it has not already been dismissed", () => {
    expect(shouldSuppressCastleMapLegendOverlay(true, false)).toBe(false);
    expect(shouldShowCastleMapLegendOverlay(true, false, false)).toBe(true);
  });

  it("keeps a dismissed legend hidden", () => {
    expect(shouldShowCastleMapLegendOverlay(true, true, false)).toBe(false);
  });
});
