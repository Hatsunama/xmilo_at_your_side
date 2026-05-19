jest.mock("react-native", () => ({
  NativeModules: {},
  Platform: { OS: "android" }
}));

jest.mock("./config", () => ({
  LOCALHOST_TOKEN: "",
  SIDECAR_BASE_URL: "http://127.0.0.1:42817"
}));

import { isSetupPermissionFinalReady, type SetupPermissionSnapshot } from "./xmiloRuntimeHost";

function snapshot(overrides: Partial<SetupPermissionSnapshot> = {}): SetupPermissionSnapshot {
  const base: SetupPermissionSnapshot = {
    schema_version: 1,
    generated_at_millis: 1,
    rows: [
      { row: "camera", complete: true, required: true, status: "complete", allowed_action: "none" },
      { row: "microphone", complete: true, required: true, status: "complete", allowed_action: "none" },
      { row: "media", complete: true, required: true, status: "complete", allowed_action: "none" },
      { row: "location", complete: true, required: true, status: "complete", allowed_action: "none" },
      { row: "physical_activity", complete: true, required: true, status: "complete", allowed_action: "none" },
      { row: "foreground_runtime", complete: true, required: true, status: "complete", allowed_action: "none" }
    ],
    gate: {
      permissions_complete: true,
      foreground_runtime_complete: true,
      review_required: true,
      final_ready_without_review: true,
      blocked_reasons: []
    },
    blocked_reasons: [],
    allowed_actions: []
  };
  return { ...base, ...overrides };
}

describe("setup permission snapshot gate", () => {
  it("requires native final readiness and UI review", () => {
    expect(isSetupPermissionFinalReady(snapshot(), true)).toBe(true);
    expect(isSetupPermissionFinalReady(snapshot(), false)).toBe(false);
    expect(isSetupPermissionFinalReady(snapshot({ gate: { ...snapshot().gate, final_ready_without_review: false } }), true)).toBe(false);
  });

  it("blocks when any native required row is incomplete", () => {
    const blocked = snapshot({
      rows: snapshot().rows.map((row) => row.row === "camera" ? { ...row, complete: false, status: "unknown", allowed_action: "open_settings" } : row)
    });
    expect(isSetupPermissionFinalReady(blocked, true)).toBe(false);
  });

  it("blocks when Physical activity is incomplete", () => {
    const blocked = snapshot({
      rows: snapshot().rows.map((row) => row.row === "physical_activity" ? { ...row, complete: false, status: "denied", allowed_action: "request" } : row)
    });
    expect(isSetupPermissionFinalReady(blocked, true)).toBe(false);
  });

  it("blocks when foreground runtime is incomplete even if permission gate is true", () => {
    const blocked = snapshot({
      rows: snapshot().rows.map((row) => row.row === "foreground_runtime" ? { ...row, complete: false, status: "missing", allowed_action: "request" } : row),
      gate: { ...snapshot().gate, permissions_complete: true, foreground_runtime_complete: false, final_ready_without_review: false }
    });
    expect(blocked.gate.permissions_complete).toBe(true);
    expect(blocked.gate.foreground_runtime_complete).toBe(false);
    expect(blocked.gate.final_ready_without_review).toBe(false);
    expect(isSetupPermissionFinalReady(blocked, true)).toBe(false);
  });

  it("blocks when final native readiness is false even if rows are complete", () => {
    const blocked = snapshot({
      gate: { ...snapshot().gate, final_ready_without_review: false }
    });
    expect(isSetupPermissionFinalReady(blocked, true)).toBe(false);
  });

  it("does not let row-level display state bypass native final readiness", () => {
    const blocked = snapshot({
      rows: snapshot().rows.map((row) => row.row === "camera" ? { ...row, complete: true, status: "complete" } : row),
      gate: { ...snapshot().gate, permissions_complete: false, final_ready_without_review: false }
    });
    expect(isSetupPermissionFinalReady(blocked, true)).toBe(false);
  });
});
