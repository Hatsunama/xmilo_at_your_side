import { useFocusEffect, useRouter } from "expo-router";
import { useCallback, useEffect, useMemo, useRef, useState } from "react";
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
import { StagedContentChip } from "../src/components/StagedContentChip";
import { StarterChips } from "../src/components/StarterChips";
import { clearStagedInputs, listStagedInputs } from "../src/lib/archiveDb";
import { authCheck, clearContext, getHealth, getReady, setContext, submitAIReport, submitCommand, taskInterrupt, type SidecarReadyStatus } from "../src/lib/bridge";
import { formatStagedInputsForContext, stageDocumentPick, stageImagePick, type StagedInputRecord } from "../src/lib/externalIntake";
import { useDailyConversation } from "../src/hooks/useDailyConversation";
import {
  firstTaskAccessMessage,
  firstTaskErrorMessage,
  formatActiveTaskStatus,
  formatRuntimeEventForUser,
  latestRelevantTaskEvent,
  safeEventText,
  safeLogSubmitResponse,
  submitResponseMessage,
  taskIdFromEvent,
  taskIdFromSubmitResponse,
} from "../src/lib/runtimeEvents";
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
const PASTE_THRESHOLD = 800;
const CONVERSATION_BOTTOM_THRESHOLD = 48;

export default function MainHallScreen() {
  const router = useRouter();
  const { state, events, pushEvent, nightlyRitual, artPresentation, refreshRuntimeState, taskProofResetVersion } = useApp();
  const [prompt, setPrompt] = useState("");
  const [busy, setBusy] = useState(false);
  const [health, setHealth] = useState<string>("Unknown");
  const [readyStatus, setReadyStatus] = useState<SidecarReadyStatus | null>(null);
  const [sendStatus, setSendStatus] = useState("");
  const [sendError, setSendError] = useState("");
  const [currentSubmitTaskId, setCurrentSubmitTaskId] = useState("");
  const [stagedInputs, setStagedInputs] = useState<StagedInputRecord[]>([]);
  const [pastedContext, setPastedContext] = useState<{ preview: string; charCount: number } | null>(null);
  const [verifiedEmail, setVerifiedEmail] = useState("");
  const [reportTarget, setReportTarget] = useState<EventEnvelope | null>(null);
  const [reportReason, setReportReason] = useState<(typeof REPORT_REASONS)[number]>("Harmful or unsafe");
  const [reportNote, setReportNote] = useState("");
  const promptLenRef = useRef(0);
  const conversationScrollRef = useRef<ScrollView>(null);
  const pendingConversationScrollRef = useRef(false);
  const lastConversationMessageCountRef = useRef(0);
  const lastConversationLastMessageIdRef = useRef<string | null>(null);
  const awaitingMainHallResponseScrollRef = useRef<{
    taskId?: string | null;
    attemptId?: string | null;
    userMessageId: string;
  } | null>(null);
  const conversationScrollTimerRef = useRef<ReturnType<typeof setTimeout> | null>(null);
  const dailyConversation = useDailyConversation(events, currentSubmitTaskId || state.active_task?.task_id);

  const refreshReadyStatus = useCallback(async () => {
    try {
      const nextReady = await getReady();
      setReadyStatus(nextReady);
      return nextReady;
    } catch {
      setReadyStatus(null);
      return null;
    }
  }, []);

  useEffect(() => {
    let disposed = false;

    async function boot() {
      try {
        const healthResp = await getHealth();
        if (disposed) return;
        setHealth(healthResp.service ?? "xmilo-sidecar");
        await Promise.all([refreshReadyStatus(), refreshRuntimeState()]);
        authCheck().then((result) => setVerifiedEmail(result?.verified_email ?? "")).catch(() => null);
        const staged = await listStagedInputs();
        if (disposed) return;
        setStagedInputs(staged);
      } catch (error: any) {
        if (disposed) return;
        setHealth(error?.message ?? "Unavailable");
      }

    }

    void boot();
    const interval = setInterval(() => void refreshReadyStatus(), 3000);
    return () => {
      disposed = true;
      clearInterval(interval);
    };
  }, [refreshReadyStatus, refreshRuntimeState]);

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
    const trimmedPrompt = nextPrompt.trim();
    if (!trimmedPrompt) return;
    setBusy(true);
    setSendError("");
    setSendStatus("Checking current task state...");
    await refreshRuntimeState();
    const latestReady = await refreshReadyStatus();
    const accessMessage = firstTaskAccessMessage(latestReady ?? readyStatus);
    if (accessMessage) {
      setSendStatus("");
      setSendError(accessMessage);
      setBusy(false);
      return;
    }
    setCurrentSubmitTaskId("");
    setSendStatus("Sending...");
    const userMessageId = await dailyConversation.appendUserPrompt(trimmedPrompt);
    awaitingMainHallResponseScrollRef.current = { userMessageId };
    try {
      const response = await submitCommand(trimmedPrompt);
      safeLogSubmitResponse(response);
      const taskId = taskIdFromSubmitResponse(response);
      const attemptId = response.attempt_id || response.immediate_state?.attempt_id || null;
      awaitingMainHallResponseScrollRef.current = {
        userMessageId,
        taskId,
        attemptId,
      };
      if (taskId) {
        setCurrentSubmitTaskId(taskId);
        await dailyConversation.updateMessageTaskIdentity(userMessageId, taskId, attemptId);
      }
      setSendStatus(submitResponseMessage(response));
      setPrompt("");
      promptLenRef.current = 0;
      await refreshRuntimeState();
    } catch (error: any) {
      const message = firstTaskErrorMessage(error?.message ?? "Failed to start task");
      setSendStatus("");
      setSendError(message);
      pushEvent({
        type: "ui_local.error",
        timestamp: new Date().toISOString(),
        source: "ui_local",
        truth_scope: "ui_submit",
        payload: { message, recoverable: true, source: "ui_local", truth_scope: "ui_submit" }
      });
      await refreshRuntimeState();
    } finally {
      setBusy(false);
    }
  }

  async function interrupt() {
    setBusy(true);
    try {
      await taskInterrupt();
      await refreshRuntimeState();
    } finally {
      setBusy(false);
    }
  }

  useEffect(() => {
    const event = events.length > 0 ? events[events.length - 1] : null;
    if (!event) return;
    const eventTaskId = taskIdFromEvent(event);
    if (event.type === "task.accepted" && eventTaskId && (currentSubmitTaskId || state.active_task?.task_id === eventTaskId)) {
      setCurrentSubmitTaskId(eventTaskId);
      setSendError("");
      setSendStatus(formatRuntimeEventForUser(event));
      return;
    }
    if (eventTaskId && currentSubmitTaskId && eventTaskId === currentSubmitTaskId) {
      setSendStatus(formatRuntimeEventForUser(event));
      if (event.type !== "runtime.error") {
        setSendError("");
      }
    }
  }, [currentSubmitTaskId, events, state.active_task?.task_id]);

  useEffect(() => {
    setSendError("");
    setSendStatus("");
    setCurrentSubmitTaskId("");
  }, [taskProofResetVersion]);

  const scheduleConversationScrollToEnd = useCallback(() => {
    if (conversationScrollTimerRef.current) {
      clearTimeout(conversationScrollTimerRef.current);
      conversationScrollTimerRef.current = null;
    }
    requestAnimationFrame(() => {
      conversationScrollTimerRef.current = setTimeout(() => {
        pendingConversationScrollRef.current = false;
        conversationScrollRef.current?.scrollToEnd({ animated: true });
        conversationScrollTimerRef.current = null;
      }, 0);
    });
  }, []);

  useEffect(() => {
    const messageCount = dailyConversation.messages.length;
    const messageCountIncreased = messageCount > lastConversationMessageCountRef.current;
    const newestMessage = messageCount > 0 ? dailyConversation.messages[messageCount - 1] : null;
    const newestMessageId = newestMessage?.id ?? null;
    const newestMessageChanged = Boolean(newestMessageId && newestMessageId !== lastConversationLastMessageIdRef.current);
    lastConversationMessageCountRef.current = messageCount;
    lastConversationLastMessageIdRef.current = newestMessageId;

    if (dailyConversation.forceNextChatScrollRef.current) {
      pendingConversationScrollRef.current = true;
      dailyConversation.forceNextChatScrollRef.current = false;
    }

    const awaitedResponse = awaitingMainHallResponseScrollRef.current;
    if (awaitedResponse && newestMessageChanged && newestMessage && newestMessage.role !== "user") {
      const taskMatches = !awaitedResponse.taskId || !newestMessage.task_id || awaitedResponse.taskId === newestMessage.task_id;
      if (taskMatches && newestMessage.id !== awaitedResponse.userMessageId) {
        pendingConversationScrollRef.current = true;
        awaitingMainHallResponseScrollRef.current = null;
        scheduleConversationScrollToEnd();
        return;
      }
    }

    if (messageCountIncreased && (pendingConversationScrollRef.current || dailyConversation.chatNearBottomRef.current)) {
      pendingConversationScrollRef.current = true;
      scheduleConversationScrollToEnd();
    }
  }, [dailyConversation.chatNearBottomRef, dailyConversation.forceNextChatScrollRef, dailyConversation.messages.length, scheduleConversationScrollToEnd]);

  useEffect(
    () => () => {
      if (conversationScrollTimerRef.current) {
        clearTimeout(conversationScrollTimerRef.current);
      }
    },
    []
  );

  useFocusEffect(
    useCallback(() => {
      void dailyConversation.loadDailyConversation();
    }, [dailyConversation.loadDailyConversation])
  );

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

  async function handlePromptChange(text: string) {
    const delta = text.length - promptLenRef.current;
    promptLenRef.current = text.length;

    if (delta > PASTE_THRESHOLD && text.length > PASTE_THRESHOLD) {
      try {
        await setContext({
          content: text,
          source: "large_paste",
          provenance: "large_paste",
          label: "Large paste",
          mime_type: "text/plain",
          created_at: new Date().toISOString()
        });
        const preview = text.slice(0, 80).replace(/\n/g, " ");
        setPastedContext({ preview, charCount: text.length });
        setPrompt("");
        setSendError("");
        promptLenRef.current = 0;
      } catch (error: any) {
        setSendError(error?.message ?? "Could not stage pasted context.");
      }
      return;
    }

    setPrompt(text);
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
      setPastedContext(null);
      await clearStagedInputs();
      await refreshStagedInputs();
    } catch (error: any) {
      Alert.alert("Clear staged inputs failed", error?.message ?? "Unknown error");
    }
  }

  async function dismissPastedContext() {
    setPastedContext(null);
    try {
      if (stagedInputs.length) {
        await refreshStagedInputs();
      } else {
        await clearContext();
      }
    } catch (_) {}
  }

  function openReport(event: EventEnvelope) {
    setReportTarget(event);
    setReportReason("Harmful or unsafe");
    setReportNote("");
  }

  async function submitReport() {
    if (!reportTarget) return;
    const outputText = safeEventText(reportTarget);

    try {
      await submitAIReport({
        task_id: reportTarget.payload?.task_id ?? state.active_task?.task_id,
        event_type: reportTarget.type,
        report_reason: reportReason,
        report_note: reportNote.trim(),
        output_text: String(outputText),
        prompt_text: state.active_task?.prompt ?? "",
        model_name: readyStatus?.byok_provider ? `local_byok:${readyStatus.byok_provider}` : "local_byok"
      });
      setReportTarget(null);
      setReportNote("");
      Alert.alert("Report sent", "Thank you. xMilo saved this AI-content report for review.");
    } catch (error: any) {
      Alert.alert("Report failed", error?.message ?? "Unknown error");
    }
  }

  const latestCurrentTaskEvent = useMemo(
    () => (currentSubmitTaskId ? latestRelevantTaskEvent(events, currentSubmitTaskId) : null),
    [currentSubmitTaskId, events]
  );
  const lifecycleStatus = latestCurrentTaskEvent
    ? formatRuntimeEventForUser(latestCurrentTaskEvent)
    : formatActiveTaskStatus(state.active_task);
  const sendBlockedMessage = firstTaskAccessMessage(readyStatus);
  const sendDisabled = busy || Boolean(sendBlockedMessage);
  const visibleSendError = sendError || (readyStatus && sendBlockedMessage ? sendBlockedMessage : "");
  const visibleSendStatus = sendStatus || (!readyStatus ? sendBlockedMessage : lifecycleStatus);
  const firstTaskValue =
    readyStatus?.first_task_eligible && readyStatus?.local_llm_turn_allowed
      ? "eligible"
      : readyStatus
      ? "not ready"
      : "checking";

  return (
    <View style={styles.screen}>
      <ScrollView contentContainerStyle={styles.content}>
        <View style={styles.heroCard}>
          <Text style={styles.kicker}>Main Hall</Text>
          <Text style={styles.title}>xMilo Main Hall</Text>
          <Text style={styles.subtitle}>
            App-owned sidecar runtime, local BYOK access, and global task events are active for Phase 9 proof.
          </Text>
          <View style={styles.statsRow}>
            <InfoPill label="Bridge" value={health} />
            <InfoPill label="State" value={state.milo_state || "idle"} />
            <InfoPill label="Room" value={state.current_room_id || "main_hall"} />
            <InfoPill label="First task" value={firstTaskValue} />
          </View>
        </View>

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

        <View style={styles.conversationCard}>
          <View style={styles.sectionHeader}>
            <Text style={styles.sectionTitle}>Daily conversation</Text>
            <Text style={styles.conversationMeta}>
              {dailyConversation.messages.length ? `${dailyConversation.messages.length} today` : "Today"}
            </Text>
          </View>
          {dailyConversation.messages.length === 0 ? (
            <View style={styles.conversationEmpty}>
              <Text style={styles.conversationEmptyTitle}>No conversation yet today</Text>
              <Text style={styles.conversationEmptyBody}>Send Milo a prompt here or in the Lair to start the shared daily thread.</Text>
            </View>
          ) : (
            <ScrollView
              ref={conversationScrollRef}
              style={styles.conversationScroll}
              contentContainerStyle={styles.conversationContent}
              nestedScrollEnabled
              showsVerticalScrollIndicator
              scrollEventThrottle={80}
              onScroll={({ nativeEvent }) => {
                const distanceFromBottom =
                  nativeEvent.contentSize.height - nativeEvent.layoutMeasurement.height - nativeEvent.contentOffset.y;
                dailyConversation.chatNearBottomRef.current = distanceFromBottom < CONVERSATION_BOTTOM_THRESHOLD;
              }}
              onContentSizeChange={() => {
                if (dailyConversation.forceNextChatScrollRef.current) {
                  pendingConversationScrollRef.current = true;
                  dailyConversation.forceNextChatScrollRef.current = false;
                }
                if (pendingConversationScrollRef.current || dailyConversation.chatNearBottomRef.current) {
                  scheduleConversationScrollToEnd();
                }
              }}
            >
              {dailyConversation.messages.map((message) => (
                <View
                  key={message.id}
                  style={[
                    styles.conversationBubble,
                    message.role === "user" ? styles.conversationBubbleUser : null,
                    message.role === "system" ? styles.conversationBubbleSystem : null
                  ]}
                >
                  <Text style={styles.conversationRole}>
                    {message.role === "user" ? "You" : message.role === "system" ? "System" : "Milo"}
                  </Text>
                  <Text style={styles.conversationText}>{message.text}</Text>
                </View>
              ))}
            </ScrollView>
          )}
        </View>

        <View style={styles.composerCard}>
          <Text style={styles.sectionTitle}>Send Milo a prompt</Text>
          {visibleSendStatus || visibleSendError ? (
            <View style={[styles.sendStatusBox, visibleSendError ? styles.sendStatusBoxError : null]}>
              <Text style={[styles.sendStatusText, visibleSendError ? styles.sendStatusTextError : null]}>
                {visibleSendError || visibleSendStatus}
              </Text>
            </View>
          ) : null}
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
          {pastedContext ? (
            <StagedContentChip
              label="Large paste"
              meta={`${pastedContext.charCount.toLocaleString()} chars`}
              preview={pastedContext.preview}
              onRemove={dismissPastedContext}
            />
          ) : null}
          {stagedInputs.length ? (
            <View style={styles.stagedChipList}>
              {stagedInputs.map((input) => (
                <StagedContentChip
                  key={input.id}
                  label={input.label}
                  meta={`${input.input_type}${input.mime_type ? ` · ${input.mime_type}` : ""}`}
                  preview={input.text_excerpt?.replace(/\s+/g, " ").slice(0, 80)}
                  onRemove={clearStaged}
                />
              ))}
            </View>
          ) : null}
          <TextInput
            multiline
            placeholder={pastedContext || stagedInputs.length ? "Now give Milo a prompt for this content..." : "Ask Milo something or send him somewhere..."}
            placeholderTextColor="#94A3B8"
            style={styles.input}
            value={prompt}
            onChangeText={handlePromptChange}
            maxLength={8000}
          />
          <Text style={styles.charCount}>{prompt.length}/8000</Text>
          <View style={styles.buttonRow}>
            <Pressable style={[styles.primaryButton, sendDisabled && styles.disabled]} disabled={sendDisabled} onPress={() => submit(prompt)}>
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
          <View style={styles.recentEventsBox}>
            <ScrollView
              style={styles.recentEventsScroll}
              contentContainerStyle={styles.recentEventsContent}
              nestedScrollEnabled
              showsVerticalScrollIndicator
            >
              {events.slice().reverse().map((event, index) => (
                <MessageBubble key={`${event.timestamp}-${index}`} event={event} onReport={openReport} />
              ))}
            </ScrollView>
          </View>
        )}

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
              This sends safe xMilo output details for review so safety and truthfulness can improve.
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
  conversationCard: { backgroundColor: "#111827", borderRadius: 18, padding: 16, borderWidth: 1, borderColor: "#1F2937" },
  conversationMeta: { color: "#94A3B8", fontSize: 12, fontWeight: "700" },
  conversationEmpty: { marginTop: 12, borderRadius: 14, borderWidth: 1, borderColor: "#1E293B", backgroundColor: "#0F172A", padding: 14 },
  conversationEmptyTitle: { color: "#F8FAFC", fontWeight: "700" },
  conversationEmptyBody: { color: "#CBD5E1", marginTop: 6, lineHeight: 19 },
  conversationScroll: { maxHeight: 400, marginTop: 12 },
  conversationContent: { gap: 10, paddingBottom: 4 },
  conversationBubble: {
    alignSelf: "flex-start",
    maxWidth: "88%",
    borderRadius: 16,
    borderWidth: 1,
    borderColor: "#263653",
    backgroundColor: "#0F172A",
    paddingHorizontal: 12,
    paddingVertical: 10
  },
  conversationBubbleUser: { alignSelf: "flex-end", backgroundColor: "#1D2B4F", borderColor: "#355084" },
  conversationBubbleSystem: { backgroundColor: "#251A33", borderColor: "#57406E" },
  conversationRole: { color: "#A5B4FC", fontSize: 11, fontWeight: "800", textTransform: "uppercase", letterSpacing: 0.6, marginBottom: 4 },
  conversationText: { color: "#E5E7EB", lineHeight: 20 },
  composerCard: { backgroundColor: "#111827", borderRadius: 18, padding: 16, borderWidth: 1, borderColor: "#1F2937" },
  sectionHeader: { flexDirection: "row", justifyContent: "space-between", alignItems: "center" },
  sectionTitle: { color: "#F8FAFC", fontSize: 18, fontWeight: "700" },
  sendStatusBox: { marginTop: 12, borderRadius: 12, borderWidth: 1, borderColor: "#1D4ED8", backgroundColor: "#0F1E3A", padding: 12 },
  sendStatusBoxError: { borderColor: "#7F1D1D", backgroundColor: "#2A1218" },
  sendStatusText: { color: "#BFDBFE", lineHeight: 18 },
  sendStatusTextError: { color: "#FCA5A5" },
  link: { color: "#93C5FD", fontWeight: "600" },
  attachRow: { flexDirection: "row", flexWrap: "wrap", gap: 8, marginTop: 12 },
  attachButton: { backgroundColor: "#1F2937", borderRadius: 12, paddingVertical: 10, paddingHorizontal: 12 },
  attachButtonText: { color: "#E2E8F0", fontWeight: "700" },
  stagedChipList: { marginTop: 12, gap: 8 },
  input: {
    minHeight: 76,
    maxHeight: 144,
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
  recentEventsBox: {
    maxHeight: 432,
    borderRadius: 20,
    borderWidth: 1,
    borderColor: "#1F2937",
    backgroundColor: "#0F172A",
    overflow: "hidden",
  },
  recentEventsScroll: {
    maxHeight: 432,
  },
  recentEventsContent: {
    gap: 12,
    padding: 12,
  },
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
