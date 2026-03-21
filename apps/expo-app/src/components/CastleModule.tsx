/**
 * CastleModule.tsx
 *
 * React Native wrapper for the Ebiten castle-go renderer.
 *
 * CURRENT STATE (v11):
 * - Returns a styled placeholder View with Milo's color palette so layout works
 *   immediately without the .aar built yet.
 * - Once castle.aar is built via ebitenmobile, swap the placeholder for
 *   the NativeViewComponent lines (commented below).
 *
 * See castle-go/BUILD.md for the full build sequence.
 */

import React, { useEffect, useRef } from "react";
import {
  NativeModules,
  Platform,
  StyleSheet,
  Text,
  View,
  type ViewStyle,
} from "react-native";

// Uncomment after castle.aar is built and placed in android/app/libs/:
// import { requireNativeComponent } from "react-native";
// const EbitenView = requireNativeComponent<{ style?: ViewStyle }>("EbitenView");

interface CastleViewProps {
  /** PicoClaw WebSocket URL. Always "ws://127.0.0.1:42817/ws" on device. */
  wsURL: string;
  style?: ViewStyle;
}

export const CastleView: React.FC<CastleViewProps> = ({ wsURL, style }) => {
  const started = useRef(false);

  useEffect(() => {
    if (started.current || Platform.OS !== "android") return;
    started.current = true;

    const { CastleModule } = NativeModules;
    if (CastleModule?.start) {
      CastleModule.start(wsURL);
    }
    // If CastleModule is not found, we silently fall through to the placeholder.
    // No error thrown — the rest of the app keeps working.
  }, [wsURL]);

  // ── Placeholder: active until .aar is built ──────────────────────────────
  // This renders a dark castle-themed panel so the Lair screen has real
  // content during development. Replace this entire return with:
  //   return <EbitenView style={style} />;
  // once the .aar is built and the NativeModule is registered.

  return (
    <View style={[styles.placeholder, style]}>
      <View style={styles.starField}>
        {STARS.map((s, i) => (
          <View
            key={i}
            style={[styles.star, { top: s.top, left: s.left, opacity: s.opacity }]}
          />
        ))}
      </View>
      <View style={styles.castleContainer}>
        <Text style={styles.castleEmoji}>🏰</Text>
        <Text style={styles.miloEmoji}>🦆</Text>
        <Text style={styles.label}>Milo's Wizard Lair</Text>
        <Text style={styles.sublabel}>
          Castle renderer loading...{"\n"}
          Build castle.aar to activate.
        </Text>
        <View style={styles.roomPill}>
          <Text style={styles.roomPillText}>⚗️  Main Hall</Text>
        </View>
      </View>
    </View>
  );
};

// Deterministic star positions so the layout is stable across renders
const STARS = Array.from({ length: 30 }, (_, i) => ({
  top: `${(i * 37 + 13) % 90}%`,
  left: `${(i * 53 + 7) % 95}%`,
  opacity: 0.2 + ((i * 17) % 60) / 100,
}));

const styles = StyleSheet.create({
  placeholder: {
    backgroundColor: "#0D0A1A",
    overflow: "hidden",
    position: "relative",
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
});
