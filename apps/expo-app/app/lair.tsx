import { useFocusEffect, useRouter } from "expo-router";
import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import {
  BackHandler,
  Keyboard,
  KeyboardAvoidingView,
  PanResponder,
  Platform,
  Pressable,
  ScrollView,
  StyleSheet,
  Text,
  TextInput,
  useWindowDimensions,
  View,
} from "react-native";

import { CastleMap3DView, CastleMapLegendPanel } from "../src/castleMap3d/CastleMap3DView";
import { StagedContentChip } from "../src/components/StagedContentChip";
import { useApp } from "../src/state/AppContext";
import { getAppSetting, initArchiveDb, setAppSetting } from "../src/lib/archiveDb";
import { getReady, submitCommand, taskInterrupt, setContext, clearContext, type SidecarReadyStatus } from "../src/lib/bridge";
import type { EventEnvelope } from "../src/types/contracts";
import { useDailyConversation } from "../src/hooks/useDailyConversation";
import {
  formatActiveTaskStatus,
  firstTaskAccessMessage,
  firstTaskErrorMessage,
  formatRuntimeEventForUser,
  latestRelevantTaskEvent,
  safeLogSubmitResponse,
  submitResponseMessage,
  taskIdFromEvent,
  taskIdFromSubmitResponse,
} from "../src/lib/runtimeEvents";

const ANDROID_KEYBOARD_CLEARANCE = 56;
const CHAT_SWIPE_THRESHOLD = 28;
const CHAT_BOTTOM_THRESHOLD = 48;
const LAIR_FIRST_RUN_OVERLAY_KEY = "lair_first_run_overlay_seen_phase9";

export default function WizardLairScreen() {
  const router = useRouter();
  const { height: windowHeight } = useWindowDimensions();
  const { state, events, nightlyRitual, pushEvent, refreshRuntimeState, taskProofResetVersion } = useApp();
  const [prompt, setPrompt] = useState("");
  const [busy, setBusy] = useState(false);
  const [inputVisible, setInputVisible] = useState(false);
  const [chatExpanded, setChatExpanded] = useState(false);
  const [sendStatus, setSendStatus] = useState("");
  const [sendError, setSendError] = useState("");
  const [readyStatus, setReadyStatus] = useState<SidecarReadyStatus | null>(null);
  const [currentSubmitTaskId, setCurrentSubmitTaskId] = useState("");
  const [keyboardOffset, setKeyboardOffset] = useState(0);
  const [pastedContext, setPastedContext] = useState<{ preview: string; charCount: number } | null>(null);
  const [firstRunOverlayVisible, setFirstRunOverlayVisible] = useState(false);
  const [firstRunOverlayReady, setFirstRunOverlayReady] = useState(false);
  const promptLenRef = useRef(0);
  const inputRef = useRef<TextInput>(null);
  const chatScrollRef = useRef<ScrollView>(null);
  const expandedChatMaxHeight = Math.round(windowHeight * 0.75);
  const dailyConversation = useDailyConversation(events, currentSubmitTaskId || state.active_task?.task_id);
  const chatMessages = dailyConversation.messages;
  const loadDailyConversation = dailyConversation.loadDailyConversation;
  const forceNextChatScrollRef = dailyConversation.forceNextChatScrollRef;

  const PASTE_THRESHOLD = 800;

  const chatPanResponder = useMemo(
    () =>
      PanResponder.create({
        onMoveShouldSetPanResponder: (_, gestureState) => Math.abs(gestureState.dy) > CHAT_SWIPE_THRESHOLD && Math.abs(gestureState.dy) > Math.abs(gestureState.dx),
        onPanResponderRelease: (_, gestureState) => {
          if (gestureState.dy < -CHAT_SWIPE_THRESHOLD) {
            setChatExpanded(true);
          } else if (gestureState.dy > CHAT_SWIPE_THRESHOLD) {
            setChatExpanded(false);
            Keyboard.dismiss();
          }
        },
      }),
    []
  );

  function openComposer() {
    setInputVisible(true);
    requestAnimationFrame(() => inputRef.current?.focus());
  }

  const leaveLair = useCallback(() => {
    router.replace("/");
  }, [router]);

  async function dismissFirstRunOverlay() {
    setFirstRunOverlayVisible(false);
    try {
      await initArchiveDb();
      await setAppSetting(LAIR_FIRST_RUN_OVERLAY_KEY, "true");
    } catch {
    }
  }

  useEffect(() => {
    const subscription = BackHandler.addEventListener("hardwareBackPress", () => {
      leaveLair();
      return true;
    });
    return () => subscription.remove();
  }, [leaveLair]);

  useEffect(() => {
    let disposed = false;
    initArchiveDb()
      .then(() => getAppSetting(LAIR_FIRST_RUN_OVERLAY_KEY))
      .then((value) => {
        if (!disposed) {
          setFirstRunOverlayVisible(value !== "true");
          setFirstRunOverlayReady(true);
        }
      })
      .catch(() => {
        if (!disposed) {
          setFirstRunOverlayVisible(true);
          setFirstRunOverlayReady(true);
        }
      });
    return () => {
      disposed = true;
    };
  }, []);

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

    async function refreshLairRuntimeState() {
      if (disposed) return;
      await Promise.all([refreshReadyStatus(), refreshRuntimeState()]);
    }

    void refreshLairRuntimeState();
    const interval = setInterval(() => void refreshReadyStatus(), 3000);
    return () => {
      disposed = true;
      clearInterval(interval);
    };
  }, [refreshReadyStatus, refreshRuntimeState]);

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

  useEffect(() => {
    setSendError("");
    setSendStatus("");
    setCurrentSubmitTaskId("");
  }, [taskProofResetVersion]);

  function hidePrompt() {
    Keyboard.dismiss();
    setInputVisible(false);
  }

  async function handleTextChange(text: string) {
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

  async function dismissPaste() {
    setPastedContext(null);
    try { await clearContext(); } catch (_) {}
  }

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
      setInputVisible(true);
      setSendStatus("");
      setSendError(accessMessage);
      setBusy(false);
      return;
    }
    setSendStatus("Sending...");
    setCurrentSubmitTaskId("");
    const userMessageId = await dailyConversation.appendUserPrompt(trimmedPrompt);
    try {
      const response = await submitCommand(trimmedPrompt);
      safeLogSubmitResponse(response);
      const taskId = taskIdFromSubmitResponse(response);
      if (taskId) {
        setCurrentSubmitTaskId(taskId);
        await dailyConversation.updateMessageTaskIdentity(userMessageId, taskId, response.attempt_id || response.immediate_state?.attempt_id);
      }
      setPrompt("");
      promptLenRef.current = 0;
      setInputVisible(false);
      setSendStatus(submitResponseMessage(response));
      await refreshRuntimeState();
    } catch (error: any) {
      const message = firstTaskErrorMessage(error?.message ?? "Failed to send prompt");
      setInputVisible(true);
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
    if (!chatExpanded || !forceNextChatScrollRef.current) return;
    forceNextChatScrollRef.current = false;
    requestAnimationFrame(() => chatScrollRef.current?.scrollToEnd({ animated: true }));
  }, [chatExpanded, chatMessages.length, forceNextChatScrollRef]);

  useFocusEffect(
    useCallback(() => {
      void loadDailyConversation();
    }, [loadDailyConversation])
  );

  const roomLabel = ROOM_LABELS[state.current_room_id ?? "main_hall"] ?? "Main Hall";
  const miloStateLabel = STATE_LABELS[state.milo_state ?? "idle"] ?? "Idle";
  const latestCurrentTaskEvent = useMemo(
    () => (currentSubmitTaskId ? latestRelevantTaskEvent(events, currentSubmitTaskId) : null),
    [currentSubmitTaskId, events]
  );
  const lifecycleStatus = latestCurrentTaskEvent
    ? formatEvent(latestCurrentTaskEvent)
    : formatActiveTaskStatus(state.active_task);
  const sendBlockedMessage = firstTaskAccessMessage(readyStatus);
  const sendDisabled = busy || Boolean(sendBlockedMessage);
  const visibleSendError = sendError || (inputVisible ? sendBlockedMessage ?? "" : "");
  const visibleSendStatus = sendStatus || lifecycleStatus;

  return (
    <KeyboardAvoidingView
      behavior={Platform.OS === "ios" ? "padding" : undefined}
      style={styles.screen}
    >
      <View style={styles.mapSurface}>
        <CastleMap3DView variant="lair" />
      </View>

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

      {firstRunOverlayVisible ? (
        <View style={styles.firstRunScrim} accessibilityViewIsModal>
          <View style={styles.firstRunCard}>
            <Text style={styles.firstRunKicker}>Welcome to the Lair</Text>
            <Text style={styles.firstRunTitle}>This is Milo's embodied room.</Text>
            <Text style={styles.firstRunBody}>
              The map stays alive behind setup and chat. Use the prompt drawer below when you want Milo to act, and return to Main Hall only when you want the denser status view.
            </Text>
            <Pressable style={styles.firstRunButton} onPress={dismissFirstRunOverlay} accessibilityRole="button">
              <Text style={styles.firstRunButtonText}>Enter the Lair</Text>
            </Pressable>
          </View>
        </View>
      ) : null}

      {nightlyRitual ? (
        <View style={[styles.ritualBanner, styles[`ritualBanner_${nightlyRitual.status}` as const]]} pointerEvents="none">
          <Text style={styles.ritualBannerEyebrow}>Nightly Ritual</Text>
          <Text style={styles.ritualBannerTitle}>{nightlyRitual.title}</Text>
          <Text style={styles.ritualBannerBody}>{nightlyRitual.visual}</Text>
          <Text style={styles.ritualBannerCue}>{nightlyRitual.physicalCue}</Text>
        </View>
      ) : null}

      <View
        style={[
          styles.bottomControls,
          Platform.OS === "android" ? { bottom: keyboardOffset } : null,
        ]}
      >
        <CastleMapLegendPanel suppressLegendOverlay={!firstRunOverlayReady || firstRunOverlayVisible} />
        <View
          style={[
            styles.chatPanel,
            chatExpanded ? [styles.chatPanelExpanded, { maxHeight: expandedChatMaxHeight }] : styles.chatPanelCollapsed,
          ]}
        >
          <View style={styles.chatHeader} {...chatPanResponder.panHandlers}>
            <Pressable style={styles.controlButton} onPress={leaveLair}>
              <Text style={styles.controlButtonText}>← Back</Text>
            </Pressable>
            <Pressable style={styles.chatTitleWrap} onPress={() => setChatExpanded(true)}>
              <Text style={styles.chatTitle}>Daily conversation</Text>
              <Text style={styles.chatSubtitle}>{chatMessages.length ? `${chatMessages.length} messages today` : "Ready when you are"}</Text>
            </Pressable>
            {chatExpanded ? (
              <Pressable
                style={styles.collapseButton}
                onPress={() => {
                  setChatExpanded(false);
                  Keyboard.dismiss();
                }}
                hitSlop={8}
              >
                <Text style={styles.collapseButtonText}>⌄</Text>
              </Pressable>
            ) : (
              <Pressable style={styles.collapseButton} onPress={() => setChatExpanded(true)} hitSlop={8}>
                <Text style={styles.collapseButtonText}>⌃</Text>
              </Pressable>
            )}
          </View>

          {chatExpanded ? (
            <ScrollView
              ref={chatScrollRef}
              style={styles.chatScroll}
              contentContainerStyle={styles.chatScrollContent}
              keyboardShouldPersistTaps="handled"
              onScroll={({ nativeEvent }) => {
                const distanceFromBottom = nativeEvent.contentSize.height - nativeEvent.layoutMeasurement.height - nativeEvent.contentOffset.y;
                dailyConversation.chatNearBottomRef.current = distanceFromBottom < CHAT_BOTTOM_THRESHOLD;
              }}
              scrollEventThrottle={80}
              onContentSizeChange={() => {
                if (dailyConversation.forceNextChatScrollRef.current || dailyConversation.chatNearBottomRef.current) {
                  dailyConversation.forceNextChatScrollRef.current = false;
                  chatScrollRef.current?.scrollToEnd({ animated: true });
                }
              }}
            >
              {chatMessages.length === 0 ? (
                <View style={styles.chatEmpty}>
                  <Text style={styles.chatEmptyTitle}>No chat yet today</Text>
                  <Text style={styles.chatEmptyBody}>Send Milo a prompt when you want to start the conversation.</Text>
                </View>
              ) : (
                chatMessages.map((message) => (
                  <View
                    key={message.id}
                    style={[
                      styles.chatBubble,
                      message.role === "user" ? styles.chatBubbleUser : null,
                      message.role === "system" ? styles.chatBubbleSystem : null,
                    ]}
                  >
                    <Text style={styles.chatRole}>{message.role === "user" ? "You" : message.role === "milo" ? "Milo" : "System"}</Text>
                    <Text style={styles.chatText}>{message.text}</Text>
                  </View>
                ))
              )}
            </ScrollView>
          ) : (
            <Pressable style={styles.chatPreview} onPress={() => setChatExpanded(true)} {...chatPanResponder.panHandlers}>
              <Text style={styles.chatPreviewText} numberOfLines={2}>
                {chatMessages.length ? chatMessages[chatMessages.length - 1].text : "Chat is minimized. Swipe up to open today’s conversation."}
              </Text>
            </Pressable>
          )}

          {visibleSendStatus || visibleSendError ? (
            <View style={[
              styles.sendStatusBox,
              visibleSendError ? styles.sendStatusBoxError : null,
            ]}>
              <Text style={[styles.sendStatusText, visibleSendError ? styles.sendStatusTextError : null]} numberOfLines={2}>
                {visibleSendError ? visibleSendError : visibleSendStatus}
              </Text>
            </View>
          ) : null}

          <View style={styles.controlRow}>
            <Pressable
              style={[styles.primaryButton, busy && styles.disabled]}
              disabled={busy}
              onPress={() => {
                if (inputVisible) {
                  hidePrompt();
                } else {
                  openComposer();
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
            <View style={styles.inputRow}>
              {pastedContext && (
                <StagedContentChip
                  label="Large paste"
                  meta={`${pastedContext.charCount.toLocaleString()} chars`}
                  preview={pastedContext.preview}
                  onRemove={dismissPaste}
                />
              )}
              <View style={styles.pasteInputRow}>
                <TextInput
                  ref={inputRef}
                  multiline
                  placeholder={pastedContext ? "Now give Milo a prompt for this content…" : "Give Milo a prompt or send him somewhere..."}
                  placeholderTextColor="#6B5A8A"
                  style={styles.input}
                  value={prompt}
                  onChangeText={handleTextChange}
                  maxLength={8000}
                />
                <Pressable
                  style={[styles.sendButton, sendDisabled && styles.disabled]}
                  disabled={sendDisabled}
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
      </View>
    </KeyboardAvoidingView>
  );
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

function formatEvent(event: EventEnvelope): string {
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
    case "task.accepted":
    case "task.completed":
    case "task.stuck":
    case "task.stale_active_recovered":
    case "runtime.error":
    case "runtime.capability_state":
    case "ui_local.error":
    case "local_provider.diagnostic":
      return formatRuntimeEventForUser(event);
    case "task.entitlement_lost":
      return "Local API key access is not ready yet. Choose an access path in setup.";
    case "milo.thought":
      return `💭 ${event.payload?.text ?? ""}`;
    case "report.ready":
      return `📜 Report ready`;
    default:
      return event.type;
  }
}

const styles = StyleSheet.create({
  screen: {
    flex: 1,
    backgroundColor: "#07101D",
  },
  mapSurface: {
    ...StyleSheet.absoluteFillObject,
    zIndex: 0,
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
  firstRunScrim: {
    ...StyleSheet.absoluteFillObject,
    zIndex: 30,
    alignItems: "center",
    justifyContent: "center",
    paddingHorizontal: 20,
    backgroundColor: "rgba(4, 8, 18, 0.62)",
  },
  firstRunCard: {
    width: "100%",
    maxWidth: 420,
    gap: 12,
    padding: 18,
    borderRadius: 8,
    borderWidth: 1,
    borderColor: "rgba(180, 198, 255, 0.34)",
    backgroundColor: "rgba(13, 18, 35, 0.94)",
  },
  firstRunKicker: {
    color: "#FCD34D",
    fontSize: 12,
    fontWeight: "800",
    textTransform: "uppercase",
  },
  firstRunTitle: {
    color: "#F8FAFC",
    fontSize: 21,
    lineHeight: 27,
    fontWeight: "800",
  },
  firstRunBody: {
    color: "#D7E1F0",
    fontSize: 14,
    lineHeight: 21,
  },
  firstRunButton: {
    marginTop: 4,
    minHeight: 44,
    alignItems: "center",
    justifyContent: "center",
    borderRadius: 8,
    backgroundColor: "#36566F",
  },
  firstRunButtonText: {
    color: "#F8FAFC",
    fontWeight: "800",
    fontSize: 15,
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
    gap: 12,
    backgroundColor: "transparent",
    borderTopWidth: 0,
    zIndex: 20,
  },
  chatPanel: {
    borderRadius: 24,
    backgroundColor: "rgba(15, 10, 30, 0.94)",
    borderWidth: 1,
    borderColor: "rgba(200, 184, 255, 0.22)",
    padding: 12,
    shadowColor: "#000",
    shadowOffset: { width: 0, height: 10 },
    shadowOpacity: 0.28,
    shadowRadius: 24,
    elevation: 8,
  },
  chatPanelCollapsed: {
    minHeight: 116,
  },
  chatPanelExpanded: {
    minHeight: 320,
  },
  chatHeader: {
    flexDirection: "row",
    alignItems: "center",
    gap: 10,
  },
  chatTitleWrap: {
    flex: 1,
  },
  chatTitle: {
    color: "#F3ECFF",
    fontSize: 15,
    fontWeight: "800",
  },
  chatSubtitle: {
    color: "#AFA0D1",
    fontSize: 11,
    fontWeight: "600",
    marginTop: 2,
  },
  collapseButton: {
    width: 34,
    height: 34,
    borderRadius: 17,
    backgroundColor: "rgba(91, 63, 166, 0.34)",
    borderWidth: 1,
    borderColor: "rgba(123, 95, 214, 0.5)",
    alignItems: "center",
    justifyContent: "center",
  },
  collapseButtonText: {
    color: "#E8D5FF",
    fontSize: 20,
    fontWeight: "800",
    lineHeight: 22,
  },
  chatPreview: {
    marginTop: 10,
    borderRadius: 16,
    backgroundColor: "rgba(30, 21, 53, 0.72)",
    borderWidth: 1,
    borderColor: "rgba(123, 95, 214, 0.24)",
    paddingHorizontal: 12,
    paddingVertical: 9,
  },
  chatPreviewText: {
    color: "#D7CAEE",
    fontSize: 13,
    lineHeight: 18,
  },
  chatScroll: {
    flexGrow: 0,
    marginTop: 12,
    marginBottom: 8,
  },
  chatScrollContent: {
    gap: 10,
    paddingBottom: 8,
  },
  chatEmpty: {
    borderRadius: 18,
    padding: 14,
    backgroundColor: "rgba(30, 21, 53, 0.58)",
    borderWidth: 1,
    borderColor: "rgba(123, 95, 214, 0.2)",
  },
  chatEmptyTitle: {
    color: "#F3ECFF",
    fontSize: 14,
    fontWeight: "800",
  },
  chatEmptyBody: {
    color: "#AFA0D1",
    fontSize: 12,
    lineHeight: 17,
    marginTop: 4,
  },
  chatBubble: {
    maxWidth: "88%",
    alignSelf: "flex-start",
    borderRadius: 18,
    paddingHorizontal: 13,
    paddingVertical: 10,
    backgroundColor: "rgba(36, 28, 64, 0.96)",
    borderWidth: 1,
    borderColor: "rgba(123, 95, 214, 0.24)",
  },
  chatBubbleUser: {
    alignSelf: "flex-end",
    backgroundColor: "rgba(91, 63, 166, 0.94)",
    borderColor: "rgba(200, 184, 255, 0.34)",
  },
  chatBubbleSystem: {
    backgroundColor: "rgba(58, 45, 84, 0.86)",
    borderColor: "rgba(255, 221, 154, 0.24)",
  },
  chatRole: {
    color: "#C8B8FF",
    fontSize: 11,
    fontWeight: "800",
    textTransform: "uppercase",
    letterSpacing: 0.6,
    marginBottom: 4,
  },
  chatText: {
    color: "#F4EEFF",
    fontSize: 14,
    lineHeight: 20,
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
