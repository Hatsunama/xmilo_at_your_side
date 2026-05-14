import { Platform } from "react-native";

import {
  getXMiloclawRuntimeStatus,
  hasNativeXMiloclawRuntimeHost,
  restartXMiloclawRuntimeHost,
} from "./xmiloRuntimeHost";

export type RuntimeCheckSnapshot = {
  source: "android_bridge_observation";
  truth_scope: RuntimeRecoveryTruthScope[];
  checked_at: string;
  runtime_host_started: boolean;
  foreground_service_started: boolean;
  runtime_files_prepared: boolean;
  sidecar_process_launched: boolean;
  sidecar_process_alive: boolean;
  bridge_connected: boolean;
  task_route_surface_ready: boolean;
  host_ready: boolean;
  last_runtime_stage?: string;
  last_health_code?: number;
  last_ready_code?: number;
  last_health_category?: string;
  last_ready_category?: string;
  restart_attempted?: boolean;
  restart_succeeded?: boolean;
  last_error?: string;
};

export type RuntimeRecoveryResult =
  | "restart_verified"
  | "restart_failed"
  | "restart_rate_limited"
  | "restart_needs_operator";

export type RuntimeRecoverySource = "android_bridge_observation" | "ui_local" | "sidecar_runtime";

export type RuntimeRecoveryTruthScope =
  | "sidecar_health"
  | "sidecar_ready"
  | "native_runtime_host"
  | "task_route_surface"
  | "ui_submit";

export type RuntimeRecoveryOutcome =
  | {
      result: "restart_verified";
      source: "android_bridge_observation" | "sidecar_runtime";
      truth_scope: RuntimeRecoveryTruthScope[];
      verified: true;
      checked_at: string;
      note: string;
      attempts_used: number;
      blocking_reason?: never;
      next_allowed_at?: string;
    }
  | {
      result: "restart_failed" | "restart_rate_limited" | "restart_needs_operator";
      source: RuntimeRecoverySource;
      truth_scope: RuntimeRecoveryTruthScope[];
      verified: false;
      checked_at: string;
      note: string;
      blocking_reason: string;
      attempts_used: number;
      next_allowed_at?: string;
    };

export type RuntimeRecoveryState = {
  status: "unknown" | "healthy" | "down" | "unready";
  status_source?: "android_bridge_observation";
  status_truth_scope?: RuntimeRecoveryTruthScope[];
  last_check?: RuntimeCheckSnapshot;
  last_result?: RuntimeRecoveryOutcome;
  budget_window_started_at?: string;
  next_allowed_at?: string;
  attempts_used: number;
  attempting: boolean;
  runtimeHostStarted?: boolean;
  foregroundServiceStarted?: boolean;
  runtimeFilesPrepared?: boolean;
  sidecarProcessLaunched?: boolean;
  sidecarProcessAlive?: boolean;
  bridgeConnected?: boolean;
  taskRouteSurfaceReady?: boolean;
  notificationsGranted?: boolean;
  appearOnTopGranted?: boolean;
  batteryUnrestricted?: boolean;
  accessibilityEnabled?: boolean;
};

const VERIFY_BUDGET_MAX_ATTEMPTS = 3;
const VERIFY_BUDGET_WINDOW_MS = 10 * 60 * 1000;
const VERIFY_BACKOFF_MS = [0, 15_000, 45_000];
const TASK_ROUTE_SURFACE_UNAVAILABLE =
  "runtime host is running, but the full task route surface is unavailable";
const SERVICE_ONLY_UNAVAILABLE =
  "foreground service is running, but runtime files/process/readiness are not complete";

type AttemptBudget = {
  window_started_at_ms: number;
  attempts_used: number;
  next_allowed_at_ms: number;
};

function nowIso() {
  return new Date().toISOString();
}

function recoveryOutcome(
  result: RuntimeRecoveryResult,
  options: {
    source: RuntimeRecoverySource;
    truth_scope: RuntimeRecoveryTruthScope[];
    verified: boolean;
    checked_at?: string;
    note: string;
    attempts_used: number;
    blocking_reason?: string;
    next_allowed_at?: string;
  },
): RuntimeRecoveryOutcome {
  if (result === "restart_verified") {
    if (!options.verified) {
      throw new Error("restart_verified requires verified=true");
    }
    if (options.source === "ui_local") {
      throw new Error("ui_local cannot verify recovery success");
    }
    if (!options.truth_scope.includes("sidecar_ready") && !options.truth_scope.includes("task_route_surface")) {
      throw new Error("restart_verified requires sidecar_ready or task_route_surface proof");
    }
    return {
      result,
      source: options.source,
      truth_scope: options.truth_scope,
      verified: true,
      checked_at: options.checked_at ?? nowIso(),
      note: options.note,
      attempts_used: options.attempts_used,
      next_allowed_at: options.next_allowed_at,
    };
  }

  if (options.verified) {
    throw new Error(`${result} requires verified=false`);
  }
  if (!options.blocking_reason) {
    throw new Error(`${result} requires blocking_reason`);
  }
  return {
    result,
    source: options.source,
    truth_scope: options.truth_scope,
    verified: false,
    checked_at: options.checked_at ?? nowIso(),
    note: options.note,
    blocking_reason: options.blocking_reason,
    attempts_used: options.attempts_used,
    next_allowed_at: options.next_allowed_at,
  };
}

export function makeUiLocalRecoveryFailure(note: string, attemptsUsed: number): RuntimeRecoveryOutcome {
  return recoveryOutcome("restart_failed", {
    source: "ui_local",
    truth_scope: ["ui_submit"],
    verified: false,
    note,
    attempts_used: attemptsUsed,
    blocking_reason: "ui_recovery_exception",
  });
}

async function checkRuntime(): Promise<RuntimeCheckSnapshot> {
  const runtime = await getXMiloclawRuntimeStatus().catch(() => null);
  const runtimeHostStarted = runtime?.runtimeHostStarted ?? false;
  const foregroundServiceStarted = runtime?.foregroundServiceStarted ?? runtimeHostStarted;
  const runtimeFilesPrepared = runtime?.runtimeFilesPrepared ?? false;
  const sidecarProcessLaunched = runtime?.sidecarProcessLaunched ?? false;
  const sidecarProcessAlive = runtime?.sidecarProcessAlive ?? false;
  const bridgeConnected = runtime?.bridgeConnected ?? false;
  const taskRouteSurfaceReady = runtime?.taskRouteSurfaceReady ?? false;
  return {
    source: "android_bridge_observation",
    truth_scope: ["native_runtime_host", "sidecar_health", "sidecar_ready", "task_route_surface"],
    checked_at: nowIso(),
    runtime_host_started: runtimeHostStarted,
    foreground_service_started: foregroundServiceStarted,
    runtime_files_prepared: runtimeFilesPrepared,
    sidecar_process_launched: sidecarProcessLaunched,
    sidecar_process_alive: sidecarProcessAlive,
    bridge_connected: bridgeConnected,
    task_route_surface_ready: taskRouteSurfaceReady,
    host_ready: runtime?.hostReady ?? false,
    last_runtime_stage: runtime?.lastRuntimeStage,
    last_health_code: runtime?.lastHealthCode,
    last_ready_code: runtime?.lastReadyCode,
    last_health_category: runtime?.lastHealthCategory,
    last_ready_category: runtime?.lastReadyCategory,
    restart_attempted: runtime?.restartAttempted,
    restart_succeeded: runtime?.restartSucceeded,
    last_error:
      runtime?.lastError ??
      (foregroundServiceStarted && (!runtimeFilesPrepared || !sidecarProcessAlive)
        ? SERVICE_ONLY_UNAVAILABLE
        : undefined) ??
      (runtimeHostStarted && bridgeConnected && !taskRouteSurfaceReady ? TASK_ROUTE_SURFACE_UNAVAILABLE : undefined),
  };
}

function classifyStatus(snapshot: RuntimeCheckSnapshot): RuntimeRecoveryState["status"] {
  if (snapshot.host_ready && snapshot.task_route_surface_ready) return "healthy";
  if (snapshot.foreground_service_started || snapshot.runtime_host_started || snapshot.bridge_connected) return "unready";
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
    status_source: "android_bridge_observation",
    status_truth_scope: ["native_runtime_host", "sidecar_health", "sidecar_ready", "task_route_surface"],
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
    status_source: "android_bridge_observation",
    status_truth_scope: snapshot.truth_scope,
    last_check: snapshot,
    runtimeHostStarted: runtime?.runtimeHostStarted ?? current.runtimeHostStarted,
    foregroundServiceStarted: runtime?.foregroundServiceStarted ?? current.foregroundServiceStarted,
    runtimeFilesPrepared: runtime?.runtimeFilesPrepared ?? current.runtimeFilesPrepared,
    sidecarProcessLaunched: runtime?.sidecarProcessLaunched ?? current.sidecarProcessLaunched,
    sidecarProcessAlive: runtime?.sidecarProcessAlive ?? current.sidecarProcessAlive,
    bridgeConnected: runtime?.bridgeConnected ?? current.bridgeConnected,
    taskRouteSurfaceReady: runtime?.taskRouteSurfaceReady ?? current.taskRouteSurfaceReady,
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
        last_result: recoveryOutcome(
          budget.attempts_used >= VERIFY_BUDGET_MAX_ATTEMPTS ? "restart_needs_operator" : "restart_rate_limited",
          {
            source: "android_bridge_observation",
            truth_scope: ["native_runtime_host"],
            verified: false,
            note: budget.attempts_used >= VERIFY_BUDGET_MAX_ATTEMPTS ? "budget exhausted" : "backoff active",
            attempts_used: budget.attempts_used,
            blocking_reason:
              budget.attempts_used >= VERIFY_BUDGET_MAX_ATTEMPTS
                ? "restart_budget_exhausted"
                : "restart_backoff_active",
            next_allowed_at: new Date(budget.next_allowed_at_ms).toISOString(),
          },
        ),
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
        status_source: "android_bridge_observation",
        status_truth_scope: refreshed.truth_scope,
        last_check: refreshed,
        attempting: false,
        last_result: recoveryOutcome("restart_verified", {
          source: "android_bridge_observation",
          truth_scope: ["sidecar_ready", "task_route_surface"],
          verified: true,
          note: "already healthy",
          attempts_used: current.attempts_used ?? 0,
        }),
      },
    };
  }

  const nextBudget = applyBudgetAttempt(budget, nowMs);
  if (Platform.OS !== "android" || !hasNativeXMiloclawRuntimeHost()) {
    return {
      next: {
        ...current,
        status,
        status_source: "android_bridge_observation",
        status_truth_scope: refreshed.truth_scope,
        last_check: refreshed,
        attempting: false,
        budget_window_started_at: new Date(nextBudget.window_started_at_ms).toISOString(),
        attempts_used: nextBudget.attempts_used,
        next_allowed_at: new Date(nextBudget.next_allowed_at_ms).toISOString(),
        last_result: recoveryOutcome("restart_needs_operator", {
          source: "android_bridge_observation",
          truth_scope: ["native_runtime_host"],
          verified: false,
          note: "runtime host unavailable",
          attempts_used: nextBudget.attempts_used,
          blocking_reason: "runtime_host_unavailable",
          next_allowed_at: new Date(nextBudget.next_allowed_at_ms).toISOString(),
        }),
      },
    };
  }

  try {
    const runtime = await restartXMiloclawRuntimeHost();
    const verified = await checkRuntime();
    const verifiedStatus = classifyStatus(verified);
    const verifiedReady = verified.host_ready && verified.task_route_surface_ready;
    return {
      next: {
        ...current,
        status: verifiedStatus,
        status_source: "android_bridge_observation",
        status_truth_scope: verified.truth_scope,
        last_check: verified,
        attempting: false,
        budget_window_started_at: new Date(nextBudget.window_started_at_ms).toISOString(),
        attempts_used: nextBudget.attempts_used,
        next_allowed_at: new Date(nextBudget.next_allowed_at_ms).toISOString(),
        runtimeHostStarted: runtime.runtimeHostStarted,
        foregroundServiceStarted: runtime.foregroundServiceStarted,
        runtimeFilesPrepared: runtime.runtimeFilesPrepared,
        sidecarProcessLaunched: runtime.sidecarProcessLaunched,
        sidecarProcessAlive: runtime.sidecarProcessAlive,
        bridgeConnected: runtime.bridgeConnected,
        taskRouteSurfaceReady: runtime.taskRouteSurfaceReady,
        notificationsGranted: runtime.notificationsGranted,
        appearOnTopGranted: runtime.appearOnTopGranted,
        batteryUnrestricted: runtime.batteryUnrestricted,
        accessibilityEnabled: runtime.accessibilityEnabled,
        last_result: recoveryOutcome(verifiedReady ? "restart_verified" : "restart_failed", {
          source: "android_bridge_observation",
          truth_scope: verifiedReady ? ["sidecar_ready", "task_route_surface"] : ["native_runtime_host"],
          verified: verifiedReady,
          note:
            runtime.lastError ??
            verified.last_error ??
            (verified.foreground_service_started && (!verified.runtime_files_prepared || !verified.sidecar_process_alive)
              ? SERVICE_ONLY_UNAVAILABLE
              : undefined) ??
            (!verified.task_route_surface_ready && verified.runtime_host_started && verified.bridge_connected
              ? TASK_ROUTE_SURFACE_UNAVAILABLE
              : "runtime host not ready"),
          attempts_used: nextBudget.attempts_used,
          blocking_reason: verifiedReady ? undefined : "runtime_host_not_ready_after_restart",
          next_allowed_at: new Date(nextBudget.next_allowed_at_ms).toISOString(),
        }),
      },
    };
  } catch (error: any) {
    return {
      next: {
        ...current,
        status,
        status_source: "android_bridge_observation",
        status_truth_scope: refreshed.truth_scope,
        last_check: refreshed,
        attempting: false,
        budget_window_started_at: new Date(nextBudget.window_started_at_ms).toISOString(),
        attempts_used: nextBudget.attempts_used,
        next_allowed_at: new Date(nextBudget.next_allowed_at_ms).toISOString(),
        last_result: recoveryOutcome("restart_failed", {
          source: "android_bridge_observation",
          truth_scope: ["native_runtime_host"],
          verified: false,
          note: error?.message ?? "recovery failed",
          attempts_used: nextBudget.attempts_used,
          blocking_reason: "runtime_restart_exception",
          next_allowed_at: new Date(nextBudget.next_allowed_at_ms).toISOString(),
        }),
      },
    };
  }
}
