import { createContext, type Dispatch, type PropsWithChildren, type SetStateAction, useCallback, useContext, useMemo, useState } from "react";
import type { EventEnvelope, RuntimeState } from "../types/contracts";
import Constants from "expo-constants";

type AppContextValue = {
  state: RuntimeState;
  setState: Dispatch<SetStateAction<RuntimeState>>;
  events: EventEnvelope[];
  pushEvent: (event: EventEnvelope) => void;
  storageStats: Record<string, any> | null;
  setStorageStats: Dispatch<SetStateAction<Record<string, any> | null>>;
  tokenMissing: boolean;
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
  const [storageStats, setStorageStats] = useState<Record<string, any> | null>(null);
  const [wakeWordEnabled, setWakeWordEnabled] = useState(false);

  const extra = Constants.expoConfig?.extra as Record<string, string | undefined> | undefined;
  const tokenMissing = !extra?.localhostToken;

  // Stable reference: setEvents (from useState) never changes, so empty deps is correct.
  // If pushEvent were inline inside useMemo, every received event creates a new function
  // reference → useEffect([pushEvent, setState]) in index.tsx re-runs → WebSocket closed
  // and re-opened → reconnect storm on every event.
  const pushEvent = useCallback((event: EventEnvelope) => {
    setEvents((prev) => [...prev, event].slice(-50));
  }, []);

  const value = useMemo<AppContextValue>(
    () => ({ state, setState, events, pushEvent, storageStats, setStorageStats, tokenMissing, wakeWordEnabled, setWakeWordEnabled }),
    [events, pushEvent, state, storageStats, tokenMissing, wakeWordEnabled]
  );

  return <AppContext.Provider value={value}>{children}</AppContext.Provider>;
}

export function useApp() {
  const value = useContext(AppContext);
  if (!value) throw new Error("useApp must be used within AppProvider");
  return value;
}
