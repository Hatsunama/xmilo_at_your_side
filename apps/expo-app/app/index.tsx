import { useRouter } from "expo-router";
import { useEffect, useMemo, useState } from "react";
import {
  Alert,
  Modal,
  Pressable,
  ScrollView,
  StyleSheet,
  Text,
  TextInput,
  View
} from "react-native";

import { MessageBubble } from "../src/components/MessageBubble";
import { StarterChips } from "../src/components/StarterChips";
import { clearStagedInputs, listStagedInputs } from "../src/lib/archiveDb";
import { authCheck, clearContext, connectBridge, getState, getTaskCurrent, getHealth, setContext, submitAIReport, submitCommand, taskInterrupt } from "../src/lib/bridge";
import { formatStagedInputsForContext, stageDocumentPick, stageImagePick, type StagedInputRecord } from "../src/lib/externalIntake";
import { useApp } from "../src/state/AppContext";
import type { EventEnvelope } from "../src/types/contracts";

const REPORT_REASONS = [
  "Harmful or unsafe",
  "Sexual or inappropriate",
  "Harassment or abusive",
  "False or misleading",
  "Pretending to have done something it did not do",
  "Other"
] as const;

export default function MainHallScreen() {
  const router = useRouter();
  const { state, setState, events, pushEvent, tokenMissing, nightlyRitual, artPresentation } = useApp();
  const [prompt, setPrompt] = useState("");
  const [busy, setBusy] = useState(false);
  const [health, setHealth] = useState<string>("Unknown");
  const [stagedInputs, setStagedInputs] = useState<StagedInputRecord[]>([]);
  const [verifiedEmail, setVerifiedEmail] = useState("");
  const [reportTarget, setReportTarget] = useState<EventEnvelope | null>(null);
  const [reportReason, setReportReason] = useState<(typeof REPORT_REASONS)[number]>("Harmful or unsafe");
  const [reportNote, setReportNote] = useState("");

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
        authCheck().then((result) => setVerifiedEmail(result?.verified_email ?? "")).catch(() => null);
        const staged = await listStagedInputs();
        setStagedInputs(staged);
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
      "What should I know about this app?",
      "Go to the Archive",
      "Show me around"
    ],
    []
  );

  async function submit(nextPrompt: string) {
    if (!nextPrompt.trim()) return;
    setBusy(true);
    try {
      await submitCommand(nextPrompt.trim());
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

  async function refreshStagedInputs() {
    const staged = await listStagedInputs();
    setStagedInputs(staged);
    const context = formatStagedInputsForContext(staged);
    if (context) {
      await setContext(context);
    } else {
      await clearContext();
    }
  }

  async function attachFile() {
    try {
      const record = await stageDocumentPick();
      if (!record) return;
      await refreshStagedInputs();
    } catch (error: any) {
      Alert.alert("Attach file failed", error?.message ?? "Unknown error");
    }
  }

  async function attachImage() {
    try {
      const record = await stageImagePick();
      if (!record) return;
      await refreshStagedInputs();
    } catch (error: any) {
      Alert.alert("Attach image failed", error?.message ?? "Unknown error");
    }
  }

  async function clearStaged() {
    try {
      await clearStagedInputs();
      await refreshStagedInputs();
    } catch (error: any) {
      Alert.alert("Clear staged inputs failed", error?.message ?? "Unknown error");
    }
  }

  function openReport(event: EventEnvelope) {
    setReportTarget(event);
    setReportReason("Harmful or unsafe");
    setReportNote("");
  }

  async function submitReport() {
    if (!reportTarget) return;
    const outputText =
      reportTarget.payload?.report_text ||
      reportTarget.payload?.text ||
      reportTarget.payload?.message ||
      JSON.stringify(reportTarget.payload);

    try {
      await submitAIReport({
        task_id: reportTarget.payload?.task_id ?? state.active_task?.task_id,
        event_type: reportTarget.type,
        report_reason: reportReason,
        report_note: reportNote.trim(),
        output_text: String(outputText),
        prompt_text: state.active_task?.prompt ?? "",
        model_name: "xAI"
      });
      setReportTarget(null);
      setReportNote("");
      Alert.alert("Report sent", "Thank you. xMilo saved this AI-content report for review.");
    } catch (error: any) {
      Alert.alert("Report failed", error?.message ?? "Unknown error");
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

        {nightlyRitual ? (
          <View style={[styles.ritualCard, styles[`ritualCard_${nightlyRitual.status}` as const]]}>
            <Text style={styles.ritualEyebrow}>Nightly Ritual</Text>
            <Text style={styles.ritualTitle}>{nightlyRitual.title}</Text>
            <Text style={styles.ritualBody}>{nightlyRitual.description}</Text>
            <Text style={styles.ritualDetail}>Chamber: {nightlyRitual.chamber}</Text>
            <Text style={styles.ritualBody}>{nightlyRitual.visual}</Text>
            <Text style={styles.ritualCue}>Vocal cue: {nightlyRitual.vocalCue}</Text>
            <Text style={styles.ritualCue}>Physical cue: {nightlyRitual.physicalCue}</Text>
          </View>
        ) : null}

        {artPresentation.degradation_reason ? (
          <View style={styles.degradationCard}>
            <Text style={styles.degradationEyebrow}>Renderer Fallback</Text>
            <Text style={styles.degradationTitle}>Expo shell remains primary</Text>
            <Text style={styles.degradationBody}>{artPresentation.degradation_reason.message}</Text>
          </View>
        ) : null}

        <View style={styles.composerCard}>
          <Text style={styles.sectionTitle}>Send Milo a prompt</Text>
          <View style={styles.attachRow}>
            <Pressable style={styles.attachButton} onPress={attachFile}>
              <Text style={styles.attachButtonText}>Attach file</Text>
            </Pressable>
            <Pressable style={styles.attachButton} onPress={attachImage}>
              <Text style={styles.attachButtonText}>Attach image</Text>
            </Pressable>
            <Pressable style={[styles.attachButton, !stagedInputs.length && styles.disabled]} disabled={!stagedInputs.length} onPress={clearStaged}>
              <Text style={styles.attachButtonText}>Clear staged</Text>
            </Pressable>
          </View>
          {stagedInputs.length ? (
            <View style={styles.stagedCard}>
              <Text style={styles.stagedTitle}>Staged for this task</Text>
              {stagedInputs.map((input) => (
                <View key={input.id} style={styles.stagedItem}>
                  <Text style={styles.stagedItemLabel}>{input.label}</Text>
                  <Text style={styles.stagedItemMeta}>
                    {input.input_type}
                    {input.mime_type ? ` • ${input.mime_type}` : ""}
                  </Text>
                  {input.text_excerpt ? <Text style={styles.stagedExcerpt}>{input.text_excerpt.slice(0, 220)}</Text> : null}
                </View>
              ))}
            </View>
          ) : null}
          <TextInput
            multiline
            placeholder="Ask Milo something or send him somewhere..."
            placeholderTextColor="#94A3B8"
            style={styles.input}
            value={prompt}
            onChangeText={setPrompt}
            maxLength={8000}
          />
          <Text style={styles.charCount}>{prompt.length}/8000</Text>
          <View style={styles.buttonRow}>
            <Pressable style={[styles.primaryButton, busy && styles.disabled]} disabled={busy} onPress={() => submit(prompt)}>
              <Text style={styles.primaryButtonText}>{busy ? "Working..." : "Send prompt"}</Text>
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
            <MessageBubble key={`${event.timestamp}-${index}`} event={event} onReport={openReport} />
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
      <Modal
        animationType="fade"
        transparent
        visible={Boolean(reportTarget)}
        onRequestClose={() => setReportTarget(null)}
      >
        <View style={styles.modalScrim}>
          <View style={styles.modalCard}>
            <Text style={styles.modalTitle}>Report AI output</Text>
            <Text style={styles.modalBody}>
              This sends the exact xMilo output for review so safety and truthfulness can improve.
            </Text>
            {verifiedEmail ? <Text style={styles.modalHint}>Signed-in email: {verifiedEmail}</Text> : null}
            <View style={styles.reasonList}>
              {REPORT_REASONS.map((reason) => (
                <Pressable
                  key={reason}
                  style={[styles.reasonPill, reportReason === reason && styles.reasonPillSelected]}
                  onPress={() => setReportReason(reason)}
                >
                  <Text style={[styles.reasonPillText, reportReason === reason && styles.reasonPillTextSelected]}>
                    {reason}
                  </Text>
                </Pressable>
              ))}
            </View>
            <TextInput
              multiline
              placeholder="Optional note for review..."
              placeholderTextColor="#94A3B8"
              style={styles.reportInput}
              value={reportNote}
              onChangeText={setReportNote}
              maxLength={500}
            />
            <View style={styles.modalButtonRow}>
              <Pressable style={styles.secondaryButton} onPress={() => setReportTarget(null)}>
                <Text style={styles.secondaryButtonText}>Cancel</Text>
              </Pressable>
              <Pressable style={styles.primaryButton} onPress={submitReport}>
                <Text style={styles.primaryButtonText}>Send report</Text>
              </Pressable>
            </View>
          </View>
        </View>
      </Modal>
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
  ritualCard: { borderRadius: 18, padding: 16, borderWidth: 1 },
  ritualCard_deferred: { backgroundColor: "#1A1528", borderColor: "#57406E" },
  ritualCard_started: { backgroundColor: "#101A30", borderColor: "#3E5B8A" },
  ritualCard_completed: { backgroundColor: "#112218", borderColor: "#2E6A4F" },
  ritualEyebrow: { color: "#C4B5FD", fontSize: 12, fontWeight: "700", textTransform: "uppercase", letterSpacing: 1 },
  ritualTitle: { color: "#F8FAFC", fontSize: 18, fontWeight: "700", marginTop: 6 },
  ritualDetail: { color: "#D8C8FB", marginTop: 8, fontWeight: "700" },
  ritualBody: { color: "#CBD5E1", marginTop: 8, lineHeight: 20 },
  ritualCue: { color: "#CBD5E1", marginTop: 8, fontStyle: "italic", lineHeight: 20 },
  degradationCard: { borderRadius: 18, padding: 16, borderWidth: 1, backgroundColor: "#0F172A", borderColor: "#334155" },
  degradationEyebrow: { color: "#93C5FD", fontSize: 12, fontWeight: "700", textTransform: "uppercase", letterSpacing: 1 },
  degradationTitle: { color: "#F8FAFC", fontSize: 18, fontWeight: "700", marginTop: 6 },
  degradationBody: { color: "#CBD5E1", marginTop: 8, lineHeight: 20 },
  pill: { paddingVertical: 8, paddingHorizontal: 12, borderRadius: 999, backgroundColor: "#0F172A", borderWidth: 1, borderColor: "#23314F" },
  pillLabel: { color: "#94A3B8", fontSize: 11 },
  pillValue: { color: "#E2E8F0", fontWeight: "600", marginTop: 2 },
  composerCard: { backgroundColor: "#111827", borderRadius: 18, padding: 16, borderWidth: 1, borderColor: "#1F2937" },
  sectionHeader: { flexDirection: "row", justifyContent: "space-between", alignItems: "center" },
  sectionTitle: { color: "#F8FAFC", fontSize: 18, fontWeight: "700" },
  link: { color: "#93C5FD", fontWeight: "600" },
  attachRow: { flexDirection: "row", flexWrap: "wrap", gap: 8, marginTop: 12 },
  attachButton: { backgroundColor: "#1F2937", borderRadius: 12, paddingVertical: 10, paddingHorizontal: 12 },
  attachButtonText: { color: "#E2E8F0", fontWeight: "700" },
  stagedCard: { marginTop: 12, backgroundColor: "#0F172A", borderRadius: 14, borderWidth: 1, borderColor: "#1E293B", padding: 12, gap: 10 },
  stagedTitle: { color: "#C4B5FD", fontWeight: "700" },
  stagedItem: { gap: 4 },
  stagedItemLabel: { color: "#F8FAFC", fontWeight: "600" },
  stagedItemMeta: { color: "#94A3B8", fontSize: 12 },
  stagedExcerpt: { color: "#CBD5E1", fontSize: 12, lineHeight: 18 },
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
  modalScrim: {
    flex: 1,
    backgroundColor: "rgba(11,16,32,0.84)",
    alignItems: "center",
    justifyContent: "center",
    padding: 20
  },
  modalCard: {
    width: "100%",
    maxWidth: 420,
    backgroundColor: "#111827",
    borderRadius: 20,
    borderWidth: 1,
    borderColor: "#312E81",
    padding: 20
  },
  modalTitle: { color: "#F8FAFC", fontSize: 20, fontWeight: "700" },
  modalBody: { color: "#CBD5E1", lineHeight: 21, marginTop: 10 },
  modalHint: { color: "#A78BFA", marginTop: 10, fontSize: 12 },
  reasonList: { flexDirection: "row", flexWrap: "wrap", gap: 8, marginTop: 14 },
  reasonPill: {
    borderRadius: 999,
    borderWidth: 1,
    borderColor: "#334155",
    paddingHorizontal: 12,
    paddingVertical: 8,
    backgroundColor: "#0F172A"
  },
  reasonPillSelected: { backgroundColor: "#312E81", borderColor: "#4338CA" },
  reasonPillText: { color: "#CBD5E1", fontSize: 12, fontWeight: "600" },
  reasonPillTextSelected: { color: "#EDE9FE" },
  reportInput: {
    marginTop: 16,
    minHeight: 96,
    textAlignVertical: "top",
    backgroundColor: "#0F172A",
    borderWidth: 1,
    borderColor: "#334155",
    borderRadius: 14,
    paddingHorizontal: 14,
    paddingVertical: 12,
    color: "#F8FAFC"
  },
  modalButtonRow: { flexDirection: "row", gap: 12, justifyContent: "flex-end", marginTop: 16 },
  footerButton: { flex: 1, backgroundColor: "#111827", borderRadius: 14, paddingVertical: 14, alignItems: "center", borderWidth: 1, borderColor: "#1F2937" },
  footerButtonText: { color: "#E5E7EB", fontWeight: "700" },
  lairButton: { backgroundColor: "#1E1535", borderColor: "#4B3B7A" },
  lairButtonText: { color: "#C8B8FF", fontWeight: "700", fontSize: 16 },
});
