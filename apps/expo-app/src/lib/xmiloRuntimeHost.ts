import { NativeModules, Platform } from "react-native";

import { LOCALHOST_TOKEN, SIDECAR_BASE_URL } from "./config";

export type AppBridgeVerifiedOperation =
  | "runtime_host_status"
  | "runtime_host_start"
  | "runtime_host_restart"
  | "native_sidecar_payload_ready"
  | "sidecar_ready_probe"
  | "task_route_surface_ready"
  | "byok_key_storage"
  | "permission_state_snapshot"
  | "capability_state_snapshot";

export type AppBridgeVerifiedProof = {
  proof_class: "app_bridge_verified";
  verified: boolean;
  source: "android_bridge";
  operation: AppBridgeVerifiedOperation;
  checked_at: string;
  summary: string;
  blocking_reason?: string;
  evidence_id?: string;
  attempt_id?: string;
  task_id?: string;
  details?: Record<string, unknown>;
};

export type TaskAttemptIdentity = {
  task_id: string;
  attempt_id: string;
};

export type XMiloclawRuntimeStatus = {
  notificationsGranted: boolean;
  appearOnTopGranted: boolean;
  batteryUnrestricted: boolean;
  dataSaverUnrestricted: boolean;
  dataSaverStatus: string;
  accessibilityEnabled: boolean;
  runtimeHostStarted: boolean;
  foregroundServiceStarted?: boolean;
  runtimeFilesPrepared?: boolean;
  sidecarProcessLaunched?: boolean;
  sidecarProcessAlive?: boolean;
  bridgeConnected: boolean;
  taskRouteSurfaceReady?: boolean;
  hostReady: boolean;
  llmMode?: string;
  byokProvider?: string;
  subscriptionEntitled?: boolean;
  bringYourOwnKeyActive?: boolean;
  phase9ApiKeyAccess?: boolean;
  firstTaskEligible?: boolean;
  relayLlmTurnAllowed?: boolean;
  localLlmTurnAllowed?: boolean;
  lastRuntimeStage?: string;
  lastHealthCode?: number;
  lastReadyCode?: number;
  lastHealthCategory?: string;
  lastReadyCategory?: string;
  lastProcessExitCode?: number;
  lastProcessUptimeMillis?: number;
  firstSafeStdoutLine?: string;
  firstSafeStdoutCategory?: string;
  firstSafeStderrLine?: string;
  firstSafeStderrCategory?: string;
  lastProcessErrorSummary?: string;
  restartAttempted?: boolean;
  restartSucceeded?: boolean;
  lastError?: string;
  checkedAtMillis?: number;
  bridgeProof?: AppBridgeVerifiedProof;
};

export type XMiloclawCapabilityStatus = {
  declared?: boolean;
  requested?: boolean;
  granted?: boolean;
  available?: boolean;
  tool_available?: boolean;
  tested?: boolean;
  last_verified_at?: string;
  failure_stage?: "manifest" | "permission" | "tool" | "device" | "runtime" | string;
  grant_scope?: "while_using" | "denied" | "unknown" | string;
  location_accuracy?: "precise" | "approximate" | "denied" | "unknown" | string;
  media_access?: "all" | "limited" | "denied" | "unknown" | string;
  accepted_for_setup?: boolean;
  currently_granted?: boolean;
  prompt_state?: "requestable" | "settings_required" | "permanent_denial" | "prompt_unavailable" | "not_applicable" | string;
  can_request_now?: boolean;
  app_op_mode?: string;
  repair_hint?: string;
  note?: string;
};

export type XMiloclawCapabilityState = {
  schema_version: 1;
  checked_at: string;
  runtime_host: {
    online: boolean;
    version: string;
    health: "ready" | "degraded" | "offline" | string;
  };
  capabilities: Record<string, XMiloclawCapabilityStatus>;
  bridgeProof?: AppBridgeVerifiedProof;
};

export type SetupPermissionSnapshotRow = {
  row: "camera" | "microphone" | "media" | "location" | "physical_activity" | "foreground_runtime" | string;
  complete: boolean;
  required: boolean;
  status: string;
  category?: string;
  currently_granted?: boolean;
  grant_scope?: string;
  media_access?: string;
  location_accuracy?: string;
  prompt_state?: string;
  allowed_action: "request" | "open_settings" | "none" | string;
  blocked_reason_key?: string;
  user_reason_key?: string;
  can_request_now?: boolean;
  app_op_mode?: string;
  requirement_text?: string;
  accepted_text?: string;
};

export type SetupPermissionSnapshot = {
  schema_version: number;
  generated_at_millis: number;
  rows: SetupPermissionSnapshotRow[];
  gate: {
    permissions_complete: boolean;
    foreground_runtime_complete: boolean;
    review_required: boolean;
    final_ready_without_review: boolean;
    blocked_reasons: string[];
  };
  blocked_reasons: string[];
  allowed_actions: Array<{ row: string; action: string }>;
};

type XMiloclawRuntimeModuleShape = {
  getStatus?: () => Promise<XMiloclawRuntimeStatus>;
  startRuntimeHost?: () => Promise<XMiloclawRuntimeStatus>;
  restartRuntimeHost?: () => Promise<XMiloclawRuntimeStatus>;
  openAccessibilitySettings?: () => Promise<boolean>;
  openNotificationSettings?: () => Promise<boolean>;
  openOverlaySettings?: () => Promise<boolean>;
  openBatteryOptimizationSettings?: () => Promise<boolean>;
  openDataSaverSettings?: () => Promise<boolean>;
  openSetupPermissionSettings?: (row: string) => Promise<{ row?: string; opened?: boolean; reason?: string }>;
  getLocalhostBearerToken?: () => Promise<string>;
  saveLocalByokApiKey?: (provider: string, apiKey: string, baseUrl: string, model: string) => Promise<string>;
  deactivateLocalByokRouting?: () => Promise<string>;
  getLocalByokStatus?: () => Promise<string>;
  getCapabilityState?: () => Promise<string>;
  getSetupPermissionSnapshot?: () => Promise<SetupPermissionSnapshot>;
  recheckSetupPermissionSnapshot?: () => Promise<SetupPermissionSnapshot>;
};

const nativeModule = NativeModules.XMiloclawRuntimeModule as XMiloclawRuntimeModuleShape | undefined;
let localhostBearerTokenPromise: Promise<string> | null = null;
let currentTaskAttemptIdentity: TaskAttemptIdentity | null = null;

export function getCurrentTaskAttemptIdentity(): TaskAttemptIdentity | null {
  return currentTaskAttemptIdentity ? { ...currentTaskAttemptIdentity } : null;
}

export function clearCurrentTaskAttemptIdentity(reason = "unknown") {
  if (!currentTaskAttemptIdentity) return;
  currentTaskAttemptIdentity = null;
  console.info(`xMilo task proof correlation cleared: ${sanitizeProofLogText(reason)}`);
}

export function captureTaskAttemptIdentityFromSidecarPayload(payload: unknown, reason = "unknown") {
  const identity = findTaskAttemptIdentity(payload);
  if (!identity) return false;
  currentTaskAttemptIdentity = identity;
  console.info(`xMilo task proof correlation captured: ${sanitizeProofLogText(reason)}`);
  return true;
}

export function hasNativeXMiloclawRuntimeHost() {
  return Platform.OS === "android" && Boolean(nativeModule?.getStatus && nativeModule?.startRuntimeHost && nativeModule?.restartRuntimeHost);
}

export async function getXMiloclawRuntimeStatus(): Promise<XMiloclawRuntimeStatus> {
  if (!nativeModule?.getStatus) {
    return {
      notificationsGranted: false,
      appearOnTopGranted: false,
      batteryUnrestricted: false,
      dataSaverUnrestricted: false,
      dataSaverStatus: "unknown",
      accessibilityEnabled: false,
      runtimeHostStarted: false,
      foregroundServiceStarted: false,
      runtimeFilesPrepared: false,
      sidecarProcessLaunched: false,
      sidecarProcessAlive: false,
      bridgeConnected: false,
      taskRouteSurfaceReady: false,
      hostReady: false,
      lastRuntimeStage: "native_module_missing",
      lastHealthCategory: "unknown",
      lastReadyCategory: "unknown",
      firstSafeStdoutCategory: "none",
      firstSafeStderrCategory: "none",
    };
  }
  const status = await nativeModule.getStatus();
  await submitAppBridgeProof(status.bridgeProof);
  return status;
}

export async function startXMiloclawRuntimeHost(): Promise<XMiloclawRuntimeStatus> {
  if (!nativeModule?.startRuntimeHost) {
    throw new Error("Native xMilo runtime host is not available in this build yet.");
  }
  const status = await nativeModule.startRuntimeHost();
  await submitAppBridgeProof(status.bridgeProof);
  return status;
}

export async function restartXMiloclawRuntimeHost(): Promise<XMiloclawRuntimeStatus> {
  if (!nativeModule?.restartRuntimeHost) {
    throw new Error("Native xMilo runtime host restart is not available in this build yet.");
  }
  const status = await nativeModule.restartRuntimeHost();
  await submitAppBridgeProof(status.bridgeProof);
  return status;
}

export async function openXMiloclawAccessibilitySettings() {
  if (!nativeModule?.openAccessibilitySettings) {
    throw new Error("Native accessibility settings launch is not available in this build yet.");
  }
  return nativeModule.openAccessibilitySettings();
}

export async function openXMiloclawNotificationSettings() {
  if (!nativeModule?.openNotificationSettings) {
    throw new Error("Native notification settings launch is not available in this build yet.");
  }
  return nativeModule.openNotificationSettings();
}

export async function openXMiloclawOverlaySettings() {
  if (!nativeModule?.openOverlaySettings) {
    throw new Error("Native overlay settings launch is not available in this build yet.");
  }
  return nativeModule.openOverlaySettings();
}

export async function openXMiloclawBatteryOptimizationSettings() {
  if (!nativeModule?.openBatteryOptimizationSettings) {
    throw new Error("Native battery optimization settings launch is not available in this build yet.");
  }
  return nativeModule.openBatteryOptimizationSettings();
}

export async function openXMiloclawDataSaverSettings() {
  if (!nativeModule?.openDataSaverSettings) {
    throw new Error("Native Data Saver settings launch is not available in this build yet.");
  }
  return nativeModule.openDataSaverSettings();
}

export async function openXMiloclawSetupPermissionSettings(row: string) {
  if (!nativeModule?.openSetupPermissionSettings) {
    throw new Error("Native setup permission settings launch is not available in this build yet.");
  }
  return nativeModule.openSetupPermissionSettings(row);
}

export async function getXMiloclawLocalhostBearerToken() {
  if (!nativeModule?.getLocalhostBearerToken) {
    throw new Error("Native localhost bearer token source is not available in this build yet.");
  }
  const token = await nativeModule.getLocalhostBearerToken();
  if (!token) {
    throw new Error("Localhost bearer token was empty.");
  }
  return token;
}

export type LocalByokStatus = {
  provider: string;
  accessMode: string;
  keyFileReady: boolean;
  keyFilePath: string;
  baseUrlReady: boolean;
  model: string;
  byokReady: boolean;
  bridgeProof?: AppBridgeVerifiedProof;
};

function parseLocalByokStatus(raw: string): LocalByokStatus {
  const parsed = JSON.parse(raw) as Partial<LocalByokStatus>;
  return {
    provider: String(parsed.provider ?? ""),
    accessMode: String(parsed.accessMode ?? ""),
    keyFileReady: Boolean(parsed.keyFileReady),
    keyFilePath: String(parsed.keyFilePath ?? ""),
    baseUrlReady: Boolean(parsed.baseUrlReady),
    model: String(parsed.model ?? ""),
    byokReady: Boolean(parsed.byokReady),
    bridgeProof: isAppBridgeVerifiedProof(parsed.bridgeProof) ? parsed.bridgeProof : undefined,
  };
}

export async function saveXMiloclawLocalByokApiKey(provider: string, apiKey: string, baseUrl = "", model = "") {
  if (!nativeModule?.saveLocalByokApiKey) {
    throw new Error("Native local API key storage is not available in this build yet.");
  }
  const status = parseLocalByokStatus(await nativeModule.saveLocalByokApiKey(provider, apiKey, baseUrl, model));
  await submitAppBridgeProof(status.bridgeProof);
  return status;
}

export async function deactivateXMiloclawLocalByokRouting() {
  if (!nativeModule?.deactivateLocalByokRouting) {
    throw new Error("Native local access-mode switching is not available in this build yet.");
  }
  const status = parseLocalByokStatus(await nativeModule.deactivateLocalByokRouting());
  await submitAppBridgeProof(status.bridgeProof);
  return status;
}

export async function getXMiloclawLocalByokStatus() {
  if (!nativeModule?.getLocalByokStatus) {
    return { provider: "", accessMode: "", keyFileReady: false, keyFilePath: "", baseUrlReady: false, model: "", byokReady: false } satisfies LocalByokStatus;
  }
  return parseLocalByokStatus(await nativeModule.getLocalByokStatus());
}

export async function getXMiloclawCapabilityState(): Promise<XMiloclawCapabilityState> {
  if (!nativeModule?.getCapabilityState) {
    return {
      schema_version: 1,
      checked_at: new Date().toISOString(),
      runtime_host: {
        online: false,
        version: "native_module_missing",
        health: "offline"
      },
      capabilities: {
        runtime_host: {
          available: false,
          tool_available: false,
          tested: true,
          failure_stage: "runtime",
          note: "Native xMilo runtime host module is not available in this build."
        }
      }
    };
  }
  const parsed = JSON.parse(await nativeModule.getCapabilityState()) as XMiloclawCapabilityState;
  await submitAppBridgeProof(parsed.bridgeProof);
  return sanitizeCapabilityStateForApp(parsed);
}

export async function refreshXMiloclawCapabilityState(): Promise<XMiloclawCapabilityState> {
  const state = await getXMiloclawCapabilityState();
  try {
    const bearer = await resolveLocalhostBearerToken();
    const response = await fetch(`${SIDECAR_BASE_URL}/runtime/capability-state`, {
      method: "POST",
      headers: {
        "Content-Type": "application/json",
        Authorization: `Bearer ${bearer}`
      },
      body: JSON.stringify(state)
    });
    if (!response.ok) {
      const text = await response.text().catch(() => "");
      console.warn(`xMilo capability state was not accepted: ${response.status} ${sanitizeProofLogText(text)}`);
    }
  } catch (error) {
    console.warn(`xMilo capability state submission failed: ${sanitizeProofLogText(String((error as Error)?.message ?? error))}`);
  }
  return state;
}

export async function getXMiloclawSetupPermissionSnapshot(): Promise<SetupPermissionSnapshot> {
  if (!nativeModule?.getSetupPermissionSnapshot) {
    return {
      schema_version: 1,
      generated_at_millis: Date.now(),
      rows: [],
      gate: {
        permissions_complete: false,
        foreground_runtime_complete: false,
        review_required: true,
        final_ready_without_review: false,
        blocked_reasons: ["native_snapshot_missing"]
      },
      blocked_reasons: ["native_snapshot_missing"],
      allowed_actions: []
    };
  }
  return nativeModule.getSetupPermissionSnapshot();
}

export async function recheckXMiloclawSetupPermissionSnapshot(): Promise<SetupPermissionSnapshot> {
  if (nativeModule?.recheckSetupPermissionSnapshot) {
    return nativeModule.recheckSetupPermissionSnapshot();
  }
  return getXMiloclawSetupPermissionSnapshot();
}

export function isSetupPermissionFinalReady(snapshot: SetupPermissionSnapshot | null | undefined, reviewedToBottom: boolean) {
  if (!snapshot || !reviewedToBottom) return false;
  const requiredRowsComplete = snapshot.rows.every((row) => row.required !== true || row.complete === true);
  const foregroundRuntimeComplete = snapshot.rows.some((row) => row.row === "foreground_runtime" && row.complete === true);
  return snapshot.gate.final_ready_without_review === true &&
    snapshot.gate.permissions_complete === true &&
    snapshot.gate.foreground_runtime_complete === true &&
    requiredRowsComplete &&
    foregroundRuntimeComplete;
}

export async function resolveLocalhostBearerToken() {
  if (!localhostBearerTokenPromise) {
    localhostBearerTokenPromise = (async () => {
      if (hasNativeXMiloclawRuntimeHost()) {
        return getXMiloclawLocalhostBearerToken();
      }

      if (!LOCALHOST_TOKEN) {
        throw new Error("Native localhost bearer source unavailable, and non-native EXPO_PUBLIC_LOCALHOST_TOKEN fallback is unset.");
      }

      return LOCALHOST_TOKEN;
    })();
  }

  return localhostBearerTokenPromise;
}

function isAppBridgeVerifiedProof(value: unknown): value is AppBridgeVerifiedProof {
  if (!value || typeof value !== "object") return false;
  const proof = value as Partial<AppBridgeVerifiedProof>;
  return proof.proof_class === "app_bridge_verified" &&
    proof.source === "android_bridge" &&
    typeof proof.verified === "boolean" &&
    typeof proof.operation === "string" &&
    typeof proof.checked_at === "string" &&
    typeof proof.summary === "string";
}

async function submitAppBridgeProof(proof?: AppBridgeVerifiedProof) {
  if (!proof) return;
  const correlatedProof = correlateAppBridgeProof(proof);
  if (!correlatedProof) {
    console.warn("xMilo app bridge proof skipped: missing_task_attempt_correlation");
    return;
  }
  const safeProof = sanitizeAppBridgeProof(correlatedProof);
  try {
    const bearer = await resolveLocalhostBearerToken();
    const response = await fetch(`${SIDECAR_BASE_URL}/runtime/evidence/app-bridge`, {
      method: "POST",
      headers: {
        "Content-Type": "application/json",
        Authorization: `Bearer ${bearer}`
      },
      body: JSON.stringify(safeProof)
    });
    if (!response.ok) {
      const text = await response.text().catch(() => "");
      console.warn(`xMilo app bridge proof was not accepted: ${response.status} ${sanitizeProofLogText(text)}`);
    }
  } catch (error) {
    console.warn(`xMilo app bridge proof submission failed: ${sanitizeProofLogText(String((error as Error)?.message ?? error))}`);
  }
}

function correlateAppBridgeProof(proof: AppBridgeVerifiedProof): AppBridgeVerifiedProof | null {
  const taskID = normalizeProofID(proof.task_id) ?? currentTaskAttemptIdentity?.task_id;
  const attemptID = normalizeProofID(proof.attempt_id) ?? currentTaskAttemptIdentity?.attempt_id;
  if (!taskID || !attemptID) return null;
  return {
    ...proof,
    task_id: taskID,
    attempt_id: attemptID
  };
}

function sanitizeAppBridgeProof(proof: AppBridgeVerifiedProof): AppBridgeVerifiedProof {
  return {
    proof_class: "app_bridge_verified",
    verified: Boolean(proof.verified),
    source: "android_bridge",
    operation: proof.operation,
    checked_at: proof.checked_at,
    summary: sanitizeProofLogText(proof.summary),
    blocking_reason: proof.blocking_reason ? sanitizeProofLogText(proof.blocking_reason) : undefined,
    evidence_id: proof.evidence_id ? sanitizeProofLogText(proof.evidence_id) : undefined,
    attempt_id: proof.attempt_id ? sanitizeProofLogText(proof.attempt_id) : undefined,
    task_id: proof.task_id ? sanitizeProofLogText(proof.task_id) : undefined,
    details: sanitizeProofDetails(proof.details)
  };
}

function sanitizeProofDetails(details?: Record<string, unknown>) {
  if (!details) return undefined;
  const safeEntries = Object.entries(details)
    .filter(([key]) => !isSensitiveProofKey(key))
    .slice(0, 32)
    .map(([key, value]) => [key, sanitizeProofDetailValue(value)] as const)
    .filter(([, value]) => value !== undefined);
  return Object.fromEntries(safeEntries);
}

function sanitizeProofDetailValue(value: unknown): unknown {
  if (typeof value === "string") return sanitizeProofLogText(value);
  if (typeof value === "boolean" || typeof value === "number" || value == null) return value;
  if (Array.isArray(value)) {
    return value.slice(0, 16).map(sanitizeProofDetailValue).filter((item) => item !== undefined);
  }
  if (typeof value === "object") {
    return sanitizeProofDetails(value as Record<string, unknown>);
  }
  return undefined;
}

function isSensitiveProofKey(key: string) {
  return /api[_-]?key|bearer|token|secret|password|provider[_-]?key|raw[_-]?key|key[_-]?file[_-]?path|authorization|config|path$/i.test(key);
}

function findTaskAttemptIdentity(value: unknown, depth = 0): TaskAttemptIdentity | null {
  if (!value || typeof value !== "object" || depth > 4) return null;
  const record = value as Record<string, unknown>;
  const direct = normalizeTaskAttemptIdentity(record);
  if (direct) return direct;

  const keysToInspect = [
    "task",
    "active_task",
    "immediate_state",
    "payload",
    "state",
    "result",
    "data"
  ];
  for (const key of keysToInspect) {
    const nested = findTaskAttemptIdentity(record[key], depth + 1);
    if (nested) return nested;
  }

  return null;
}

function normalizeTaskAttemptIdentity(record: Record<string, unknown>): TaskAttemptIdentity | null {
  const taskID = normalizeProofID(record.task_id);
  const attemptID = normalizeProofID(record.attempt_id);
  if (!taskID || !attemptID) return null;
  return { task_id: taskID, attempt_id: attemptID };
}

function normalizeProofID(value: unknown) {
  if (typeof value !== "string") return undefined;
  const trimmed = value.trim();
  return trimmed ? trimmed : undefined;
}

function sanitizeProofLogText(value: string) {
  return value
    .replace(/eyJ[A-Za-z0-9_-]{10,}\.[A-Za-z0-9_-]{10,}\.[A-Za-z0-9_-]{10,}/g, "<redacted>")
    .replace(/authorization\s*[:=]\s*bearer\s+[^\s,"'}]+/gi, "authorization: bearer <redacted>")
    .replace(/bearer\s+[A-Za-z0-9._-]{16,}/gi, "bearer <redacted>")
    .replace(/(api[_-]?key|provider[_-]?key|secret|token)\s*[:=]\s*[^\s,"'}]+/gi, "$1=<redacted>")
    .slice(0, 240);
}

function sanitizeCapabilityStateForApp(state: XMiloclawCapabilityState): XMiloclawCapabilityState {
  const capabilities: Record<string, XMiloclawCapabilityStatus> = {};
  for (const [name, capability] of Object.entries(state.capabilities ?? {})) {
    capabilities[sanitizeCapabilityKey(name)] = {
      declared: typeof capability.declared === "boolean" ? capability.declared : undefined,
      requested: typeof capability.requested === "boolean" ? capability.requested : undefined,
      granted: typeof capability.granted === "boolean" ? capability.granted : undefined,
      available: typeof capability.available === "boolean" ? capability.available : undefined,
      tool_available: typeof capability.tool_available === "boolean" ? capability.tool_available : undefined,
      tested: typeof capability.tested === "boolean" ? capability.tested : undefined,
      last_verified_at: typeof capability.last_verified_at === "string" ? sanitizeProofLogText(capability.last_verified_at) : undefined,
      failure_stage: typeof capability.failure_stage === "string" ? sanitizeProofLogText(capability.failure_stage) : undefined,
      grant_scope: typeof capability.grant_scope === "string" ? sanitizeProofLogText(capability.grant_scope) : undefined,
      location_accuracy: typeof capability.location_accuracy === "string" ? sanitizeProofLogText(capability.location_accuracy) : undefined,
      media_access: typeof capability.media_access === "string" ? sanitizeProofLogText(capability.media_access) : undefined,
      accepted_for_setup: typeof capability.accepted_for_setup === "boolean" ? capability.accepted_for_setup : undefined,
      currently_granted: typeof capability.currently_granted === "boolean" ? capability.currently_granted : undefined,
      prompt_state: typeof capability.prompt_state === "string" ? sanitizeProofLogText(capability.prompt_state) : undefined,
      can_request_now: typeof capability.can_request_now === "boolean" ? capability.can_request_now : undefined,
      app_op_mode: typeof capability.app_op_mode === "string" ? sanitizeProofLogText(capability.app_op_mode) : undefined,
      repair_hint: typeof capability.repair_hint === "string" ? sanitizeProofLogText(capability.repair_hint) : undefined,
      note: typeof capability.note === "string" ? sanitizeProofLogText(capability.note) : undefined
    };
  }
  return {
    schema_version: 1,
    checked_at: typeof state.checked_at === "string" ? sanitizeProofLogText(state.checked_at) : new Date().toISOString(),
    runtime_host: {
      online: Boolean(state.runtime_host?.online),
      version: sanitizeProofLogText(String(state.runtime_host?.version ?? "unknown")),
      health: sanitizeProofLogText(String(state.runtime_host?.health ?? "unknown"))
    },
    capabilities,
    bridgeProof: isAppBridgeVerifiedProof(state.bridgeProof) ? state.bridgeProof : undefined
  };
}

function sanitizeCapabilityKey(value: string) {
  return value.replace(/[^a-z0-9_:-]/gi, "_").slice(0, 64);
}
