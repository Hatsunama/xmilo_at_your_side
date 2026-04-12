/**
 * lair.tsx — Milo's Wizard Lair screen
 *
 * This screen is the primary visual experience of xMilo.
 * It renders the castle scene (Ebiten via CastleView) and overlays
 * the chat input bar at the bottom so the user can give Milo tasks
 * without leaving the castle view.
 *
 * The castle renderer connects directly to the xMilo Sidecar via WebSocket.
 * Room routing, Milo movement, and all spatial decisions happen
 * inside the sidecar runtime — this screen just shows the result.
 *
 * Navigation: accessible from the main index screen via "Enter the Lair".
 */

import { useRouter } from "expo-router";
import { useEffect, useRef, useState } from "react";
import {
  NativeModules,
  KeyboardAvoidingView,
  Platform,
  Pressable,
  StyleSheet,
  Text,
  TextInput,
  View,
} from "react-native";

import { CastleView } from "../src/components/CastleModule";
import { useApp } from "../src/state/AppContext";
import { submitCommand, taskInterrupt, setContext, clearContext } from "../src/lib/bridge";
import { SIDECAR_BASE_URL } from "../src/lib/config";
import { resolveLocalhostBearerToken } from "../src/lib/xmiloRuntimeHost";

// Derive the WebSocket URL from the HTTP base URL.
// SIDECAR_BASE_URL is "http://127.0.0.1:42817" → "ws://127.0.0.1:42817/ws"
const WS_BASE_URL = SIDECAR_BASE_URL.replace("http://", "ws://").replace(
  "https://",
  "wss://"
) + "/ws";

export default function WizardLairScreen() {
  const router = useRouter();
  const { state, events, nightlyRitual, artPresentation } = useApp();
  const [prompt, setPrompt] = useState("");
  const [busy, setBusy] = useState(false);
  const [inputVisible, setInputVisible] = useState(false);
  const [pastedContext, setPastedContext] = useState<{ preview: string; charCount: number } | null>(null);
  const [wsURL, setWsURL] = useState<string | null>(null);
  const [wsPrepError, setWsPrepError] = useState<string>("");
  const [gesturePacket, setGesturePacket] = useState("");
  const promptLenRef = useRef(0);
  const lastGesturePacketRef = useRef("");
  const nativePresenterLaunchedRef = useRef(false);
  const nativeCastleModule = NativeModules.CastleModule as { start?: (wsURL: string) => void } | undefined;
  // Dedicated native presenter disabled: NativeCastleActivity launched via FLAG_ACTIVITY_NEW_TASK
  // from a reactApplicationContext, which creates a separate Android task stack and breaks
  // back navigation (launcher bounce, no HUD/controls). Route through CastleView/EbitenView instead.
  const useDedicatedNativePresenter = false;

  // Chars added in a single onChangeText that signals a paste rather than typing.
  const PASTE_THRESHOLD = 800;

  useEffect(() => {
    let cancelled = false;

    async function prepWS() {
      try {
        const bearer = await resolveLocalhostBearerToken();
        if (cancelled) return;
        setWsURL(`${WS_BASE_URL}?token=${encodeURIComponent(bearer)}`);
        setWsPrepError("");
      } catch (e: any) {
        if (cancelled) return;
        setWsURL(null);
        setWsPrepError(e?.message ?? "Localhost bearer token was not available.");
      }
    }

    prepWS();
    return () => {
      cancelled = true;
    };
  }, []);

  useEffect(() => {
    if (!useDedicatedNativePresenter || !wsURL || nativePresenterLaunchedRef.current) {
      return;
    }
    nativePresenterLaunchedRef.current = true;
    nativeCastleModule?.start?.(wsURL);
  }, [nativeCastleModule, useDedicatedNativePresenter, wsURL]);

  async function handleTextChange(text: string) {
    const delta = text.length - promptLenRef.current;
    promptLenRef.current = text.length;

    if (delta > PASTE_THRESHOLD && text.length > PASTE_THRESHOLD) {
      // Large paste detected — store in sidecar, show chip, clear input
      const preview = text.slice(0, 80).replace(/\n/g, " ");
      setPastedContext({ preview, charCount: text.length });
      setPrompt("");
      promptLenRef.current = 0;
      try { await setContext(text); } catch (_) {}
      return;
    }
    setPrompt(text);
  }

  async function dismissPaste() {
    setPastedContext(null);
    try { await clearContext(); } catch (_) {}
  }

  async function submit(nextPrompt: string) {
    if (!nextPrompt.trim()) return;
    setBusy(true);
    setInputVisible(false);
    try {
      await submitCommand(nextPrompt.trim());
      setPrompt("");
      promptLenRef.current = 0;
      // Context persists in sidecar until user explicitly dismisses the chip
    } catch (_err) {
      // error event will surface via the WS bridge
    } finally {
      setBusy(false);
    }
  }

  async function interrupt() {
    setBusy(true);
    try {
      await taskInterrupt();
    } finally {
      setBusy(false);
    }
  }

  function emitGesturePacket(kind: "start" | "move" | "end" | "cancel", nativeEvent: any) {
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
  }

  const roomLabel = ROOM_LABELS[state.current_room_id ?? "main_hall"] ?? "Main Hall";
  const miloStateLabel = STATE_LABELS[state.milo_state ?? "idle"] ?? "Idle";
  const lastEvent = events.length > 0 ? events[events.length - 1] : null;

  return (
    <View style={styles.screen}>
      {/* Castle scene — fills entire screen */}
      {useDedicatedNativePresenter ? (
        <View style={StyleSheet.absoluteFill} pointerEvents="none" />
      ) : wsURL ? (
        <CastleView
          wsURL={wsURL}
          gesturePacket={gesturePacket}
          style={StyleSheet.absoluteFill}
          roomLabel={roomLabel}
          miloStateLabel={miloStateLabel}
          nightlyRitual={nightlyRitual}
        />
      ) : (
        <View style={[StyleSheet.absoluteFill, styles.wsPrep]}>
          <Text style={styles.wsPrepTitle}>Preparing castle link…</Text>
          <Text style={styles.wsPrepBody}>
            {wsPrepError ? `Localhost bearer token not ready: ${wsPrepError}` : "Loading localhost bearer token."}
          </Text>
        </View>
      )}

      <View
        style={styles.gestureSurface}
        onStartShouldSetResponder={() => true}
        onMoveShouldSetResponder={() => true}
        onTouchStart={({ nativeEvent }) => emitGesturePacket("start", nativeEvent)}
        onTouchMove={({ nativeEvent }) => emitGesturePacket("move", nativeEvent)}
        onTouchEnd={({ nativeEvent }) => emitGesturePacket("end", nativeEvent)}
        onTouchCancel={({ nativeEvent }) => emitGesturePacket("cancel", nativeEvent)}
      />

      {/* Top HUD — room + state indicators */}
      <View style={styles.topHud} pointerEvents="none">
        <View style={styles.hudPill}>
          <Text style={styles.hudPillText}>
            {ROOM_ICONS[state.current_room_id ?? "main_hall"] ?? "🏰"} {roomLabel}
          </Text>
        </View>
        <View style={[styles.hudPill, styles.statePill]}>
          <Text style={styles.hudPillText}>
            {STATE_ICONS[state.milo_state ?? "idle"] ?? "💤"} {miloStateLabel}
          </Text>
        </View>
      </View>

      {/* Last event ticker — shows what Milo is doing */}
      {lastEvent && (
        <View style={styles.tickerBar} pointerEvents="none">
          <Text style={styles.tickerText} numberOfLines={1}>
            {formatEvent(lastEvent)}
          </Text>
        </View>
      )}

      {nightlyRitual ? (
        <View style={[styles.ritualBanner, styles[`ritualBanner_${nightlyRitual.status}` as const]]} pointerEvents="none">
          <Text style={styles.ritualBannerEyebrow}>Nightly Ritual</Text>
          <Text style={styles.ritualBannerTitle}>{nightlyRitual.title}</Text>
          <Text style={styles.ritualBannerBody}>{nightlyRitual.visual}</Text>
          <Text style={styles.ritualBannerCue}>{nightlyRitual.physicalCue}</Text>
        </View>
      ) : null}

      {artPresentation.degradation_reason ? (
        <View style={styles.degradationCard} pointerEvents="none">
          <Text style={styles.degradationEyebrow}>Renderer Fallback</Text>
          <Text style={styles.degradationTitle}>Expo presentation active</Text>
          <Text style={styles.degradationBody}>{artPresentation.degradation_reason.message}</Text>
        </View>
      ) : null}

      {/* Bottom controls */}
      <KeyboardAvoidingView
        behavior={Platform.OS === "ios" ? "padding" : "height"}
        style={styles.bottomControls}
      >
        {inputVisible ? (
          <View style={styles.inputRow}>
            {pastedContext && (
              <View style={styles.pasteChip}>
                <Text style={styles.pasteChipText} numberOfLines={1}>
                  📎 {pastedContext.charCount.toLocaleString()} chars · "{pastedContext.preview}…"
                </Text>
                <Pressable onPress={dismissPaste} hitSlop={8} style={styles.pasteChipDismiss}>
                  <Text style={styles.pasteChipDismissText}>✕</Text>
                </Pressable>
              </View>
            )}
            <View style={styles.pasteInputRow}>
            <TextInput
              autoFocus
              multiline
              placeholder={pastedContext ? "Now give Milo a prompt for this content…" : "Give Milo a prompt or send him somewhere..."}
              placeholderTextColor="#6B5A8A"
              style={styles.input}
              value={prompt}
              onChangeText={handleTextChange}
              maxLength={8000}
            />
            <Pressable
              style={[styles.sendButton, busy && styles.disabled]}
              disabled={busy}
              onPress={() => submit(prompt)}
            >
              <Text style={styles.sendButtonText}>
                {busy ? "..." : "▶"}
              </Text>
            </Pressable>
            </View>
          </View>
        ) : (
          <View style={styles.controlRow}>
            <Pressable
              style={styles.controlButton}
              onPress={() => router.back()}
            >
              <Text style={styles.controlButtonText}>← Back</Text>
            </Pressable>
            <Pressable
              style={[styles.primaryButton, busy && styles.disabled]}
              disabled={busy}
              onPress={() => setInputVisible(true)}
            >
              <Text style={styles.primaryButtonText}>
                {busy ? "Working..." : "✦ Send Milo a prompt"}
              </Text>
            </Pressable>
            {busy && (
              <Pressable style={styles.controlButton} onPress={interrupt}>
                <Text style={styles.controlButtonText}>Stop</Text>
              </Pressable>
            )}
          </View>
        )}
      </KeyboardAvoidingView>
    </View>
  );
}

// ── Room metadata ─────────────────────────────────────────────────────────────

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

const ROOM_ICONS: Record<string, string> = {
  main_hall: "🏰",
  trophy_room: "🏆",
  study: "📚",
  workshop: "🛠️",
  observatory: "🔭",
  potions_room: "⚗️",
  war_room: "⚔️",
  library: "📚",
  training_room: "🥋",
  spellbook: "✨",
  cauldron: "⚗️",
  crystal_orb: "🔮",
  baby_dragon: "🐉",
  trophy: "🏆",
  archive: "🗃️",
};

const STATE_LABELS: Record<string, string> = {
  idle: "Idle",
  moving: "Moving",
  working: "Working",
};

const STATE_ICONS: Record<string, string> = {
  idle: "💤",
  moving: "🚶",
  working: "⚡",
};

function formatEvent(event: { type: string; payload?: any }): string {
  switch (event.type) {
    case "maintenance.nightly_deferred":
      return "⏳ Nightly upkeep is waiting for the current task to finish";
    case "maintenance.nightly_started":
      return "🌙 Milo has started the 2 AM upkeep ritual";
    case "maintenance.nightly_completed":
      return "✨ Nightly upkeep complete. Archive sealed and update check finished";
    case "milo.movement_started":
      return `Milo is heading to ${ROOM_LABELS[event.payload?.to_room] ?? event.payload?.to_room}...`;
    case "milo.room_changed":
      return `Milo arrived at ${ROOM_LABELS[event.payload?.room_id] ?? event.payload?.room_id}`;
    case "task.completed":
      return `✓ Done: ${event.payload?.summary ?? "Task complete"}`;
    case "task.stuck":
      return `⚠ Stuck: ${event.payload?.reason ?? "Unknown error"}`;
    case "milo.thought":
      return `💭 ${event.payload?.text ?? ""}`;
    case "report.ready":
      return `📜 Report ready`;
    default:
      return event.type;
  }
}

// ── Styles ────────────────────────────────────────────────────────────────────

const styles = StyleSheet.create({
  screen: {
    flex: 1,
    backgroundColor: "#0D0A1A",
  },
  wsPrep: {
    alignItems: "center",
    justifyContent: "center",
    paddingHorizontal: 22,
    gap: 10,
    backgroundColor: "#0D0A1A",
  },
  wsPrepTitle: {
    color: "#EAF4FF",
    fontSize: 18,
    fontWeight: "700",
    textAlign: "center",
  },
  wsPrepBody: {
    color: "#9BB7D8",
    fontSize: 13,
    textAlign: "center",
    lineHeight: 18,
  },
  topHud: {
    position: "absolute",
    top: 52,
    left: 16,
    right: 16,
    flexDirection: "row",
    gap: 8,
    zIndex: 10,
  },
  hudPill: {
    paddingHorizontal: 12,
    paddingVertical: 6,
    borderRadius: 999,
    backgroundColor: "rgba(13, 10, 26, 0.82)",
    borderWidth: 1,
    borderColor: "rgba(200, 184, 255, 0.2)",
  },
  statePill: {
    marginLeft: "auto",
  },
  hudPillText: {
    color: "#C8B8FF",
    fontWeight: "600",
    fontSize: 13,
  },
  tickerBar: {
    position: "absolute",
    bottom: 110,
    left: 16,
    right: 16,
    backgroundColor: "rgba(13, 10, 26, 0.75)",
    borderRadius: 10,
    paddingHorizontal: 14,
    paddingVertical: 8,
    borderWidth: 1,
    borderColor: "rgba(200, 184, 255, 0.15)",
    zIndex: 10,
  },
  tickerText: {
    color: "#B8A8D8",
    fontSize: 13,
  },
  ritualBanner: {
    position: "absolute",
    top: 104,
    right: 16,
    width: 188,
    borderRadius: 16,
    paddingHorizontal: 14,
    paddingVertical: 12,
    borderWidth: 1,
    zIndex: 10,
  },
  ritualBanner_deferred: {
    backgroundColor: "rgba(39, 26, 58, 0.86)",
    borderColor: "rgba(160, 122, 216, 0.38)",
  },
  ritualBanner_started: {
    backgroundColor: "rgba(16, 30, 54, 0.88)",
    borderColor: "rgba(110, 162, 255, 0.38)",
  },
  ritualBanner_completed: {
    backgroundColor: "rgba(18, 41, 30, 0.88)",
    borderColor: "rgba(96, 210, 151, 0.34)",
  },
  ritualBannerEyebrow: {
    color: "#C8B8FF",
    fontSize: 11,
    fontWeight: "700",
    textTransform: "uppercase",
    letterSpacing: 1,
  },
  ritualBannerTitle: {
    color: "#F3ECFF",
    fontSize: 15,
    fontWeight: "700",
    marginTop: 4,
  },
  ritualBannerBody: {
    color: "#D7CAEE",
    fontSize: 12,
    lineHeight: 18,
    marginTop: 6,
  },
  ritualBannerCue: {
    color: "#BCAEE0",
    fontSize: 11,
    lineHeight: 16,
    marginTop: 6,
    fontStyle: "italic",
  },
  degradationCard: {
    position: "absolute",
    left: 16,
    right: 16,
    bottom: 162,
    borderRadius: 14,
    paddingHorizontal: 14,
    paddingVertical: 12,
    backgroundColor: "rgba(12, 18, 31, 0.82)",
    borderWidth: 1,
    borderColor: "rgba(148, 163, 184, 0.22)",
    zIndex: 10,
  },
  degradationEyebrow: {
    color: "#93C5FD",
    fontSize: 11,
    fontWeight: "700",
    textTransform: "uppercase",
    letterSpacing: 1,
  },
  degradationTitle: {
    color: "#F8FAFC",
    fontSize: 15,
    fontWeight: "700",
    marginTop: 4,
  },
  degradationBody: {
    color: "#CBD5E1",
    fontSize: 12,
    lineHeight: 18,
    marginTop: 6,
  },
  bottomControls: {
    position: "absolute",
    bottom: 0,
    left: 0,
    right: 0,
    padding: 16,
    paddingBottom: 28,
    backgroundColor: "rgba(13, 10, 26, 0.88)",
    borderTopWidth: 1,
    borderTopColor: "rgba(200, 184, 255, 0.12)",
    zIndex: 20,
  },
  gestureSurface: {
    position: "absolute",
    left: 0,
    right: 0,
    top: 0,
    bottom: 140,
    backgroundColor: "transparent",
    zIndex: 5,
  },
  controlRow: {
    flexDirection: "row",
    gap: 10,
    alignItems: "center",
  },
  controlButton: {
    paddingHorizontal: 16,
    paddingVertical: 12,
    borderRadius: 14,
    backgroundColor: "#1E1535",
    borderWidth: 1,
    borderColor: "#3B2D5A",
  },
  controlButtonText: {
    color: "#C8B8FF",
    fontWeight: "600",
    fontSize: 14,
  },
  primaryButton: {
    flex: 1,
    paddingVertical: 14,
    borderRadius: 14,
    backgroundColor: "#5B3FA6",
    alignItems: "center",
    borderWidth: 1,
    borderColor: "#7B5FD6",
  },
  primaryButtonText: {
    color: "#FFFFFF",
    fontWeight: "700",
    fontSize: 15,
  },
  inputRow: {
    flexDirection: "column",
  },
  input: {
    flex: 1,
    minHeight: 46,
    maxHeight: 120,
    color: "#E8D5FF",
    backgroundColor: "#1E1535",
    borderRadius: 14,
    borderWidth: 1,
    borderColor: "#4B3B7A",
    paddingHorizontal: 14,
    paddingVertical: 10,
    textAlignVertical: "top",
    fontSize: 15,
  },
  sendButton: {
    width: 46,
    height: 46,
    borderRadius: 14,
    backgroundColor: "#5B3FA6",
    alignItems: "center",
    justifyContent: "center",
  },
  sendButtonText: {
    color: "#FFFFFF",
    fontSize: 18,
    fontWeight: "700",
  },
  disabled: {
    opacity: 0.5,
  },
  pasteChip: {
    flexDirection: "row",
    alignItems: "center",
    backgroundColor: "#2A1E4A",
    borderRadius: 10,
    borderWidth: 1,
    borderColor: "#5B3FA6",
    paddingHorizontal: 12,
    paddingVertical: 7,
    marginBottom: 8,
    gap: 8,
  },
  pasteChipText: {
    flex: 1,
    color: "#C8B8FF",
    fontSize: 12,
    fontWeight: "600",
  },
  pasteChipDismiss: {
    paddingHorizontal: 4,
  },
  pasteChipDismissText: {
    color: "#7B5FD6",
    fontSize: 14,
    fontWeight: "700",
  },
  pasteInputRow: {
    flexDirection: "row",
    gap: 10,
    alignItems: "flex-end",
  },
});
