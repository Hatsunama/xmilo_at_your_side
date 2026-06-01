jest.mock("react-native", () => ({
  NativeModules: {},
  Platform: { OS: "android" }
}));

jest.mock("./config", () => ({
  LOCALHOST_TOKEN: "",
  SIDECAR_BASE_URL: "http://127.0.0.1:42817"
}));

import {
  getBatteryBackgroundSetupStatus,
  getRuntimeSetupMissing,
  isRuntimeReadyForSetup,
  isSetupPermissionFinalReady,
  type SetupPermissionSnapshot,
  type XMiloclawRuntimeStatus
} from "./xmiloRuntimeHost";
import fs from "fs";
import path from "path";

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

function runtimeStatus(overrides: Partial<XMiloclawRuntimeStatus> = {}): XMiloclawRuntimeStatus {
  return {
    notificationsGranted: true,
    appearOnTopGranted: true,
    batteryUnrestricted: false,
    dataSaverUnrestricted: true,
    dataSaverStatus: "disabled",
    accessibilityEnabled: true,
    runtimeHostStarted: true,
    foregroundServiceStarted: true,
    runtimeFilesPrepared: true,
    sidecarProcessLaunched: true,
    sidecarProcessAlive: true,
    bridgeConnected: true,
    taskRouteSurfaceReady: true,
    hostReady: false,
    ...overrides
  };
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

describe("setup battery background compatibility", () => {
  it("keeps battery setup status limited to implemented native proof states", () => {
    const runtimeHostSource = fs.readFileSync(path.join(__dirname, "./xmiloRuntimeHost.ts"), "utf8");
    expect(runtimeHostSource).toContain('"verified_unrestricted"');
    expect(runtimeHostSource).toContain('"settings_available_unverified"');
    expect(runtimeHostSource).not.toContain("background_usage_allowed");
  });

  it("allows Seeker-style fallback when exact unrestricted state is not verified", () => {
    const status = runtimeStatus({ batteryUnrestricted: false, hostReady: false });
    const setupSource = fs.readFileSync(path.join(__dirname, "../../app/setup.tsx"), "utf8");
    expect(getBatteryBackgroundSetupStatus(status)).toBe("settings_available_unverified");
    expect(isRuntimeReadyForSetup(status)).toBe(true);
    expect(getRuntimeSetupMissing(status)).toEqual([]);
    expect(setupSource).toContain('return getBatteryBackgroundSetupStatus(status) === "verified_unrestricted" ? "granted" : "accepted";');
    expect(setupSource).toContain('if (id === "unrestricted_battery_access") return status === "granted" || status === "accepted";');
  });

  it("keeps verified unrestricted as the clean pass state", () => {
    const status = runtimeStatus({ batteryUnrestricted: true, hostReady: true });
    expect(getBatteryBackgroundSetupStatus(status)).toBe("verified_unrestricted");
    expect(isRuntimeReadyForSetup(status)).toBe(true);
  });

  it("does not claim background usage is allowed without an explicit native field", () => {
    const status = runtimeStatus({ batteryUnrestricted: false, hostReady: false });
    const setupSource = fs.readFileSync(path.join(__dirname, "../../app/setup.tsx"), "utf8");
    expect(getBatteryBackgroundSetupStatus(status)).toBe("settings_available_unverified");
    expect(setupSource).toContain("Battery controls vary by device. xMilo accepted this device's available battery configuration.");
    expect(setupSource).not.toContain("Background usage is allowed on this device");
  });

  it("treats accepted unverified battery state as pass styling, not warning styling", () => {
    const setupSource = fs.readFileSync(path.join(__dirname, "../../app/setup.tsx"), "utf8");
    expect(setupSource).toContain('const acceptedBattery = key === "unrestricted_battery_access" && accepted;');
    expect(setupSource).toContain("const visuallyAccepted = granted || acceptedBattery;");
    expect(setupSource).toContain('status === "granted" || (row.id === "unrestricted_battery_access" && status === "accepted")');
    expect(setupSource).toContain('row.status !== "granted" && row.status !== "accepted"');
    expect(setupSource).not.toContain('return getBatteryBackgroundSetupStatus(status) === "verified_unrestricted" ? "granted" : "warning";');
    expect(setupSource).not.toContain('row.id === "unrestricted_battery_access" && status === "warning"');
  });

  it("uses truthful fallback copy without requiring an Unrestricted label", () => {
    const setupSource = fs.readFileSync(path.join(__dirname, "../../app/setup.tsx"), "utf8");
    expect(setupSource).toContain("Android and OEM battery controls vary by device.");
    expect(setupSource).not.toContain("Missing permission");
    expect(setupSource).not.toContain("Problem detected");
    expect(setupSource).not.toContain("Required action");
    expect(setupSource).not.toContain("Unrestricted battery access");
    expect(setupSource).not.toContain("App info → Battery → Unrestricted");
    expect(setupSource).not.toMatch(/fully guaranteed background|fully verified unrestricted|guaranteed background|unrestricted granted/i);
  });

  it("keeps explicitly missing runtime prerequisites blocking without using hostReady as a battery proxy", () => {
    expect(isRuntimeReadyForSetup(runtimeStatus({ batteryUnrestricted: false, hostReady: false, sidecarProcessAlive: false }))).toBe(false);
    expect(getRuntimeSetupMissing(runtimeStatus({ sidecarProcessAlive: false }))).toContain("Sidecar process alive");
    expect(isRuntimeReadyForSetup(runtimeStatus({ batteryUnrestricted: false, hostReady: false, bridgeConnected: false }))).toBe(false);
    expect(getRuntimeSetupMissing(runtimeStatus({ bridgeConnected: false }))).toContain("Bridge connected");
    expect(isRuntimeReadyForSetup(runtimeStatus({ batteryUnrestricted: false, hostReady: false, taskRouteSurfaceReady: false }))).toBe(false);
    expect(getRuntimeSetupMissing(runtimeStatus({ taskRouteSurfaceReady: false }))).toContain("Task route surface ready");
  });

  it("keeps BYOK setup readiness off native hostReady while preserving BYOK checks", () => {
    const setupSource = fs.readFileSync(path.join(__dirname, "../../app/setup.tsx"), "utf8");
    const byokBlock = setupSource.slice(
      setupSource.indexOf("async function waitForByokReadinessAfterSave"),
      setupSource.indexOf("const runFinalRuntimeConfirmation")
    );
    expect(isRuntimeReadyForSetup(runtimeStatus({ batteryUnrestricted: false, hostReady: false }))).toBe(true);
    expect(byokBlock).toContain("isRuntimeReadyForSetup(ready)");
    expect(byokBlock).not.toContain("ready.hostReady === true");
    expect(byokBlock).toContain('ready.taskRouteSurfaceReady === true');
    expect(byokBlock).toContain('ready.llmMode === "local_byok"');
    expect(byokBlock).toContain("ready.byokProvider === resolved.provider");
    expect(byokBlock).toContain("ready.bringYourOwnKeyActive === true");
    expect(byokBlock).toContain("ready.localLlmTurnAllowed === true");
    expect(byokBlock).toContain("ready.firstTaskEligible === true");
  });

  it("keeps setup runtime display off native hostReady", () => {
    const setupSource = fs.readFileSync(path.join(__dirname, "../../app/setup.tsx"), "utf8");
    const prepareBlock = setupSource.slice(
      setupSource.indexOf('runtimeStep === "prepare_hidden_runtime_host"'),
      setupSource.indexOf('runtimeStep === "return_to_xmilo"')
    );
    const recheckBlock = setupSource.slice(
      setupSource.indexOf('runtimeStep === "re_check"'),
      setupSource.indexOf("Stage: {runtimeProof.lastRuntimeStage")
    );
    expect(isRuntimeReadyForSetup(runtimeStatus({ batteryUnrestricted: false, hostReady: false }))).toBe(true);
    expect(prepareBlock).toContain("Runtime ready for setup");
    expect(prepareBlock).toContain("isRuntimeReadyForSetup(runtimeProof)");
    expect(prepareBlock).not.toContain("runtimeProof.hostReady");
    expect(recheckBlock).toContain("Runtime ready for setup");
    expect(recheckBlock).toContain("isRuntimeReadyForSetup(runtimeProof)");
    expect(recheckBlock).not.toContain("runtimeProof.hostReady");
  });

  it("keeps battery setup copy free of fake unrestricted or guaranteed background claims", () => {
    const setupSource = fs.readFileSync(path.join(__dirname, "../../app/setup.tsx"), "utf8");
    expect(setupSource).not.toMatch(/unrestricted granted|guaranteed background work|background work guaranteed|guaranteed background/i);
  });
});
