const mockListTodayLairChatMessages = jest.fn();

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
  listTodayLairChatMessages: mockListTodayLairChatMessages,
}));

jest.mock("./runtimeEvents", () => ({
  safeEventText: (event: { payload?: Record<string, unknown> }) =>
    String(event.payload?.message ?? event.payload?.summary ?? event.payload?.text ?? ""),
}));

import { buildSettingsReportBundle, redactSensitiveText } from "./reportBundle";
import type { RuntimeState } from "../types/contracts";

const rawSecrets = [
  "api_key=sk-user-provided-value",
  "sk-user-provided-value",
  "Authorization: Bearer user-provided-token",
  "user-provided-token",
  "X-API-Key: sk-user-provided-value",
  "API key is sk-user-provided-value",
  "token: user-provided-token",
  "password: user-provided-password",
  "secret: user-provided-secret",
];
const rawSecretForms = rawSecrets.filter((secret) => secret !== "sk-user-provided-value" && secret !== "user-provided-token");

function runtimeState(): RuntimeState {
  return {
    milo_state: "awake",
    current_room_id: "lair",
    current_anchor_id: "desk",
    last_meaningful_user_action_at: "2026-05-31T12:00:00.000Z",
    runtime_id: "runtime-test",
    active_task: {
      task_id: "task-1",
      attempt_id: "attempt-1",
      prompt: "please use password: user-provided-password",
      intent: "report diagnostics",
      room_id: "lair",
      anchor_id: "desk",
      status: "running",
      started_at: "2026-05-31T12:00:00.000Z",
      updated_at: "2026-05-31T12:00:01.000Z",
      retry_count: 0,
      max_retries: 1,
    },
    queued_task: {
      task_id: "task-2",
      attempt_id: "attempt-2",
      prompt: "queued secret: user-provided-secret",
      intent: "next diagnostics",
      room_id: "lair",
      anchor_id: "desk",
      status: "queued",
      started_at: "2026-05-31T12:00:00.000Z",
      updated_at: "2026-05-31T12:00:01.000Z",
      retry_count: 0,
      max_retries: 1,
    },
  };
}

describe("report bundle redaction", () => {
  beforeEach(() => {
    mockListTodayLairChatMessages.mockReset();
  });

  it("redacts current-turn synthetic secrets from report-bound text", () => {
    for (const secret of rawSecretForms) {
      const redacted = redactSensitiveText(`useful context ${secret}`);
      expect(redacted).toContain("useful context");
      expect(redacted).not.toContain(secret);
    }
  });

  it("redacts user notes, daily chat excerpts, runtime event summaries, and prompt-like runtime state", async () => {
    mockListTodayLairChatMessages.mockResolvedValue([
      {
        id: "chat-1",
        chat_date: "2026-05-31",
        role: "user",
        text: "user saw Authorization: Bearer user-provided-token during useful setup context",
        task_id: null,
        attempt_id: null,
        source_event_type: null,
        created_at: "2026-05-31T12:00:00.000Z",
      },
    ]);

    const payload = await buildSettingsReportBundle({
      userNote: "report note api_key=sk-user-provided-value with useful non-secret context",
      runtimeState: runtimeState(),
      events: [
        {
          type: "runtime.error",
          timestamp: "2026-05-31T12:00:02.000Z",
          source: "ui_local",
          payload: {
            message: "runtime saw X-API-Key: sk-user-provided-value while checking useful diagnostics",
          },
        },
      ],
      clientReportId: "settings_test",
      createdAt: "2026-05-31T12:00:03.000Z",
    });

    const serialized = JSON.stringify(payload);
    for (const secret of rawSecrets) {
      expect(serialized).not.toContain(secret);
    }
    expect(serialized).not.toContain("please use password");
    expect(serialized).not.toContain("queued secret");
    expect(serialized).toContain("useful non-secret context");
    expect(serialized).toContain("useful setup context");
    expect(serialized).toContain("useful diagnostics");
  });
});
