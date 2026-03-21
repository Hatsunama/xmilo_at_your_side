/**
 * lair.tsx — Milo's Wizard Lair screen
 *
 * This screen is the primary visual experience of xMilo.
 * It renders the castle scene (Ebiten via CastleView) and overlays
 * the chat input bar at the bottom so the user can give Milo tasks
 * without leaving the castle view.
 *
 * The castle renderer connects directly to PicoClaw via WebSocket.
 * Room routing, Milo movement, and all spatial decisions happen
 * inside PicoClaw — this screen just shows the result.
 *
 * Navigation: accessible from the main index screen via "Enter the Lair".
 */

import { useRouter } from "expo-router";
import { useRef, useState } from "react";
import {
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
import { startTask, taskInterrupt, setContext, clearContext } from "../src/lib/bridge";
import { SIDECAR_BASE_URL } from "../src/lib/config";

// Derive the WebSocket URL from the HTTP base URL.
// SIDECAR_BASE_URL is "http://127.0.0.1:42817" → "ws://127.0.0.1:42817/ws"
const WS_URL = SIDECAR_BASE_URL.replace("http://", "ws://").replace(
  "https://",
  "wss://"
) + "/ws";

export default function WizardLairScreen() {
  const router = useRouter();
  const { state, events } = useApp();
  const [prompt, setPrompt] = useState("");
  const [busy, setBusy] = useState(false);
  const [inputVisible, setInputVisible] = useState(false);
  const [pastedContext, setPastedContext] = useState<{ preview: string; charCount: number } | null>(null);
  const promptLenRef = useRef(0);

  // Chars added in a single onChangeText that signals a paste rather than typing.
  const PASTE_THRESHOLD = 800;

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
      await startTask(nextPrompt.trim());
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

  const roomLabel = ROOM_LABELS[state.current_room_id ?? "main_hall"] ?? "Main Hall";
  const miloStateLabel = STATE_LABELS[state.milo_state ?? "idle"] ?? "Idle";
  const lastEvent = events.length > 0 ? events[events.length - 1] : null;

  return (
    <View style={styles.screen}>
      {/* Castle scene — fills entire screen */}
      <CastleView wsURL={WS_URL} style={StyleSheet.absoluteFill} />

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
              placeholder={pastedContext ? "Now give Milo a task for this content…" : "Give Milo a task..."}
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
                {busy ? "Working..." : "✦ Give Milo a task"}
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
