const mockDb = {
  execAsync: jest.fn(async () => undefined),
  runAsync: jest.fn(async () => undefined),
  getAllAsync: jest.fn(async () => []),
};

jest.mock("expo-sqlite", () => ({
  openDatabaseAsync: jest.fn(async () => mockDb),
}));

jest.mock("./runtimeEvents", () => ({
  firstTaskErrorMessage: (value: string) => value,
}));

import { appendLairChatMessage } from "./lairDailyChat";

describe("lair daily chat local storage boundary", () => {
  beforeEach(() => {
    jest.clearAllMocks();
  });

  it("keeps local user-owned chat rows instead of deleting them when text contains a synthetic secret", async () => {
    const saved = await appendLairChatMessage(
      {
        id: "local-secret-row",
        role: "user",
        text: "local user-owned note password: user-provided-password with useful local context",
        created_at: "2026-05-31T12:00:00.000Z",
      },
      "2026-05-31"
    );

    expect(saved).not.toBeNull();
    expect(saved?.text).toContain("useful local context");
    expect(mockDb.runAsync).toHaveBeenCalledWith(
      expect.stringContaining("INSERT OR IGNORE INTO lair_daily_chat_messages"),
      "local-secret-row",
      "2026-05-31",
      "user",
      expect.stringContaining("useful local context"),
      null,
      null,
      null,
      "2026-05-31T12:00:00.000Z"
    );
  });
});
