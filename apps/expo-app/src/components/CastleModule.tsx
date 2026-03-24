/**
 * CastleModule.tsx
 *
 * React Native wrapper for the Ebiten castle-go renderer.
 *
 * CURRENT STATE:
 * - Starts the native castle renderer when the Android classpath exposes
 *   the generated castle.aar package.
 * - Falls back honestly to the Expo placeholder when the native renderer
 *   is unavailable or intentionally bypassed.
 *
 * See castle-go/BUILD.md for the full build sequence.
 */

import React, { useCallback, useRef } from "react";
import {
  NativeModules,
  type LayoutChangeEvent,
  Platform,
  requireNativeComponent,
  StyleSheet,
  Text,
  View,
  type DimensionValue,
  type ViewProps,
  type ViewStyle,
} from "react-native";

import { getCastleRendererStatus } from "../lib/castleRenderer";
import type { NightlyRitualState } from "../lib/maintenanceRitual";

type EbitenViewProps = ViewProps & {
  style?: ViewStyle;
};

const EbitenView = requireNativeComponent<EbitenViewProps>("EbitenView");

interface CastleViewProps {
  /** PicoClaw WebSocket URL. Always "ws://127.0.0.1:42817/ws" on device. */
  wsURL: string;
  style?: ViewStyle;
  roomLabel?: string;
  miloStateLabel?: string;
  nightlyRitual?: NightlyRitualState | null;
}

export const CastleView: React.FC<CastleViewProps> = ({ wsURL, style, roomLabel, miloStateLabel, nightlyRitual }) => {
  const started = useRef(false);
  const rendererStatus = getCastleRendererStatus();

  const startNativeRenderer = useCallback(() => {
    if (started.current || Platform.OS !== "android" || !rendererStatus.available) return;
    started.current = true;

    const { CastleModule } = NativeModules;
    if (CastleModule?.start) {
      CastleModule.start(wsURL);
    }
  }, [rendererStatus.available, wsURL]);

  const handleLayout = useCallback(
    (_event: LayoutChangeEvent) => {
      startNativeRenderer();
    },
    [startNativeRenderer]
  );

  if (rendererStatus.available) {
    return <EbitenView style={style} onLayout={handleLayout} />;
  }

  const ritualPalette = nightlyRitual ? RITUAL_PALETTES[nightlyRitual.status] : null;
  const chamberLabel = roomLabel ?? "Main Hall";
  const stateLabel = miloStateLabel ?? "Idle";
  const title = nightlyRitual ? nightlyRitual.chamber : "Milo's Wizard Lair";
  const subtitle = nightlyRitual
    ? `${nightlyRitual.visual}\n${nightlyRitual.vocalCue}`
    : rendererStatus.degradation?.message ?? 'Castle renderer loading...\nBuild castle.aar to activate.';

  return (
    <View style={[styles.placeholder, ritualPalette?.background && { backgroundColor: ritualPalette.background }, style]}>
      <View style={[styles.glow, ritualPalette?.glow && { backgroundColor: ritualPalette.glow }]} />
      <View style={styles.starField}>
        {STARS.map((s, i) => (
          <View
            key={i}
            style={[
              styles.star,
              { top: s.top, left: s.left, opacity: s.opacity },
              ritualPalette?.star && { backgroundColor: ritualPalette.star }
            ]}
          />
        ))}
      </View>
      <View style={styles.castleContainer}>
        <Text style={styles.castleEmoji}>🏰</Text>
        <Text style={styles.miloEmoji}>🦆</Text>
        <Text style={[styles.label, ritualPalette?.text && { color: ritualPalette.text }]}>{title}</Text>
        <Text style={[styles.sublabel, ritualPalette?.subtext && { color: ritualPalette.subtext }]}>{subtitle}</Text>
        {nightlyRitual ? (
          <View style={[styles.ritualPill, ritualPalette?.pill && { backgroundColor: ritualPalette.pill }]}>
            <Text style={styles.ritualPillText}>
              {nightlyRitual.status === "deferred" ? "Moon deferred" : nightlyRitual.status === "started" ? "Ritual in motion" : "Archive sealed"}
            </Text>
          </View>
        ) : null}
        {!rendererStatus.available ? (
          <View style={styles.fallbackPill}>
            <Text style={styles.fallbackPillText}>Expo fallback active</Text>
          </View>
        ) : null}
        <View style={styles.roomPill}>
          <Text style={styles.roomPillText}>⚗️  {chamberLabel}</Text>
        </View>
        <View style={styles.statePill}>
          <Text style={styles.statePillText}>✦ {stateLabel}</Text>
        </View>
      </View>
    </View>
  );
};

// Deterministic star positions so the layout is stable across renders
const STARS = Array.from({ length: 30 }, (_, i) => ({
  top: `${(i * 37 + 13) % 90}%` as DimensionValue,
  left: `${(i * 53 + 7) % 95}%` as DimensionValue,
  opacity: 0.2 + ((i * 17) % 60) / 100,
}));

const RITUAL_PALETTES = {
  deferred: {
    background: "#120E21",
    glow: "rgba(129, 98, 196, 0.18)",
    star: "#BBA4FF",
    text: "#F1E9FF",
    subtext: "#B6A3D9",
    pill: "rgba(73, 52, 113, 0.82)"
  },
  started: {
    background: "#0A1426",
    glow: "rgba(94, 161, 255, 0.18)",
    star: "#A9D2FF",
    text: "#EAF4FF",
    subtext: "#9BB7D8",
    pill: "rgba(29, 60, 112, 0.82)"
  },
  completed: {
    background: "#0B1611",
    glow: "rgba(110, 213, 159, 0.16)",
    star: "#BFF1D9",
    text: "#ECFFF5",
    subtext: "#A2CCB7",
    pill: "rgba(30, 89, 63, 0.82)"
  }
} as const;

const styles = StyleSheet.create({
  placeholder: {
    backgroundColor: "#0D0A1A",
    overflow: "hidden",
    position: "relative",
  },
  glow: {
    position: "absolute",
    width: 260,
    height: 260,
    borderRadius: 130,
    top: "14%",
    alignSelf: "center",
  },
  starField: {
    ...StyleSheet.absoluteFillObject,
  },
  star: {
    position: "absolute",
    width: 2,
    height: 2,
    borderRadius: 1,
    backgroundColor: "#C8B8FF",
  },
  castleContainer: {
    flex: 1,
    alignItems: "center",
    justifyContent: "center",
    gap: 10,
  },
  castleEmoji: {
    fontSize: 64,
  },
  miloEmoji: {
    fontSize: 40,
    marginTop: -12,
  },
  label: {
    color: "#E8D5FF",
    fontSize: 20,
    fontWeight: "700",
    marginTop: 8,
    letterSpacing: 0.5,
  },
  sublabel: {
    color: "#8B7BAB",
    fontSize: 13,
    textAlign: "center",
    lineHeight: 20,
  },
  roomPill: {
    marginTop: 12,
    paddingHorizontal: 18,
    paddingVertical: 8,
    borderRadius: 999,
    backgroundColor: "#1E1535",
    borderWidth: 1,
    borderColor: "#4B3B7A",
  },
  roomPillText: {
    color: "#C8B8FF",
    fontWeight: "600",
    fontSize: 14,
  },
  statePill: {
    marginTop: 8,
    paddingHorizontal: 14,
    paddingVertical: 7,
    borderRadius: 999,
    backgroundColor: "rgba(13, 10, 26, 0.55)",
    borderWidth: 1,
    borderColor: "rgba(200, 184, 255, 0.14)",
  },
  statePillText: {
    color: "#D9CBFA",
    fontWeight: "600",
    fontSize: 13,
  },
  ritualPill: {
    marginTop: 10,
    paddingHorizontal: 16,
    paddingVertical: 8,
    borderRadius: 999,
    borderWidth: 1,
    borderColor: "rgba(255,255,255,0.08)",
  },
  ritualPillText: {
    color: "#F8F3FF",
    fontWeight: "700",
    fontSize: 12,
    letterSpacing: 0.4,
    textTransform: "uppercase",
  },
  fallbackPill: {
    marginTop: 8,
    paddingHorizontal: 14,
    paddingVertical: 8,
    borderRadius: 999,
    backgroundColor: "rgba(17, 24, 39, 0.72)",
    borderWidth: 1,
    borderColor: "rgba(203, 213, 225, 0.14)",
  },
  fallbackPillText: {
    color: "#E2E8F0",
    fontWeight: "700",
    fontSize: 12,
    letterSpacing: 0.4,
    textTransform: "uppercase",
  },
});
