jest.mock("./bridge", () => ({
  toUserFacingLocalProviderError: (value: string) => value,
}));

import { safeDisplayText, safeEventText } from "./runtimeEvents";

const secretCases = [
  "api_key=sk-user-provided-value",
  "Authorization: Bearer user-provided-token",
  "X-API-Key: sk-user-provided-value",
  "API key is sk-user-provided-value",
  "token: user-provided-token",
  "password: user-provided-password",
  "secret: user-provided-secret",
];

describe("runtime event report-bound redaction", () => {
  it("redacts synthetic secret forms from event display text", () => {
    for (const secret of secretCases) {
      const text = safeDisplayText(`useful runtime context ${secret}`);
      expect(text).toContain("useful runtime context");
      expect(text).not.toContain(secret);
    }
  });

  it("redacts event summaries before report bundle inclusion", () => {
    const summary = safeEventText({
      type: "runtime.error",
      timestamp: "2026-05-31T12:00:00.000Z",
      source: "ui_local",
      payload: {
        message: "useful runtime diagnostics X-API-Key: sk-user-provided-value",
      },
    });

    expect(summary).toContain("useful runtime diagnostics");
    expect(summary).not.toContain("X-API-Key: sk-user-provided-value");
    expect(summary).not.toContain("sk-user-provided-value");
  });
});
