import { createContext, type Dispatch, type PropsWithChildren, type SetStateAction, useCallback, useContext, useEffect, useMemo, useState } from "react";
import type { ArtPresentationState, EventEnvelope, RuntimeState } from "../types/contracts";
import { getAppSetting, initArchiveDb, setAppSetting } from "../lib/archiveDb";
import { connectBridge, getState, getTaskCurrent } from "../lib/bridge";
import { getLatestNightlyRitualState, type NightlyRitualState } from "../lib/maintenanceRitual";
import { deriveArtPresentationState } from "../lib/artPresentation";
import { eventNeedsRuntimeRefresh, safeLogRuntimeEvent, safeLogRuntimeRefresh } from "../lib/runtimeEvents";

type AppContextValue = {
  state: RuntimeState;
  setState: Dispatch<SetStateAction<RuntimeState>>;
  events: EventEnvelope[];
  pushEvent: (event: EventEnvelope) => void;
  resetRuntimeTaskProof: () => void;
  taskProofResetVersion: number;
  storageStats: Record<string, any> | null;
  setStorageStats: Dispatch<SetStateAction<Record<string, any> | null>>;
  refreshRuntimeState: () => Promise<void>;
  nightlyRitual: NightlyRitualState | null;
  artPresentation: ArtPresentationState;
  speakAloudEnabled: boolean;
  setSpeakAloudEnabled: Dispatch<SetStateAction<boolean>>;
  wakeWordEnabled: boolean;
  setWakeWordEnabled: Dispatch<SetStateAction<boolean>>;
};

const AppContext = createContext<AppContextValue | null>(null);

const initialState: RuntimeState = {
  milo_state: "idle",
  current_room_id: "main_hall",
  current_anchor_id: "main_hall_center",
  last_meaningful_user_action_at: "",
  active_task: null,
  queued_task: null,
  runtime_id: "app-shell"
};

export function AppProvider({ children }: PropsWithChildren) {
  const [state, setState] = useState<RuntimeState>(initialState);
  const [events, setEvents] = useState<EventEnvelope[]>([]);
  const [taskProofResetVersion, setTaskProofResetVersion] = useState(0);
  const [storageStats, setStorageStats] = useState<Record<string, any> | null>(null);
  const [speakAloudEnabled, setSpeakAloudEnabled] = useState(true);
  const [wakeWordEnabled, setWakeWordEnabled] = useState(false);

  useEffect(() => {
    initArchiveDb()
      .then(async () => {
        const [wakeWordValue, speakAloudValue] = await Promise.all([
          getAppSetting("wake_word_enabled"),
          getAppSetting("speak_aloud_enabled")
        ]);

        if (wakeWordValue !== null) {
          setWakeWordEnabled(wakeWordValue === "true");
        }

        if (speakAloudValue !== null) {
          setSpeakAloudEnabled(speakAloudValue === "true");
        }
      })
      .catch(() => null);
  }, []);

  useEffect(() => {
    initArchiveDb()
      .then(() => setAppSetting("wake_word_enabled", wakeWordEnabled ? "true" : "false"))
      .catch(() => null);
  }, [wakeWordEnabled]);

  useEffect(() => {
    initArchiveDb()
      .then(() => setAppSetting("speak_aloud_enabled", speakAloudEnabled ? "true" : "false"))
      .catch(() => null);
  }, [speakAloudEnabled]);

  // Stable reference: setEvents (from useState) never changes, so empty deps is correct.
  // If pushEvent were inline inside useMemo, every received event creates a new function
  // reference → useEffect([pushEvent, setState]) in index.tsx re-runs → WebSocket closed
  // and re-opened → reconnect storm on every event.
  const pushEvent = useCallback((event: EventEnvelope) => {
    setEvents((prev) => [...prev, event].slice(-50));
  }, []);

  const resetRuntimeTaskProof = useCallback(() => {
    setEvents([]);
    setTaskProofResetVersion((version) => version + 1);
    setState((prev) => ({
      ...prev,
      active_task: null,
      queued_task: null,
    }));
  }, []);

  const refreshRuntimeState = useCallback(async () => {
    const [stateResult, taskResult] = await Promise.allSettled([getState(), getTaskCurrent()]);

    if (stateResult.status === "fulfilled") {
      setState(stateResult.value);
    }

    if (taskResult.status === "fulfilled") {
      const task = taskResult.value?.task ?? null;
      setState((prev) => ({ ...prev, active_task: task }));
      safeLogRuntimeRefresh(task);
    }
  }, []);

  useEffect(() => {
    let disposed = false;

    const cleanup = connectBridge((event) => {
      if (disposed) return;
      safeLogRuntimeEvent(event);
      pushEvent(event);
      if (eventNeedsRuntimeRefresh(event)) {
        void refreshRuntimeState();
      }
    });

    void refreshRuntimeState();

    return () => {
      disposed = true;
      cleanup();
    };
  }, [pushEvent, refreshRuntimeState]);

  const nightlyRitual = useMemo(() => getLatestNightlyRitualState(events), [events]);
  const artPresentation = useMemo(
    () => deriveArtPresentationState({ state, events, nightlyRitual }),
    [events, nightlyRitual, state]
  );

  const value = useMemo<AppContextValue>(
    () => ({
      state,
      setState,
      events,
      pushEvent,
      resetRuntimeTaskProof,
      taskProofResetVersion,
      storageStats,
      setStorageStats,
      refreshRuntimeState,
      nightlyRitual,
      artPresentation,
      speakAloudEnabled,
      setSpeakAloudEnabled,
      wakeWordEnabled,
      setWakeWordEnabled
    }),
    [artPresentation, events, nightlyRitual, pushEvent, refreshRuntimeState, resetRuntimeTaskProof, speakAloudEnabled, state, storageStats, taskProofResetVersion, wakeWordEnabled]
  );

  return <AppContext.Provider value={value}>{children}</AppContext.Provider>;
}

export function useApp() {
  const value = useContext(AppContext);
  if (!value) throw new Error("useApp must be used within AppProvider");
  return value;
}
