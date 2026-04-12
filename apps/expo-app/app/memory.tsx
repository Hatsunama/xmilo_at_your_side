import { useEffect, useMemo, useRef, useState } from "react";
import { useRouter } from "expo-router";
import * as Clipboard from "expo-clipboard";
import { Alert, Pressable, ScrollView, StyleSheet, Text, TextInput, View } from "react-native";

import { useApp } from "../src/state/AppContext";
import { startTask } from "../src/lib/bridge";

const MEMORY_CLASSES = [
  "user_preference",
  "user_profile",
  "task_continuity",
  "approved_summary",
  "runtime_observation",
  "quarantined",
] as const;

type MemoryClass = (typeof MEMORY_CLASSES)[number];

type TaskStartOk = {
  task_id?: string;
  intake_gate?: any;
};

type TaskStartError = Error & {
  error_code?: string;
  intake_gate?: any;
};

function safeJson(value: any) {
  try {
    return JSON.stringify(value, null, 2);
  } catch {
    return String(value);
  }
}

export default function MemoryScreen() {
  const router = useRouter();
  const { events, state } = useApp();
  const [selectedClass, setSelectedClass] = useState<MemoryClass>("user_preference");
  const [key, setKey] = useState("");
  const [value, setValue] = useState("");
  const [busy, setBusy] = useState(false);
  const [lastIntakeGate, setLastIntakeGate] = useState<any>(null);
  const [lastStartErrorCode, setLastStartErrorCode] = useState<string>("");
  const [lastTaskId, setLastTaskId] = useState<string>("");
  const [lastMemoryPhase, setLastMemoryPhase] = useState<string>("");
  const [lastMemoryMessage, setLastMemoryMessage] = useState<string>("");
  const [lastCompletedSummary, setLastCompletedSummary] = useState<string>("");
  const lastSeenEventTsRef = useRef<string>("");

  const pendingLabel = useMemo(() => {
    if (state.pending_approval) return "Pending (state.pending_approval)";
    if (lastMemoryPhase) return "Pending (memory phase in progress)";
    return "No";
  }, [lastMemoryPhase, state.pending_approval]);

  const verifiedLabel = useMemo(() => {
    const gateText = safeJson(lastIntakeGate);
    return gateText.toLowerCase().includes("verified") ? "Yes (runtime safety_status)" : "Unknown";
  }, [lastIntakeGate]);

  const rememberedLabel = useMemo(() => {
    return lastCompletedSummary ? "Yes (task.completed after memory phase)" : "No";
  }, [lastCompletedSummary]);

  useEffect(() => {
    if (!events.length) return;
    const lastEvent = events[events.length - 1];
    if (lastEvent.timestamp === lastSeenEventTsRef.current) return;
    lastSeenEventTsRef.current = lastEvent.timestamp;

    if (lastEvent.type === "task.intake_evaluated") {
      const assessment = (lastEvent.payload as any)?.assessment;
      if (assessment) setLastIntakeGate(assessment);
    }

    if (lastEvent.type === "task.message_emitted") {
      const phase = String((lastEvent.payload as any)?.phase ?? "");
      const message = String((lastEvent.payload as any)?.message ?? "");
      if (phase === "memory_write" || phase === "memory_read") {
        setLastMemoryPhase(phase);
        setLastMemoryMessage(message);
      }
    }

    if (lastEvent.type === "task.completed") {
      const summary = String((lastEvent.payload as any)?.summary ?? "");
      if (lastMemoryPhase) {
        setLastCompletedSummary(summary);
        setLastMemoryPhase("");
      }
    }
  }, [events, lastMemoryPhase]);

  function buildRememberPrompt() {
    const keyPart = key.trim();
    const valuePart = value.trim();
    return `Remember that (${selectedClass}) ${keyPart}: ${valuePart}`;
  }

  function buildCorrectPrompt() {
    const keyPart = key.trim();
    const valuePart = value.trim();
    return `Correct what you remember for (${selectedClass}) ${keyPart}: ${valuePart}`;
  }

  function buildForgetPrompt() {
    const keyPart = key.trim();
    return `Forget what you remember for (${selectedClass}) ${keyPart}`;
  }

  async function runTask(prompt: string) {
    if (!prompt.trim()) {
      Alert.alert("Prompt required", "Fill the fields first.");
      return;
    }
    setBusy(true);
    setLastStartErrorCode("");
    try {
      const resp = (await startTask(prompt.trim())) as TaskStartOk;
      setLastTaskId(String(resp?.task_id ?? ""));
      setLastIntakeGate(resp?.intake_gate ?? null);
    } catch (err: any) {
      const error = err as TaskStartError;
      setLastStartErrorCode(String(error?.error_code ?? ""));
      setLastIntakeGate(error?.intake_gate ?? null);
      Alert.alert("Task start failed", error?.message ?? "Unknown error");
    } finally {
      setBusy(false);
    }
  }

  async function copyRepairCommand() {
    try {
      await Clipboard.setStringAsync("/runtime-recovery");
      Alert.alert("Copied", "Recovery route copied. Open the hidden runtime recovery screen.");
    } catch (error: any) {
      Alert.alert("Copy failed", error?.message ?? "Unknown error");
    }
  }

  const intakeGateText = safeJson(lastIntakeGate);

  return (
    <ScrollView style={styles.screen} contentContainerStyle={styles.content}>
      <View style={styles.card}>
        <Text style={styles.title}>Memory</Text>
        <Text style={styles.body}>
          This surface is contract-bound: the app does not read or write memory tables directly. It only starts tasks and renders what the runtime reports.
        </Text>

        <View style={styles.row}>
          <Text style={styles.k}>Remembered</Text>
          <Text style={styles.v}>{rememberedLabel}</Text>
        </View>
        <View style={styles.row}>
          <Text style={styles.k}>Verified</Text>
          <Text style={styles.v}>{verifiedLabel}</Text>
        </View>
        <View style={styles.row}>
          <Text style={styles.k}>Pending</Text>
          <Text style={styles.v}>{pendingLabel}</Text>
        </View>

        {state.resume_checkpoint ? (
          <Text style={styles.hint}>Checkpoint: present (runtime-owned). Resume UI is only added once the contract is explicit.</Text>
        ) : null}
      </View>

      <View style={styles.card}>
        <Text style={styles.sectionTitle}>Correction and edits</Text>
        <Text style={styles.body}>
          Approved classes: {MEMORY_CLASSES.join(", ")}.
        </Text>

        <Text style={styles.label}>Class</Text>
        <TextInput style={styles.input} value={selectedClass} editable={false} />
        <View style={styles.chipRow}>
          {MEMORY_CLASSES.map((c) => (
            <Pressable key={c} style={[styles.chip, c === selectedClass && styles.chipActive]} onPress={() => setSelectedClass(c)}>
              <Text style={styles.chipText}>{c}</Text>
            </Pressable>
          ))}
        </View>

        <Text style={styles.label}>Key</Text>
        <TextInput style={styles.input} value={key} onChangeText={setKey} placeholder="Example: wake_word_enabled" placeholderTextColor="#64748B" />
        <Text style={styles.label}>Value</Text>
        <TextInput style={styles.input} value={value} onChangeText={setValue} placeholder="Example: true" placeholderTextColor="#64748B" />

        <Pressable style={[styles.button, busy && styles.disabled]} disabled={busy} onPress={() => runTask(buildRememberPrompt())}>
          <Text style={styles.buttonText}>{busy ? "Working..." : "Remember"}</Text>
        </Pressable>
        <Pressable style={[styles.secondaryButton, busy && styles.disabled]} disabled={busy} onPress={() => runTask(buildCorrectPrompt())}>
          <Text style={styles.buttonText}>Correct</Text>
        </Pressable>
        <Pressable style={[styles.secondaryButton, busy && styles.disabled]} disabled={busy} onPress={() => runTask(buildForgetPrompt())}>
          <Text style={styles.buttonText}>Forget</Text>
        </Pressable>
      </View>

      <View style={styles.card}>
        <Text style={styles.sectionTitle}>Runtime proof</Text>
        <Text style={styles.body}>
          Task id: {lastTaskId || "—"} {lastStartErrorCode ? `(error_code=${lastStartErrorCode})` : ""}
        </Text>
        {lastMemoryPhase ? <Text style={styles.hint}>Memory phase: {lastMemoryPhase}</Text> : null}
        {lastMemoryMessage ? <Text style={styles.hint}>Message: {lastMemoryMessage}</Text> : null}
        {lastCompletedSummary ? <Text style={styles.hint}>Completed summary: {lastCompletedSummary}</Text> : null}
        <Text style={styles.label}>intake_gate (raw)</Text>
        <Text selectable style={styles.code}>{intakeGateText}</Text>
      </View>

      <View style={styles.card}>
        <Text style={styles.sectionTitle}>Repair command</Text>
        <Text style={styles.body}>
          If memory tasks fail because the local runtime is down, open the app-owned hidden runtime recovery screen.
        </Text>
        <Pressable style={styles.secondaryButton} onPress={() => router.push("/runtime-recovery")}>
          <Text style={styles.buttonText}>Open runtime recovery</Text>
        </Pressable>
        <Pressable style={styles.secondaryButton} onPress={copyRepairCommand}>
          <Text style={styles.buttonText}>Copy recovery route</Text>
        </Pressable>
        <Text selectable style={styles.code}>{"/runtime-recovery"}</Text>
      </View>
    </ScrollView>
  );
}

const styles = StyleSheet.create({
  screen: { flex: 1, backgroundColor: "#0B1020" },
  content: { padding: 16, gap: 14 },
  card: { backgroundColor: "#111827", borderRadius: 18, padding: 16, borderWidth: 1, borderColor: "#1F2937" },
  title: { color: "#F8FAFC", fontSize: 22, fontWeight: "700" },
  sectionTitle: { color: "#F8FAFC", fontSize: 16, fontWeight: "700" },
  body: { color: "#CBD5E1", lineHeight: 22, marginTop: 8 },
  row: { flexDirection: "row", justifyContent: "space-between", alignItems: "center", marginTop: 10, gap: 10 },
  k: { color: "#94A3B8", fontSize: 13 },
  v: { color: "#F8FAFC", fontWeight: "700", fontSize: 13, textAlign: "right", flexShrink: 1 },
  hint: { color: "#7C6FA0", fontSize: 12, marginTop: 10 },
  label: { color: "#94A3B8", marginTop: 12, fontSize: 12, fontWeight: "700" },
  input: {
    marginTop: 8,
    backgroundColor: "#0F172A",
    borderWidth: 1,
    borderColor: "#334155",
    borderRadius: 12,
    paddingHorizontal: 14,
    paddingVertical: 12,
    color: "#F8FAFC",
  },
  chipRow: { flexDirection: "row", flexWrap: "wrap", gap: 8, marginTop: 10 },
  chip: { backgroundColor: "#0F172A", borderWidth: 1, borderColor: "#334155", paddingHorizontal: 10, paddingVertical: 8, borderRadius: 999 },
  chipActive: { borderColor: "#60A5FA" },
  chipText: { color: "#E2E8F0", fontWeight: "700", fontSize: 12 },
  button: { marginTop: 16, backgroundColor: "#2563EB", borderRadius: 14, paddingVertical: 14, alignItems: "center" },
  secondaryButton: { marginTop: 12, backgroundColor: "#1F2937", borderRadius: 14, paddingVertical: 14, alignItems: "center" },
  buttonText: { color: "#FFFFFF", fontWeight: "700" },
  disabled: { opacity: 0.5 },
  code: {
    marginTop: 10,
    backgroundColor: "#0B1020",
    borderRadius: 12,
    padding: 12,
    borderWidth: 1,
    borderColor: "#334155",
    color: "#E2E8F0",
    fontFamily: "monospace",
  },
});
