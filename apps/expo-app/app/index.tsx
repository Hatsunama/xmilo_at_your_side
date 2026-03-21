import { useRouter } from "expo-router";
import { useEffect, useMemo, useState } from "react";
import {
  Pressable,
  ScrollView,
  StyleSheet,
  Text,
  TextInput,
  View
} from "react-native";

import { MessageBubble } from "../src/components/MessageBubble";
import { StarterChips } from "../src/components/StarterChips";
import { useApp } from "../src/state/AppContext";
import { connectBridge, getState, getTaskCurrent, getHealth, startTask, taskInterrupt } from "../src/lib/bridge";

export default function MainHallScreen() {
  const router = useRouter();
  const { state, setState, events, pushEvent, tokenMissing } = useApp();
  const [prompt, setPrompt] = useState("");
  const [busy, setBusy] = useState(false);
  const [health, setHealth] = useState<string>("Unknown");

  useEffect(() => {
    let cleanup: (() => void) | undefined;

    async function boot() {
      try {
        const [healthResp, runtimeResp, taskResp] = await Promise.all([
          getHealth(),
          getState(),
          getTaskCurrent()
        ]);
        setHealth(healthResp.service ?? "xmilo-sidecar");
        setState(runtimeResp);
        if (taskResp?.task) {
          setState((prev) => ({ ...prev, active_task: taskResp.task }));
        }
      } catch (error: any) {
        setHealth(error?.message ?? "Unavailable");
      }

      cleanup = connectBridge((event) => {
        pushEvent(event);
        if (event.type === "task.accepted" || event.type === "task.completed" || event.type === "task.stuck" || event.type === "task.cancelled" || event.type === "milo.state_changed" || event.type === "milo.room_changed") {
          getState().then(setState).catch(() => null);
          getTaskCurrent().then((data) => {
            setState((prev) => ({ ...prev, active_task: data?.task ?? null }));
          }).catch(() => null);
        }
      });
    }

    boot();
    return () => cleanup?.();
  }, [pushEvent, setState]);

  const starterPrompts = useMemo(
    () => [
      "Summarize what you can do",
      "Help me plan my day",
      "Organize my priorities",
      "What should I know about this app?"
    ],
    []
  );

  async function submit(nextPrompt: string) {
    if (!nextPrompt.trim()) return;
    setBusy(true);
    try {
      await startTask(nextPrompt.trim());
      setPrompt("");
    } catch (error: any) {
      pushEvent({
        type: "runtime.error",
        timestamp: new Date().toISOString(),
        payload: { message: error?.message ?? "Failed to start task", recoverable: true }
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

  return (
    <View style={styles.screen}>
      <ScrollView contentContainerStyle={styles.content}>
        <View style={styles.heroCard}>
          <Text style={styles.kicker}>Main Hall</Text>
          <Text style={styles.title}>xMilo Main Hall — starter shell</Text>
          <Text style={styles.subtitle}>
            Fixed dark theme, localhost bridge, sidecar events, and a real task input loop. Setup, access, and world-view polish are still tracked separately.
          </Text>
          <View style={styles.statsRow}>
            <InfoPill label="Bridge" value={health} />
            <InfoPill label="State" value={state.milo_state || "idle"} />
            <InfoPill label="Room" value={state.current_room_id || "main_hall"} />
          </View>
          {tokenMissing ? (
            <Text style={styles.warning}>Missing EXPO_PUBLIC_LOCALHOST_TOKEN. The app shell can load, but localhost calls will fail until you set it.</Text>
          ) : null}
        </View>

        <StarterChips
          prompts={starterPrompts}
          onSelect={(value) => {
            setPrompt(value);
            submit(value);
          }}
        />

        <View style={styles.composerCard}>
          <Text style={styles.sectionTitle}>Give Milo a task</Text>
          <TextInput
            multiline
            placeholder="Ask Milo something..."
            placeholderTextColor="#94A3B8"
            style={styles.input}
            value={prompt}
            onChangeText={setPrompt}
            maxLength={8000}
          />
          <Text style={styles.charCount}>{prompt.length}/8000</Text>
          <View style={styles.buttonRow}>
            <Pressable style={[styles.primaryButton, busy && styles.disabled]} disabled={busy} onPress={() => submit(prompt)}>
              <Text style={styles.primaryButtonText}>{busy ? "Working..." : "Start task"}</Text>
            </Pressable>
            <Pressable style={styles.secondaryButton} onPress={interrupt}>
              <Text style={styles.secondaryButtonText}>Interrupt</Text>
            </Pressable>
          </View>
        </View>

        <View style={styles.sectionHeader}>
          <Text style={styles.sectionTitle}>Recent events</Text>
          <Pressable onPress={() => router.push("/archive")}>
            <Text style={styles.link}>Open archive</Text>
          </Pressable>
        </View>

        {events.length === 0 ? (
          <View style={styles.emptyCard}>
            <Text style={styles.emptyTitle}>No events yet</Text>
            <Text style={styles.emptyBody}>Once the sidecar starts emitting events, they will show up here.</Text>
          </View>
        ) : (
          events.slice().reverse().map((event, index) => (
            <MessageBubble key={`${event.timestamp}-${index}`} event={event} />
          ))
        )}

        <View style={styles.footerRow}>
          <Pressable style={[styles.footerButton, styles.lairButton]} onPress={() => router.push("/lair")}>
            <Text style={styles.lairButtonText}>🏰 Enter the Lair</Text>
          </Pressable>
        </View>

        <View style={styles.footerRow}>
          <Pressable style={styles.footerButton} onPress={() => router.push("/setup")}>
            <Text style={styles.footerButtonText}>Setup</Text>
          </Pressable>
          <Pressable style={styles.footerButton} onPress={() => router.push("/settings")}>
            <Text style={styles.footerButtonText}>Settings</Text>
          </Pressable>
        </View>
      </ScrollView>
    </View>
  );
}

function InfoPill({ label, value }: { label: string; value: string }) {
  return (
    <View style={styles.pill}>
      <Text style={styles.pillLabel}>{label}</Text>
      <Text style={styles.pillValue}>{value}</Text>
    </View>
  );
}

const styles = StyleSheet.create({
  screen: { flex: 1, backgroundColor: "#0B1020" },
  content: { padding: 16, gap: 16 },
  heroCard: { backgroundColor: "#111827", borderRadius: 18, padding: 16, borderWidth: 1, borderColor: "#1F2937" },
  kicker: { color: "#93C5FD", fontSize: 12, fontWeight: "700", textTransform: "uppercase", letterSpacing: 1 },
  title: { color: "#F8FAFC", fontSize: 24, fontWeight: "700", marginTop: 6 },
  subtitle: { color: "#CBD5E1", marginTop: 8, lineHeight: 20 },
  warning: { color: "#FCA5A5", marginTop: 12, lineHeight: 18 },
  statsRow: { flexDirection: "row", flexWrap: "wrap", gap: 8, marginTop: 14 },
  pill: { paddingVertical: 8, paddingHorizontal: 12, borderRadius: 999, backgroundColor: "#0F172A", borderWidth: 1, borderColor: "#23314F" },
  pillLabel: { color: "#94A3B8", fontSize: 11 },
  pillValue: { color: "#E2E8F0", fontWeight: "600", marginTop: 2 },
  composerCard: { backgroundColor: "#111827", borderRadius: 18, padding: 16, borderWidth: 1, borderColor: "#1F2937" },
  sectionHeader: { flexDirection: "row", justifyContent: "space-between", alignItems: "center" },
  sectionTitle: { color: "#F8FAFC", fontSize: 18, fontWeight: "700" },
  link: { color: "#93C5FD", fontWeight: "600" },
  input: {
    minHeight: 120,
    color: "#F8FAFC",
    backgroundColor: "#0F172A",
    borderRadius: 14,
    borderWidth: 1,
    borderColor: "#1E293B",
    padding: 14,
    marginTop: 12,
    textAlignVertical: "top"
  },
  charCount: { color: "#94A3B8", alignSelf: "flex-end", marginTop: 8 },
  buttonRow: { flexDirection: "row", gap: 12, marginTop: 12 },
  primaryButton: { flex: 1, backgroundColor: "#2563EB", paddingVertical: 14, borderRadius: 14, alignItems: "center" },
  primaryButtonText: { color: "#FFFFFF", fontWeight: "700" },
  secondaryButton: { paddingVertical: 14, paddingHorizontal: 18, borderRadius: 14, backgroundColor: "#1F2937" },
  secondaryButtonText: { color: "#E5E7EB", fontWeight: "700" },
  disabled: { opacity: 0.6 },
  emptyCard: { backgroundColor: "#111827", borderRadius: 18, padding: 16, borderWidth: 1, borderColor: "#1F2937" },
  emptyTitle: { color: "#F8FAFC", fontWeight: "700" },
  emptyBody: { color: "#CBD5E1", marginTop: 8, lineHeight: 20 },
  footerRow: { flexDirection: "row", gap: 12 },
  footerButton: { flex: 1, backgroundColor: "#111827", borderRadius: 14, paddingVertical: 14, alignItems: "center", borderWidth: 1, borderColor: "#1F2937" },
  footerButtonText: { color: "#E5E7EB", fontWeight: "700" },
  lairButton: { backgroundColor: "#1E1535", borderColor: "#4B3B7A" },
  lairButtonText: { color: "#C8B8FF", fontWeight: "700", fontSize: 16 },
});
