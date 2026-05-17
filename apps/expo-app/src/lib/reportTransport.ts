import { Platform } from "react-native";
import { authCheck, getHealth, getReady } from "./bridge";
import {
  isSidecarAuthSessionFailureClass,
  type SettingsReportFailureClass,
  type SidecarAuthSessionFailureClass
} from "./reportFailureClasses";
import {
  getXMiloclawRuntimeStatus,
  hasNativeXMiloclawRuntimeHost,
  startXMiloclawRuntimeHost
} from "./xmiloRuntimeHost";

export type SettingsReportTransportReadiness = {
  ok: boolean;
  reason:
    | "ready"
    | "runtime_host_unavailable"
    | "sidecar_unreachable"
    | "relay_session_unavailable"
    | "backend_unavailable"
    | "unknown";
  failure_class?: SettingsReportFailureClass;
  detail?: string;
};

type TransportReason = SettingsReportTransportReadiness["reason"];

const DEFAULT_TIMEOUT_MS = 10000;
const SIDECAR_POLL_INTERVAL_MS = 350;

export async function ensureSettingsReportTransportReady(options?: {
  timeoutMs?: number;
}): Promise<SettingsReportTransportReadiness> {
  const timeoutMs = Math.max(1500, options?.timeoutMs ?? DEFAULT_TIMEOUT_MS);
  const deadlineAt = Date.now() + timeoutMs;
  return withTimeout(prepareSettingsReportTransport(deadlineAt), timeoutMs);
}

async function prepareSettingsReportTransport(deadlineAt: number): Promise<SettingsReportTransportReadiness> {
  if (Platform.OS === "android" && hasNativeXMiloclawRuntimeHost()) {
    const runtimeReady = await ensureRuntimeHostForReport();
    if (!runtimeReady.ok) return runtimeReady;
  }

  const sidecarReady = await waitForSidecarHttpReady(deadlineAt);
  if (!sidecarReady.ok) return sidecarReady;

  try {
    logSettingsReportTransportBreadcrumb("settings_report_auth_check_attempted");
    await authCheck();
    logSettingsReportTransportBreadcrumb("settings_report_auth_check_ready");
  } catch (error) {
    const failureClass = classifyAuthCheckFailureClass(error);
    logSettingsReportTransportBreadcrumb("settings_report_auth_check_failed", {
      class: failureClass,
      status: safeStatusField(error)
    });
    if (failureClass === "sidecar_health_unreachable") {
      return failure("sidecar_unreachable", error, "sidecar_health_unreachable");
    }
    const reason = classifyRelayReadinessFailure(error);
    return failure(reason, error, failureClass);
  }

  return { ok: true, reason: "ready" };
}

async function waitForSidecarHttpReady(deadlineAt: number): Promise<SettingsReportTransportReadiness> {
  let lastError: unknown = "sidecar unreachable";

  logSettingsReportTransportBreadcrumb("settings_report_health_wait_started");
  while (Date.now() < deadlineAt) {
    try {
      await getHealth();
      logSettingsReportTransportBreadcrumb("settings_report_health_ready");
      try {
        await getReady();
      } catch {
      }
      return { ok: true, reason: "ready" };
    } catch (error) {
      lastError = error;
      await delay(Math.min(SIDECAR_POLL_INTERVAL_MS, Math.max(0, deadlineAt - Date.now())));
    }
  }

  logSettingsReportTransportBreadcrumb("settings_report_health_failed", { class: "sidecar_health_unreachable" });
  return failure("sidecar_unreachable", lastError, "sidecar_health_unreachable");
}

async function ensureRuntimeHostForReport(): Promise<SettingsReportTransportReadiness> {
  const before = await getXMiloclawRuntimeStatus().catch(() => null);
  if (isReportRuntimeUsable(before)) return { ok: true, reason: "ready" };
  if (isReportRuntimeLaunched(before)) return { ok: true, reason: "ready" };

  try {
    const started = await startXMiloclawRuntimeHost();
    if (isReportRuntimeUsable(started)) return { ok: true, reason: "ready" };
    if (isReportRuntimeLaunched(started)) return { ok: true, reason: "ready" };
  } catch (error) {
    return failure("runtime_host_unavailable", error, "transport_start_failed");
  }

  const after = await getXMiloclawRuntimeStatus().catch(() => null);
  if (isReportRuntimeUsable(after)) return { ok: true, reason: "ready" };
  if (isReportRuntimeLaunched(after)) return { ok: true, reason: "ready" };

  return failure("runtime_host_unavailable", after?.lastError ?? after?.lastRuntimeStage ?? "runtime host unavailable", "transport_start_failed");
}

function isReportRuntimeUsable(status: Awaited<ReturnType<typeof getXMiloclawRuntimeStatus>> | null) {
  if (!status) return false;
  return Boolean(
    (status.foregroundServiceStarted ?? status.runtimeHostStarted) &&
      status.runtimeFilesPrepared &&
      status.sidecarProcessAlive &&
      status.bridgeConnected
  );
}

function isReportRuntimeLaunched(status: Awaited<ReturnType<typeof getXMiloclawRuntimeStatus>> | null) {
  if (!status) return false;
  return Boolean(
    (status.foregroundServiceStarted ?? status.runtimeHostStarted) &&
      status.runtimeFilesPrepared &&
      status.sidecarProcessAlive
  );
}

function withTimeout(
  promise: Promise<SettingsReportTransportReadiness>,
  timeoutMs: number
): Promise<SettingsReportTransportReadiness> {
  let timeout: ReturnType<typeof setTimeout> | null = null;
  const timeoutPromise = new Promise<SettingsReportTransportReadiness>((resolve) => {
    timeout = setTimeout(() => {
      resolve({
        ok: false,
        reason: "unknown",
        failure_class: "unknown_report_send_failure",
        detail: "report transport readiness timed out"
      });
    }, timeoutMs);
  });

  return Promise.race([promise, timeoutPromise]).finally(() => {
    if (timeout) clearTimeout(timeout);
  });
}

function delay(ms: number) {
  if (ms <= 0) return Promise.resolve();
  return new Promise<void>((resolve) => setTimeout(resolve, ms));
}

function classifyRelayReadinessFailure(error: unknown): Exclude<TransportReason, "ready"> {
  const text = safeDetail(error).toLowerCase();
  if (text.includes("no jwt") || text.includes("invalid token") || text.includes("unauthorized") || text.includes("401")) {
    return "relay_session_unavailable";
  }
  if (
    text.includes("network") ||
    text.includes("failed to fetch") ||
    text.includes("bad gateway") ||
    text.includes("502") ||
    text.includes("relay") ||
    text.includes("service")
  ) {
    return "backend_unavailable";
  }
  return "relay_session_unavailable";
}

function classifyAuthCheckFailureClass(error: unknown): SettingsReportFailureClass {
  const sidecarClass = sidecarAuthSessionFailureClass(error);
  if (sidecarClass) return sidecarClass;
  const status = sidecarErrorStatus(error);
  if (status === 0) return "sidecar_health_unreachable";
  if (typeof status === "number") return "sidecar_auth_check_failed";
  return looksLikeConnectionFailure(error) ? "sidecar_health_unreachable" : "sidecar_auth_check_failed";
}

function sidecarAuthSessionFailureClass(error: unknown): SidecarAuthSessionFailureClass | null {
  if (!error || typeof error !== "object") return null;
  const value = (error as { errorClass?: unknown }).errorClass;
  return typeof value === "string" && isSidecarAuthSessionFailureClass(value) ? value : null;
}

function sidecarErrorStatus(error: unknown) {
  if (!error || typeof error !== "object") return null;
  const status = (error as { status?: unknown }).status;
  return typeof status === "number" && Number.isFinite(status) ? status : null;
}

function safeStatusField(error: unknown) {
  const status = sidecarErrorStatus(error);
  if (typeof status !== "number" || status < 0) return undefined;
  return String(status);
}

function looksLikeConnectionFailure(error: unknown) {
  const text = safeDetail(error).toLowerCase();
  return (
    text.includes("network request failed") ||
    text.includes("failed to fetch") ||
    text.includes("failed to connect") ||
    text.includes("connection refused") ||
    text.includes("connection reset") ||
    text.includes("connect to") ||
    text.includes("unreachable") ||
    text.includes("warming up") ||
    text.includes("timed out") ||
    text.includes("timeout")
  );
}

function logSettingsReportTransportBreadcrumb(event: string, fields?: Record<string, string | undefined>) {
  const parts = [`xMilo ${event}`];
  for (const [key, value] of Object.entries(fields ?? {})) {
    if (typeof value === "undefined") continue;
    parts.push(`${key}=${value}`);
  }
  console.info(parts.join(" "));
}

function failure(
  reason: Exclude<TransportReason, "ready">,
  error: unknown,
  failureClass: SettingsReportFailureClass
): SettingsReportTransportReadiness {
  return { ok: false, reason, failure_class: failureClass, detail: safeDetail(error) };
}

function safeDetail(error: unknown) {
  const raw = typeof error === "string" ? error : String((error as Error | undefined)?.message ?? error ?? "");
  return raw
    .replace(/eyJ[A-Za-z0-9_-]{10,}\.[A-Za-z0-9_-]{10,}\.[A-Za-z0-9_-]{10,}/g, "<redacted>")
    .replace(/authorization\s*[:=]\s*bearer\s+[^\s,"'}]+/gi, "authorization: bearer <redacted>")
    .replace(/bearer\s+[A-Za-z0-9._-]{16,}/gi, "bearer <redacted>")
    .replace(/[A-Za-z0-9_-]{48,}/g, "<redacted>")
    .slice(0, 160);
}
