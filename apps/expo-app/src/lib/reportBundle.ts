import Constants from "expo-constants";
import * as Device from "expo-device";
import type { EventEnvelope, RuntimeState } from "../types/contracts";
import { listTodayLairChatMessages, localChatDate } from "./lairDailyChat";
import { safeEventText } from "./runtimeEvents";

export type SettingsReportBundle = {
  schema_version: 1;
  client_report_id: string;
  created_at: string;
  bundle_schema_version: 1;
  bundle_type: "settings_report";
  report_source: "settings_button";
  report_reason: "settings_issue";
  user_note: string;
  app_runtime_metadata: {
    app_version: string;
    app_ownership: "expo_app";
    runtime_id: string;
    platform: string;
    os_name: string;
    os_version: string;
    device_model: string;
  };
  today_conversation: {
    source: "lair_daily_chat";
    availability: "included" | "source_unavailable" | "not_collected_in_this_build";
    chat_date: string;
    message_count: number;
    messages: Array<{
      role: string;
      text: string;
      task_id: string | null;
      attempt_id: string | null;
      source_event_type: string | null;
      created_at: string;
    }>;
  };
  recent_runtime_events: {
    source: "app_context_events";
    availability: "included" | "source_unavailable" | "not_collected_in_this_build";
    event_count: number;
    events: Array<{
      type: string;
      timestamp: string;
      source: string;
      summary: string;
    }>;
  };
  runtime_state_summary: {
    source: "app_context_state";
    availability: "included" | "source_unavailable";
    runtime_id: string;
    milo_state: string;
    current_room_id: string;
    current_anchor_id: string;
    active_task: {
      present: boolean;
      task_id: string;
      attempt_id: string;
      status: string;
      intent: string;
      room_id: string;
      anchor_id: string;
      retry_count: number | null;
    };
    queued_task: {
      present: boolean;
      task_id: string;
      status: string;
    };
  };
  source_availability: Record<string, "included" | "source_unavailable" | "not_collected_in_this_build">;
  redaction_summary: {
    version: "phase15_app_local_redactor_v1";
    user_note_redacted: boolean;
    conversation_redacted: boolean;
    runtime_events_redacted: boolean;
    stored_credentials: "not_collected_in_this_build";
    authorization_headers: "not_collected_in_this_build";
    provider_configuration: "not_collected_in_this_build";
    system_instructions: "not_collected_in_this_build";
    tool_payload_content: "not_collected_in_this_build";
    backend_provider_bodies: "not_collected_in_this_build";
  };
};

export type SettingsReportSubmitPayload = {
  client_report_id: string;
  created_at: string;
  bundle_schema_version: 1;
  bundle_type: "settings_report";
  bundle_hash: string;
  app_version: string;
  runtime_id: string;
  report_reason: "settings_issue";
  user_note: string;
  bundle: SettingsReportBundle;
};

export async function buildSettingsReportBundle(input: {
  userNote: string;
  runtimeState: RuntimeState;
  events: EventEnvelope[];
  clientReportId?: string;
  createdAt?: string;
}): Promise<SettingsReportSubmitPayload> {
  const createdAt = input.createdAt ?? new Date().toISOString();
  const clientReportId = input.clientReportId ?? createClientReportId(createdAt);
  const userNote = redactSensitiveText(input.userNote).slice(0, 1200);
  const appVersion = safeToken(Constants.expoConfig?.version ?? Constants.nativeAppVersion ?? "unknown", "unknown");
  const chatDate = localChatDate();
  const conversation = await collectTodayConversation(chatDate);
  const runtimeEvents = collectRecentRuntimeEvents(input.events);
  const runtimeStateSummary = summarizeRuntimeState(input.runtimeState);

  const bundle: SettingsReportBundle = {
    schema_version: 1,
    client_report_id: clientReportId,
    created_at: createdAt,
    bundle_schema_version: 1,
    bundle_type: "settings_report",
    report_source: "settings_button",
    report_reason: "settings_issue",
    user_note: userNote,
    app_runtime_metadata: {
      app_version: appVersion,
      app_ownership: "expo_app",
      runtime_id: safeToken(input.runtimeState.runtime_id, "unknown"),
      platform: safeToken(Device.osName || "unknown", "unknown"),
      os_name: safeToken(Device.osName || "unknown", "unknown"),
      os_version: safeToken(Device.osVersion || "unknown", "unknown"),
      device_model: redactSensitiveText(Device.modelName || "unknown").slice(0, 120),
    },
    today_conversation: conversation,
    recent_runtime_events: runtimeEvents,
    runtime_state_summary: runtimeStateSummary,
    source_availability: {
      today_conversation: conversation.availability,
      recent_runtime_events: runtimeEvents.availability,
      runtime_state_summary: runtimeStateSummary.availability,
      app_runtime_logs: "not_collected_in_this_build",
      task_diagnostics: runtimeEvents.availability,
    },
    redaction_summary: {
      version: "phase15_app_local_redactor_v1",
      user_note_redacted: userNote !== input.userNote,
      conversation_redacted: true,
      runtime_events_redacted: true,
      stored_credentials: "not_collected_in_this_build",
      authorization_headers: "not_collected_in_this_build",
      provider_configuration: "not_collected_in_this_build",
      system_instructions: "not_collected_in_this_build",
      tool_payload_content: "not_collected_in_this_build",
      backend_provider_bodies: "not_collected_in_this_build",
    },
  };

  const bundleHash = hashSettingsReportBundle(bundle);
  return {
    client_report_id: clientReportId,
    created_at: createdAt,
    bundle_schema_version: 1,
    bundle_type: "settings_report",
    bundle_hash: bundleHash,
    app_version: appVersion,
    runtime_id: safeToken(input.runtimeState.runtime_id, ""),
    report_reason: "settings_issue",
    user_note: userNote,
    bundle,
  };
}

export function redactSensitiveText(value: unknown) {
  if (typeof value !== "string") return "";
  return value
    .replace(/sk-[A-Za-z0-9_-]{16,}/g, "<redacted>")
    .replace(/xai-[A-Za-z0-9_-]{16,}/gi, "<redacted>")
    .replace(/sk-ant-[A-Za-z0-9_-]{16,}/gi, "<redacted>")
    .replace(/anthropic[_-]?[A-Za-z0-9_-]{20,}/gi, "<redacted>")
    .replace(/eyJ[A-Za-z0-9_-]{10,}\.[A-Za-z0-9_-]{10,}\.[A-Za-z0-9_-]{10,}/g, "<redacted>")
    .replace(/authorization\s*[:=]\s*bearer\s+[^\s,"'}]+/gi, "authorization: bearer <redacted>")
    .replace(/bearer\s+[A-Za-z0-9._-]{16,}/gi, "bearer <redacted>")
    .replace(/\b(password|secret|token|api[_-]?key|provider[_-]?key)\s*[:=]\s*["']?[^"'\s,;]{4,}/gi, "$1=<redacted>")
    .replace(/\b[A-Za-z0-9_-]{64,}\b/g, "<redacted>")
    .trim();
}

export function hashSettingsReportBundle(bundle: SettingsReportBundle) {
  const serialized = stableStringify(bundle);
  let high = 0x811c9dc5;
  let low = 0x811c9dc5;
  for (let index = 0; index < serialized.length; index += 1) {
    const code = serialized.charCodeAt(index);
    high ^= code;
    low ^= code;
    high = Math.imul(high, 16777619);
    low = Math.imul(low ^ (high >>> 16), 2246822507);
  }
  return `fnv1a64:${unsignedHex(high)}${unsignedHex(low)}`;
}

function createClientReportId(createdAt: string) {
  const timestamp = createdAt.replace(/[^0-9A-Za-z]/g, "").slice(0, 20);
  const randomPart = Math.random().toString(36).slice(2, 12);
  return `settings_${timestamp}_${randomPart}`;
}

async function collectTodayConversation(chatDate: string): Promise<SettingsReportBundle["today_conversation"]> {
  try {
    const messages = await listTodayLairChatMessages(chatDate);
    return {
      source: "lair_daily_chat",
      availability: "included",
      chat_date: chatDate,
      message_count: messages.length,
      messages: messages.slice(-80).map((message) => ({
        role: safeToken(message.role, "unknown"),
        text: redactSensitiveText(message.text).replace(/\s+/g, " ").slice(0, 2000),
        task_id: safeNullableToken(message.task_id),
        attempt_id: safeNullableToken(message.attempt_id),
        source_event_type: safeNullableToken(message.source_event_type),
        created_at: safeToken(message.created_at, ""),
      })),
    };
  } catch {
    return {
      source: "lair_daily_chat",
      availability: "source_unavailable",
      chat_date: chatDate,
      message_count: 0,
      messages: [],
    };
  }
}

function collectRecentRuntimeEvents(events: EventEnvelope[]): SettingsReportBundle["recent_runtime_events"] {
  if (!Array.isArray(events)) {
    return { source: "app_context_events", availability: "source_unavailable", event_count: 0, events: [] };
  }
  const recentEvents = events.slice(-40).map((event) => ({
    type: safeToken(event.type, "unknown"),
    timestamp: safeToken(event.timestamp, ""),
    source: safeToken(event.source ?? "unknown", "unknown"),
    summary: redactSensitiveText(safeEventText(event)).replace(/\s+/g, " ").slice(0, 240),
  }));
  return {
    source: "app_context_events",
    availability: "included",
    event_count: recentEvents.length,
    events: recentEvents,
  };
}

function summarizeRuntimeState(state: RuntimeState): SettingsReportBundle["runtime_state_summary"] {
  const activeTask = state.active_task ?? null;
  const queuedTask = state.queued_task ?? null;
  return {
    source: "app_context_state",
    availability: "included",
    runtime_id: safeToken(state.runtime_id, "unknown"),
    milo_state: safeToken(state.milo_state, "unknown"),
    current_room_id: safeToken(state.current_room_id, "unknown"),
    current_anchor_id: safeToken(state.current_anchor_id, "unknown"),
    active_task: {
      present: Boolean(activeTask),
      task_id: safeToken(activeTask?.task_id, ""),
      attempt_id: safeToken(activeTask?.attempt_id, ""),
      status: safeToken(activeTask?.status, ""),
      intent: safeToken(activeTask?.intent, ""),
      room_id: safeToken(activeTask?.room_id, ""),
      anchor_id: safeToken(activeTask?.anchor_id, ""),
      retry_count: typeof activeTask?.retry_count === "number" ? activeTask.retry_count : null,
    },
    queued_task: {
      present: Boolean(queuedTask),
      task_id: safeToken(queuedTask?.task_id, ""),
      status: safeToken(queuedTask?.status, ""),
    },
  };
}

function safeNullableToken(value: unknown) {
  const token = safeToken(value, "");
  return token || null;
}

function safeToken(value: unknown, fallback: string) {
  if (typeof value !== "string" && typeof value !== "number" && typeof value !== "boolean") return fallback;
  const text = String(value).trim();
  if (!text) return fallback;
  return text.replace(/[^A-Za-z0-9_.:/-]/g, "_").slice(0, 160);
}

function stableStringify(value: unknown): string {
  if (value === null || typeof value !== "object") {
    return JSON.stringify(value);
  }
  if (Array.isArray(value)) {
    return `[${value.map((item) => stableStringify(item)).join(",")}]`;
  }
  const record = value as Record<string, unknown>;
  return `{${Object.keys(record)
    .sort()
    .map((key) => `${JSON.stringify(key)}:${stableStringify(record[key])}`)
    .join(",")}}`;
}

function unsignedHex(value: number) {
  return (value >>> 0).toString(16).padStart(8, "0");
}
