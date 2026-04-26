import { usePathname } from "expo-router";
import React, { createContext, type PropsWithChildren, useCallback, useContext, useEffect, useMemo, useRef, useState } from "react";
import { Platform, StyleSheet, View } from "react-native";

import { SIDECAR_BASE_URL } from "../lib/config";
import { resolveLocalhostBearerToken } from "../lib/xmiloRuntimeHost";
import { useApp } from "../state/AppContext";
import { CastleView } from "./CastleModule";

const WS_BASE_URL = SIDECAR_BASE_URL.replace("http://", "ws://").replace(
  "https://",
  "wss://"
) + "/ws";

type CastleSurfaceContextValue = {
  active: boolean;
  ready: boolean;
  prepError: string;
  emitGesturePacket: (kind: "start" | "move" | "end" | "cancel", nativeEvent: any) => void;
};

const CastleSurfaceContext = createContext<CastleSurfaceContextValue | null>(null);

export function PersistentCastleSurfaceProvider({ children }: PropsWithChildren) {
  const pathname = usePathname();
  const { state, nightlyRitual } = useApp();
  const [wsURL, setWsURL] = useState<string | null>(null);
  const [prepError, setPrepError] = useState("");
  const [gesturePacket, setGesturePacket] = useState("");
  const lastGesturePacketRef = useRef("");
  const active = pathname === "/lair";

  useEffect(() => {
    if (!active || wsURL) return;
    let cancelled = false;

    async function prepWS() {
      try {
        const bearer = await resolveLocalhostBearerToken();
        if (cancelled) return;
        setWsURL(`${WS_BASE_URL}?token=${encodeURIComponent(bearer)}`);
        setPrepError("");
      } catch (error: any) {
        if (cancelled) return;
        setWsURL(null);
        setPrepError(error?.message ?? "Localhost bearer token was not available.");
      }
    }

    prepWS();
    return () => {
      cancelled = true;
    };
  }, [active, wsURL]);

  const emitGesturePacket = useCallback((kind: "start" | "move" | "end" | "cancel", nativeEvent: any) => {
    if (!active) return;
    const touches = (nativeEvent?.touches ?? []).map((touch: any) => ({
      identifier: touch.identifier,
      x: touch.pageX ?? touch.locationX ?? 0,
      y: touch.pageY ?? touch.locationY ?? 0,
    }));
    const changedTouches = (nativeEvent?.changedTouches ?? []).map((touch: any) => ({
      identifier: touch.identifier,
      x: touch.pageX ?? touch.locationX ?? 0,
      y: touch.pageY ?? touch.locationY ?? 0,
    }));
    const packet = JSON.stringify({
      kind,
      timestamp: Date.now(),
      touches,
      changedTouches,
    });
    if (packet !== lastGesturePacketRef.current) {
      lastGesturePacketRef.current = packet;
      setGesturePacket(packet);
    }
  }, [active]);

  const roomLabel = ROOM_LABELS[state.current_room_id ?? "main_hall"] ?? "Main Hall";
  const miloStateLabel = STATE_LABELS[state.milo_state ?? "idle"] ?? "Idle";

  const value = useMemo<CastleSurfaceContextValue>(
    () => ({
      active,
      ready: Boolean(wsURL),
      prepError,
      emitGesturePacket,
    }),
    [active, emitGesturePacket, prepError, wsURL]
  );

  return (
    <CastleSurfaceContext.Provider value={value}>
      <View style={styles.root}>
        {Platform.OS === "android" && wsURL ? (
          <CastleView
            wsURL={wsURL}
            gesturePacket={gesturePacket}
            style={styles.castleSurface}
            roomLabel={roomLabel}
            miloStateLabel={miloStateLabel}
            nightlyRitual={nightlyRitual}
          />
        ) : null}
        <View style={styles.routeLayer}>{children}</View>
      </View>
    </CastleSurfaceContext.Provider>
  );
}

export function usePersistentCastleSurface() {
  const value = useContext(CastleSurfaceContext);
  if (!value) throw new Error("usePersistentCastleSurface must be used within PersistentCastleSurfaceProvider");
  return value;
}

const ROOM_LABELS: Record<string, string> = {
  main_hall: "Main Hall",
  trophy_room: "Trophy Room",
  study: "Study",
  workshop: "Workshop",
  observatory: "Observatory",
  potions_room: "Potions Room",
  war_room: "War Room",
  library: "Library",
  training_room: "Training Room",
  spellbook: "Spellbook",
  cauldron: "Cauldron",
  crystal_orb: "Crystal Orb",
  baby_dragon: "Dragon's Den",
  trophy: "Trophy Hall",
  archive: "Archive",
};

const STATE_LABELS: Record<string, string> = {
  idle: "Idle",
  moving: "Moving",
  working: "Working",
};

const styles = StyleSheet.create({
  root: {
    flex: 1,
    backgroundColor: "#0B1020",
  },
  castleSurface: {
    ...StyleSheet.absoluteFillObject,
    zIndex: 0,
  },
  routeLayer: {
    flex: 1,
    zIndex: 1,
  },
});
