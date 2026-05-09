import type { CommandSubmitResponse, EventEnvelope, ProviderDiagnosticPayload, TaskSnapshot } from "../types/contracts";
import { toUserFacingLocalProviderError, type SidecarReadyStatus } from "./bridge";

const REFRESH_EVENT_TYPES = new Set([
  "runtime.ready",
  "runtime.error",
  "task.accepted",
  "task.completed",
  "task.stuck",
  "task.cancelled",
  "task.stale_active_recovered",
  "report.ready",
  "local_provider.diagnostic",
  "milo.state_changed",
  "milo.room_changed",
  "milo.movement_started",
]);

const TASK_EVENT_TYPES = new Set([
  "task.accepted",
  "task.completed",
  "task.stuck",
  "task.cancelled",
  "task.stale_active_recovered",
  "report.ready",
  "runtime.error",
  "local_provider.diagnostic",
]);

function safeToken(value: unknown, fallback = "") {
  if (typeof value !== "string" && typeof value !== "number" && typeof value !== "boolean") return fallback;
  const text = String(value).trim();
  if (!text) return fallback;
  return text.replace(/[^A-Za-z0-9_.:/-]/g, "_").slice(0, 96);
}

function safeHost(value: unknown) {
  if (typeof value !== "string") return "";
  return value.trim().replace(/[^A-Za-z0-9.-]/g, "_").slice(0, 96);
}

function safePath(value: unknown) {
  if (typeof value !== "string") return "";
  const path = value.trim();
  if (!path.startsWith("/")) return "";
  return path.replace(/[^A-Za-z0-9_./-]/g, "_").slice(0, 96);
}

function safeStatus(value: unknown) {
  return typeof value === "number" && Number.isFinite(value) ? String(value) : "";
}

function safeMessage(value: unknown, fallback: string) {
  if (typeof value !== "string") return fallback;
  const text = value.trim();
  if (!text) return fallback;
  return redactSensitiveText(text).replace(/\s+/g, " ").slice(0, 160);
}

export function safeDisplayText(value: unknown, fallback = "Event received.") {
  return safeMessage(value, fallback);
}

function safeUserFacingError(value: unknown, fallback: string) {
  return safeMessage(toUserFacingLocalProviderError(String(value ?? fallback)), fallback);
}

function redactSensitiveText(text: string) {
  return text
    .replace(/eyJ[A-Za-z0-9_-]{10,}\.[A-Za-z0-9_-]{10,}\.[A-Za-z0-9_-]{10,}/g, "<redacted>")
    .replace(/authorization\s*[:=]\s*bearer\s+[^\s,"'}]+/gi, "authorization: bearer <redacted>")
    .replace(/bearer\s+[A-Za-z0-9._-]{16,}/gi, "bearer <redacted>")
    .replace(/[A-Za-z0-9_-]{48,}/g, "<redacted>");
}

export function eventNeedsRuntimeRefresh(event: EventEnvelope) {
  return REFRESH_EVENT_TYPES.has(event.type);
}

export function isTaskLifecycleEvent(event: EventEnvelope) {
  return TASK_EVENT_TYPES.has(event.type);
}

export function taskIdFromEvent(event: EventEnvelope) {
  return safeToken(event.payload?.task_id);
}

export function taskIdFromSubmitResponse(response: CommandSubmitResponse | null | undefined) {
  return safeToken(response?.task_id || response?.immediate_state?.task_id);
}

export function formatProviderDiagnostic(payload: ProviderDiagnosticPayload | undefined) {
  const parts: string[] = [];
  const provider = safeToken(payload?.provider);
  const code = safeToken(payload?.error_code || payload?.code || payload?.error_category || payload?.category);
  const host = safeHost(payload?.base_url_host);
  const endpoint = safePath(payload?.endpoint_path);
  const httpStatus = safeStatus(payload?.http_status);
  const networkClass = safeToken(payload?.network_class);
  const providerClass = safeToken(payload?.provider_error_class);

  if (provider) parts.push(`provider ${provider}`);
  if (code) parts.push(`code ${code}`);
  if (providerClass) parts.push(`class ${providerClass}`);
  if (httpStatus) parts.push(`HTTP ${httpStatus}`);
  if (networkClass) parts.push(`network ${networkClass}`);
  if (host) parts.push(`host ${host}`);
  if (endpoint) parts.push(`endpoint ${endpoint}`);

  return parts.length ? `Provider diagnostic: ${parts.join(" · ")}` : "Provider diagnostic received.";
}

export function formatRuntimeEventForUser(event: EventEnvelope) {
  switch (event.type) {
    case "task.accepted":
      return "✓ Task accepted · waiting for provider";
    case "task.completed":
      return `✓ Done: ${safeMessage(event.payload?.summary, "Task complete")}`;
    case "task.stuck":
      return `⚠ Stuck: ${safeUserFacingError(event.payload?.reason, "Unknown error")}`;
    case "runtime.error":
      return `⚠ Runtime: ${safeUserFacingError(event.payload?.message, "Unknown runtime error")}`;
    case "local_provider.diagnostic":
      return formatProviderDiagnostic(event.payload as ProviderDiagnosticPayload);
    case "task.stale_active_recovered":
      return "Recovered stale task state. Retry is available.";
    case "report.ready":
      return "📜 Report ready";
    case "task.cancelled":
      return "Task cancelled.";
    default:
      return event.type;
  }
}

export function safeEventText(event: EventEnvelope) {
  if (event.type === "local_provider.diagnostic") {
    return formatProviderDiagnostic(event.payload as ProviderDiagnosticPayload);
  }
  if (event.type.startsWith("task.") || event.type === "runtime.error") {
    return formatRuntimeEventForUser(event);
  }
  const candidate = event.payload?.report_text || event.payload?.text || event.payload?.message || event.payload?.summary || event.payload?.reason;
  return safeDisplayText(candidate);
}

export function firstTaskAccessMessage(readyStatus: SidecarReadyStatus | null) {
  if (!readyStatus) {
    return "Checking local first-task access...";
  }
  if (!readyStatus.ok) {
    return "Local sidecar is not ready yet.";
  }
  if (readyStatus.first_task_eligible && readyStatus.local_llm_turn_allowed) {
    return "";
  }
  if (!readyStatus.bring_your_own_key_active) {
    return "Missing local BYOK provider config. Choose Use my own API key in setup.";
  }
  return "Local API key access is refreshing. Try again in a moment.";
}

export function firstTaskErrorMessage(rawMessage: string) {
  return toUserFacingLocalProviderError(rawMessage);
}

export function submitResponseMessage(response: CommandSubmitResponse) {
  if (response.kind === "movement") {
    return response.handled ? "Movement accepted · Milo is on the way" : "Movement request submitted";
  }
  if (response.kind === "task") {
    const intakeProof = response.intake_gate ? " · intake checked" : "";
    return response.task_id || response.immediate_state?.task_id
      ? `Task accepted${intakeProof} · waiting for provider`
      : `Task submitted${intakeProof} · waiting for provider`;
  }
  return response.handled ? "Submitted to Milo" : "Submitted · checking task state";
}

export function latestRelevantTaskEvent(events: EventEnvelope[], taskId: string) {
  for (let index = events.length - 1; index >= 0; index -= 1) {
    const event = events[index];
    if (!isTaskLifecycleEvent(event)) continue;
    const eventTaskId = taskIdFromEvent(event);
    if (!taskId) return event;
    if (eventTaskId && eventTaskId === taskId) return event;
  }
  return null;
}

export function formatActiveTaskStatus(task: TaskSnapshot | null | undefined) {
  if (!task) return "";
  switch (task.status) {
    case "running":
    case "accepted":
      return "Task active · waiting for provider";
    case "stuck":
      return `⚠ Stuck: ${safeUserFacingError(task.stuck_reason, "Unknown error")}`;
    case "completed":
      return "✓ Task complete";
    default:
      return `Task ${safeToken(task.status, "active")}`;
  }
}

export function safeLogRuntimeEvent(event: EventEnvelope) {
  const fields = [
    "event_received=true",
    `type=${safeToken(event.type, "unknown")}`,
  ];
  const taskId = taskIdFromEvent(event);
  const diagnostic = event.type === "local_provider.diagnostic" ? (event.payload as ProviderDiagnosticPayload) : null;
  const provider = safeToken(diagnostic?.provider);
  const httpStatus = safeStatus(diagnostic?.http_status);
  const networkClass = safeToken(diagnostic?.network_class);
  const providerClass = safeToken(diagnostic?.provider_error_class);
  const code = safeToken(diagnostic?.error_code || diagnostic?.code || event.payload?.error_code);

  if (taskId) fields.push(`task_id=${taskId}`);
  if (provider) fields.push(`provider=${provider}`);
  if (httpStatus) fields.push(`http_status=${httpStatus}`);
  if (networkClass) fields.push(`network_class=${networkClass}`);
  if (providerClass) fields.push(`provider_error_class=${providerClass}`);
  if (code) fields.push(`code=${code}`);

  console.log(`[XMILO_RUNTIME_EVENT] ${fields.join(" ")}`);
}

export function safeLogRuntimeRefresh(task: TaskSnapshot | null | undefined) {
  const fields = ["task_current_refreshed=true"];
  const taskId = safeToken(task?.task_id);
  const status = safeToken(task?.status);
  if (taskId) fields.push(`active_task_id=${taskId}`);
  fields.push(`active_task_present=${task ? "true" : "false"}`);
  if (status) fields.push(`active_task_status=${status}`);
  console.log(`[XMILO_RUNTIME_STATE] ${fields.join(" ")}`);
}

export function safeLogSubmitResponse(response: CommandSubmitResponse) {
  const fields = [
    "submit_response=true",
    `handled=${response.handled ? "true" : "false"}`,
    `kind=${safeToken(response.kind, "unknown")}`,
  ];
  const taskId = taskIdFromSubmitResponse(response);
  if (taskId) fields.push(`task_id=${taskId}`);
  console.log(`[XMILO_SUBMIT] ${fields.join(" ")}`);
}
