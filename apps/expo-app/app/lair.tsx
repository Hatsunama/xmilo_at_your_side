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
  BackHandler,
  Keyboard,
  KeyboardAvoidingView,
  Platform,
  Pressable,
  StyleSheet,
  Text,
  TextInput,
  View,
} from "react-native";

import { usePersistentCastleSurface } from "../src/components/PersistentCastleSurface";
import { useApp } from "../src/state/AppContext";
import { submitCommand, taskInterrupt, setContext, clearContext } from "../src/lib/bridge";

const ANDROID_KEYBOARD_CLEARANCE = 56;

export default function WizardLairScreen() {
  const router = useRouter();
  const { state, events, nightlyRitual, pushEvent } = useApp();
  const [prompt, setPrompt] = useState("");
  const [busy, setBusy] = useState(false);
  const [inputVisible, setInputVisible] = useState(false);
  const [sendStatus, setSendStatus] = useState("");
  const [sendError, setSendError] = useState("");
  const [keyboardOffset, setKeyboardOffset] = useState(0);
  const [pastedContext, setPastedContext] = useState<{ preview: string; charCount: number } | null>(null);
  const promptLenRef = useRef(0);
  const castleSurface = usePersistentCastleSurface();

  // Chars added in a single onChangeText that signals a paste rather than typing.
  const PASTE_THRESHOLD = 800;

  useEffect(() => {
    const subscription = BackHandler.addEventListener("hardwareBackPress", () => {
      leaveLair();
      return true;
    });
    return () => subscription.remove();
  }, []);

  useEffect(() => {
    const showSubscription = Keyboard.addListener("keyboardDidShow", (event) => {
      setKeyboardOffset(Platform.OS === "android" ? event.endCoordinates.height + ANDROID_KEYBOARD_CLEARANCE : 0);
    });
    const hideSubscription = Keyboard.addListener("keyboardDidHide", () => {
      setKeyboardOffset(0);
    });
    return () => {
      showSubscription.remove();
      hideSubscription.remove();
    };
  }, []);

  function leaveLair() {
    router.replace("/");
  }

  function hidePrompt() {
    Keyboard.dismiss();
    setInputVisible(false);
  }

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
    setSendError("");
    setSendStatus("Sending...");
    setBusy(true);
    try {
      await submitCommand(nextPrompt.trim());
      setPrompt("");
      promptLenRef.current = 0;
      setInputVisible(false);
      setSendStatus("Sent to Milo");
      // Context persists in sidecar until user explicitly dismisses the chip
    } catch (error: any) {
      const message = error?.message ?? "Failed to send prompt";
      setInputVisible(true);
      setSendStatus("");
      setSendError(message);
      pushEvent({
        type: "runtime.error",
        timestamp: new Date().toISOString(),
        payload: { message, recoverable: true }
      });
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

  const roomLabel = ROOM_LABELS[state.current_room_id ?? "main_hall"] ?? "Main Hall";
  const miloStateLabel = STATE_LABELS[state.milo_state ?? "idle"] ?? "Idle";
  const lastEvent = events.length > 0 ? events[events.length - 1] : null;

  return (
    <KeyboardAvoidingView
      behavior={Platform.OS === "ios" ? "padding" : undefined}
      style={styles.screen}
    >
      {!castleSurface.ready ? (
        <View style={[StyleSheet.absoluteFill, styles.wsPrep]}>
          <Text style={styles.wsPrepTitle}>Preparing castle link…</Text>
          <Text style={styles.wsPrepBody}>
            {castleSurface.prepError ? `Localhost bearer token not ready: ${castleSurface.prepError}` : "Loading localhost bearer token."}
          </Text>
        </View>
      ) : null}

      <View
        style={styles.gestureSurface}
        onStartShouldSetResponder={() => true}
        onMoveShouldSetResponder={() => true}
        onTouchStart={({ nativeEvent }) => castleSurface.emitGesturePacket("start", nativeEvent)}
        onTouchMove={({ nativeEvent }) => castleSurface.emitGesturePacket("move", nativeEvent)}
        onTouchEnd={({ nativeEvent }) => castleSurface.emitGesturePacket("end", nativeEvent)}
        onTouchCancel={({ nativeEvent }) => castleSurface.emitGesturePacket("cancel", nativeEvent)}
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

      {/* Bottom controls */}
      <View
        style={[
          styles.bottomControls,
          Platform.OS === "android" ? { bottom: keyboardOffset } : null,
        ]}
      >
        {sendStatus || sendError ? (
          <View style={[
            styles.sendStatusBox,
            sendError ? styles.sendStatusBoxError : null,
            !inputVisible ? styles.sendStatusBoxClosed : null,
          ]}>
            <Text style={[styles.sendStatusText, sendError ? styles.sendStatusTextError : null]} numberOfLines={2}>
              {sendError ? `Send failed: ${sendError}` : sendStatus}
            </Text>
          </View>
        ) : null}
        <View style={styles.controlRow}>
          <Pressable
            style={styles.controlButton}
            onPress={leaveLair}
          >
            <Text style={styles.controlButtonText}>← Back</Text>
          </Pressable>
          <Pressable
            style={[styles.primaryButton, busy && styles.disabled]}
            disabled={busy}
            onPress={() => {
              if (inputVisible) {
                hidePrompt();
              } else {
                setInputVisible(true);
              }
            }}
          >
            <Text style={styles.primaryButtonText}>
              {busy ? "Working..." : inputVisible ? "Hide prompt" : "✦ Send Milo a prompt"}
            </Text>
          </Pressable>
          {busy && (
            <Pressable style={styles.controlButton} onPress={interrupt}>
              <Text style={styles.controlButtonText}>Stop</Text>
            </Pressable>
          )}
        </View>
        {inputVisible ? (
          <View
            style={styles.inputRow}
          >
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
        ) : null}
      </View>
    </KeyboardAvoidingView>
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
    backgroundColor: "transparent",
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
  bottomControls: {
    position: "absolute",
    bottom: 0,
    left: 0,
    right: 0,
    padding: 16,
    paddingBottom: 28,
    backgroundColor: "transparent",
    borderTopWidth: 0,
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
    marginTop: 10,
  },
  sendStatusBox: {
    backgroundColor: "rgba(46, 36, 83, 0.92)",
    borderRadius: 10,
    borderWidth: 1,
    borderColor: "rgba(123, 95, 214, 0.45)",
    paddingHorizontal: 12,
    paddingVertical: 8,
    marginBottom: 10,
  },
  sendStatusBoxError: {
    backgroundColor: "rgba(68, 30, 42, 0.94)",
    borderColor: "rgba(255, 126, 154, 0.5)",
  },
  sendStatusBoxClosed: {
    backgroundColor: "transparent",
    borderWidth: 0,
    paddingHorizontal: 0,
    paddingVertical: 2,
    marginBottom: 6,
  },
  sendStatusText: {
    color: "#D7CAEE",
    fontSize: 13,
    fontWeight: "700",
  },
  sendStatusTextError: {
    color: "#FFC7D2",
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
