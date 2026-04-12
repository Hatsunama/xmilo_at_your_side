import { Platform } from "react-native";

import {
  getXMiloclawRuntimeStatus,
  hasNativeXMiloclawRuntimeHost,
  restartXMiloclawRuntimeHost,
} from "./xmiloRuntimeHost";

export type RuntimeCheckSnapshot = {
  checked_at: string;
  runtime_host_started: boolean;
  bridge_connected: boolean;
  host_ready: boolean;
  restart_attempted?: boolean;
  restart_succeeded?: boolean;
  last_error?: string;
};

export type RuntimeRecoveryResult =
  | "restart_verified"
  | "restart_failed"
  | "restart_rate_limited"
  | "restart_needs_operator";

export type RuntimeRecoveryState = {
  status: "unknown" | "healthy" | "down" | "unready";
  last_check?: RuntimeCheckSnapshot;
  last_result?: {
    result: RuntimeRecoveryResult;
    at: string;
    note?: string;
  };
  budget_window_started_at?: string;
  next_allowed_at?: string;
  attempts_used: number;
  attempting: boolean;
  runtimeHostStarted?: boolean;
  bridgeConnected?: boolean;
  notificationsGranted?: boolean;
  appearOnTopGranted?: boolean;
  batteryUnrestricted?: boolean;
  accessibilityEnabled?: boolean;
};

const VERIFY_BUDGET_MAX_ATTEMPTS = 3;
const VERIFY_BUDGET_WINDOW_MS = 10 * 60 * 1000;
const VERIFY_BACKOFF_MS = [0, 15_000, 45_000];

type AttemptBudget = {
  window_started_at_ms: number;
  attempts_used: number;
  next_allowed_at_ms: number;
};

function nowIso() {
  return new Date().toISOString();
}

async function checkRuntime(): Promise<RuntimeCheckSnapshot> {
  const runtime = await getXMiloclawRuntimeStatus().catch(() => null);
  return {
    checked_at: nowIso(),
    runtime_host_started: runtime?.runtimeHostStarted ?? false,
    bridge_connected: runtime?.bridgeConnected ?? false,
    host_ready: runtime?.hostReady ?? false,
    restart_attempted: runtime?.restartAttempted,
    restart_succeeded: runtime?.restartSucceeded,
    last_error: runtime?.lastError,
  };
}

function classifyStatus(snapshot: RuntimeCheckSnapshot): RuntimeRecoveryState["status"] {
  if (snapshot.host_ready) return "healthy";
  if (snapshot.runtime_host_started || snapshot.bridge_connected) return "unready";
  return "down";
}

function initBudget(nowMs: number): AttemptBudget {
  return {
    window_started_at_ms: nowMs,
    attempts_used: 0,
    next_allowed_at_ms: nowMs,
  };
}

function currentBudget(state: RuntimeRecoveryState, nowMs: number): AttemptBudget {
  const windowStarted = state.budget_window_started_at ? Date.parse(state.budget_window_started_at) : nowMs;
  const nextAllowed = state.next_allowed_at ? Date.parse(state.next_allowed_at) : nowMs;
  return {
    window_started_at_ms: Number.isFinite(windowStarted) ? windowStarted : nowMs,
    attempts_used: state.attempts_used ?? 0,
    next_allowed_at_ms: Number.isFinite(nextAllowed) ? nextAllowed : nowMs,
  };
}

function applyBudgetAttempt(budget: AttemptBudget, nowMs: number): AttemptBudget {
  let next = budget;
  if (nowMs - budget.window_started_at_ms > VERIFY_BUDGET_WINDOW_MS) {
    next = initBudget(nowMs);
  }

  const attemptsUsed = next.attempts_used + 1;
  const backoff = VERIFY_BACKOFF_MS[Math.min(attemptsUsed - 1, VERIFY_BACKOFF_MS.length - 1)] ?? 0;
  return {
    window_started_at_ms: next.window_started_at_ms,
    attempts_used: attemptsUsed,
    next_allowed_at_ms: nowMs + backoff,
  };
}

function canAttempt(budget: AttemptBudget, nowMs: number) {
  if (budget.attempts_used >= VERIFY_BUDGET_MAX_ATTEMPTS) return false;
  return nowMs >= budget.next_allowed_at_ms;
}

export function initRuntimeRecoveryState(): RuntimeRecoveryState {
  return {
    status: "unknown",
    attempts_used: 0,
    attempting: false,
  };
}

export async function refreshRuntimeRecoveryState(current: RuntimeRecoveryState): Promise<RuntimeRecoveryState> {
  const snapshot = await checkRuntime();
  const runtime = await getXMiloclawRuntimeStatus().catch(() => null);
  return {
    ...current,
    status: classifyStatus(snapshot),
    last_check: snapshot,
    runtimeHostStarted: runtime?.runtimeHostStarted ?? current.runtimeHostStarted,
    bridgeConnected: runtime?.bridgeConnected ?? current.bridgeConnected,
    notificationsGranted: runtime?.notificationsGranted ?? current.notificationsGranted,
    appearOnTopGranted: runtime?.appearOnTopGranted ?? current.appearOnTopGranted,
    batteryUnrestricted: runtime?.batteryUnrestricted ?? current.batteryUnrestricted,
    accessibilityEnabled: runtime?.accessibilityEnabled ?? current.accessibilityEnabled,
  };
}

export async function attemptRuntimeRecovery(current: RuntimeRecoveryState): Promise<{
  next: RuntimeRecoveryState;
}> {
  const nowMs = Date.now();
  const budget = currentBudget(current, nowMs);

  if (!canAttempt(budget, nowMs)) {
    return {
      next: {
        ...current,
        attempting: false,
        budget_window_started_at: new Date(budget.window_started_at_ms).toISOString(),
        attempts_used: budget.attempts_used,
        next_allowed_at: new Date(budget.next_allowed_at_ms).toISOString(),
        last_result: {
          result: budget.attempts_used >= VERIFY_BUDGET_MAX_ATTEMPTS ? "restart_needs_operator" : "restart_rate_limited",
          at: nowIso(),
          note: budget.attempts_used >= VERIFY_BUDGET_MAX_ATTEMPTS ? "budget exhausted" : "backoff active",
        },
      },
    };
  }

  const refreshed = await checkRuntime();
  const status = classifyStatus(refreshed);
  if (status === "healthy") {
    return {
      next: {
        ...current,
        status,
        last_check: refreshed,
        attempting: false,
        last_result: {
          result: "restart_verified",
          at: nowIso(),
          note: "already healthy",
        },
      },
    };
  }

  const nextBudget = applyBudgetAttempt(budget, nowMs);
  if (Platform.OS !== "android" || !hasNativeXMiloclawRuntimeHost()) {
    return {
      next: {
        ...current,
        status,
        last_check: refreshed,
        attempting: false,
        budget_window_started_at: new Date(nextBudget.window_started_at_ms).toISOString(),
        attempts_used: nextBudget.attempts_used,
        next_allowed_at: new Date(nextBudget.next_allowed_at_ms).toISOString(),
        last_result: { result: "restart_needs_operator", at: nowIso(), note: "hidden runtime host unavailable" },
      },
    };
  }

  try {
    const runtime = await restartXMiloclawRuntimeHost();
    const verified = await checkRuntime();
    const verifiedStatus = classifyStatus(verified);
    return {
      next: {
        ...current,
        status: verifiedStatus,
        last_check: verified,
        attempting: false,
        budget_window_started_at: new Date(nextBudget.window_started_at_ms).toISOString(),
        attempts_used: nextBudget.attempts_used,
        next_allowed_at: new Date(nextBudget.next_allowed_at_ms).toISOString(),
        runtimeHostStarted: runtime.runtimeHostStarted,
        bridgeConnected: runtime.bridgeConnected,
        notificationsGranted: runtime.notificationsGranted,
        appearOnTopGranted: runtime.appearOnTopGranted,
        batteryUnrestricted: runtime.batteryUnrestricted,
        accessibilityEnabled: runtime.accessibilityEnabled,
        last_result: {
          result: verified.host_ready ? "restart_verified" : "restart_failed",
          at: nowIso(),
          note: runtime.lastError ?? verified.last_error ?? "runtime host not ready",
        },
      },
    };
  } catch (error: any) {
    return {
      next: {
        ...current,
        status,
        last_check: refreshed,
        attempting: false,
        budget_window_started_at: new Date(nextBudget.window_started_at_ms).toISOString(),
        attempts_used: nextBudget.attempts_used,
        next_allowed_at: new Date(nextBudget.next_allowed_at_ms).toISOString(),
        last_result: {
          result: "restart_failed",
          at: nowIso(),
          note: error?.message ?? "recovery failed",
        },
      },
    };
  }
}
