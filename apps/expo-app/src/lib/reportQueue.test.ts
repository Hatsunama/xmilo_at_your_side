const mockWrittenFiles: Record<string, string> = {};
const mockDb = {
  execAsync: jest.fn(async () => undefined),
  runAsync: jest.fn(async () => undefined),
  getAllAsync: jest.fn(async (query: string) => {
    if (query.startsWith("PRAGMA")) return [];
    return [];
  }),
  getFirstAsync: jest.fn(async () => ({ count: 0 })),
};

jest.mock("expo-file-system/legacy", () => ({
  documentDirectory: "file://document/",
  EncodingType: { UTF8: "utf8" },
  makeDirectoryAsync: jest.fn(async () => undefined),
  writeAsStringAsync: jest.fn(async (uri: string, content: string) => {
    mockWrittenFiles[uri] = content;
  }),
  readAsStringAsync: jest.fn(async (uri: string) => mockWrittenFiles[uri] ?? ""),
}));

jest.mock("expo-sqlite", () => ({
  openDatabaseAsync: jest.fn(async () => mockDb),
}));

jest.mock("expo-constants", () => ({
  __esModule: true,
  default: {
    expoConfig: { version: "test-version" },
    nativeAppVersion: "native-test-version",
  },
}));

jest.mock("expo-device", () => ({
  osName: "Android",
  osVersion: "15",
  modelName: "Pixel Test",
}));

jest.mock("./lairDailyChat", () => ({
  localChatDate: () => "2026-05-31",
  listTodayLairChatMessages: jest.fn(async () => []),
}));

jest.mock("./runtimeEvents", () => ({
  safeEventText: (event: { payload?: Record<string, unknown> }) =>
    String(event.payload?.message ?? event.payload?.summary ?? event.payload?.text ?? ""),
}));

jest.mock("./bridge", () => ({
  submitSettingsReport: jest.fn(async () => ({ accepted: true, report_id: "relay-report-1" })),
}));

import { submitSettingsReport } from "./bridge";
import { drainQueuedSettingsReports, enqueueSettingsReport } from "./reportQueue";
import type { SettingsReportSubmitPayload } from "./reportBundle";

const rawSecrets = [
  "api_key=sk-user-provided-value",
  "sk-user-provided-value",
  "Authorization: Bearer user-provided-token",
  "user-provided-token",
  "X-API-Key: sk-user-provided-value",
  "API key is sk-user-provided-value",
  "password: user-provided-password",
  "secret: user-provided-secret",
];

function rawPayload(): SettingsReportSubmitPayload {
  return {
    client_report_id: "settings_queue_test",
    created_at: "2026-05-31T12:00:00.000Z",
    bundle_schema_version: 1,
    bundle_type: "settings_report",
    bundle_hash: "fnv1a64:test",
    app_version: "test",
    runtime_id: "runtime-test",
    report_reason: "settings_issue",
    user_note: "queue note api_key=sk-user-provided-value with useful queue context",
    bundle: {
      schema_version: 1,
      client_report_id: "settings_queue_test",
      created_at: "2026-05-31T12:00:00.000Z",
      bundle_schema_version: 1,
      bundle_type: "settings_report",
      report_source: "settings_button",
      report_reason: "settings_issue",
      user_note: "API key is sk-user-provided-value but useful note remains",
      app_runtime_metadata: {
        app_version: "test",
        app_ownership: "expo_app",
        runtime_id: "runtime-test",
        platform: "Android",
        os_name: "Android",
        os_version: "15",
        device_model: "Pixel Test",
      },
      today_conversation: {
        source: "lair_daily_chat",
        availability: "included",
        chat_date: "2026-05-31",
        message_count: 1,
        messages: [
          {
            role: "user",
            text: "daily chat Authorization: Bearer user-provided-token with useful daily context",
            task_id: null,
            attempt_id: null,
            source_event_type: null,
            created_at: "2026-05-31T12:00:00.000Z",
          },
        ],
      },
      recent_runtime_events: {
        source: "app_context_events",
        availability: "included",
        event_count: 1,
        events: [
          {
            type: "runtime.error",
            timestamp: "2026-05-31T12:00:01.000Z",
            source: "ui_local",
            summary: "runtime X-API-Key: sk-user-provided-value with useful runtime context",
          },
        ],
      },
      runtime_state_summary: {
        source: "app_context_state",
        availability: "included",
        runtime_id: "runtime-test",
        milo_state: "awake",
        current_room_id: "lair",
        current_anchor_id: "desk",
        active_task: {
          present: true,
          task_id: "task-1",
          attempt_id: "attempt-1",
          status: "running",
          intent: "password: user-provided-password",
          room_id: "lair",
          anchor_id: "desk",
          retry_count: 0,
        },
        queued_task: {
          present: true,
          task_id: "task-2",
          status: "secret: user-provided-secret",
        },
      },
      source_availability: {
        today_conversation: "included",
        recent_runtime_events: "included",
        runtime_state_summary: "included",
      },
      redaction_summary: {
        version: "phase15_app_local_redactor_v1",
        user_note_redacted: true,
        conversation_redacted: true,
        runtime_events_redacted: true,
        stored_credentials: "not_collected_in_this_build",
        authorization_headers: "not_collected_in_this_build",
        provider_configuration: "not_collected_in_this_build",
        system_instructions: "not_collected_in_this_build",
        tool_payload_content: "not_collected_in_this_build",
        backend_provider_bodies: "not_collected_in_this_build",
      },
    },
  };
}

describe("settings report queue redaction", () => {
  beforeEach(() => {
    for (const key of Object.keys(mockWrittenFiles)) delete mockWrittenFiles[key];
    jest.clearAllMocks();
  });

  it("persists a redacted queued payload while preserving useful non-secret context", async () => {
    const queued = await enqueueSettingsReport({ payload: rawPayload(), lastError: "offline" });
    const written = mockWrittenFiles[queued.bundle_uri];

    expect(written).toBeTruthy();
    for (const secret of rawSecrets) {
      expect(written).not.toContain(secret);
    }
    expect(written).toContain("useful queue context");
    expect(written).toContain("useful daily context");
    expect(written).toContain("useful runtime context");
  });

  it("submits the redacted queued payload when draining retry reports", async () => {
    const queued = await enqueueSettingsReport({ payload: rawPayload(), lastError: "offline" });
    (mockDb.getAllAsync as jest.Mock).mockImplementation(async (query: string) => {
      if (query.startsWith("PRAGMA")) return [];
      return [queued];
    });

    await drainQueuedSettingsReports({ limit: 1 });

    expect(submitSettingsReport).toHaveBeenCalledTimes(1);
    const submitted = JSON.stringify((submitSettingsReport as jest.Mock).mock.calls[0][0]);
    for (const secret of rawSecrets) {
      expect(submitted).not.toContain(secret);
    }
    expect(submitted).toContain("useful queue context");
  });
});
