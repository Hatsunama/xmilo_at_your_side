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
import { OLLAMA_CONNECTION_MESSAGE, buildProviderDisplayRows, getProviderSetupCopy, getProviderSetupMode, isGuidedLocalServerProvider } from "./providerSetup";
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

describe("provider setup UX contract", () => {
  it("consumes sidecar provider connection fields in the app bridge contract", () => {
    const bridgeSource = fs.readFileSync(path.join(__dirname, "./bridge.ts"), "utf8");
    expect(bridgeSource).toContain("connection_kind");
    expect(bridgeSource).toContain("base_url_entry_mode");
    expect(bridgeSource).toContain("advanced_manual_base_url_allowed");
    expect(bridgeSource).toContain("requires_connection_target");
    expect(bridgeSource).toContain("requires_reachability_probe");
    expect(bridgeSource).toContain("setup_guidance_code");
    expect(bridgeSource).toContain("local_provider_connection_target_required");
    expect(bridgeSource).toContain("local_provider_manual_url_invalid");
  });

  it("keeps cloud setup on canonical provider defaults without normal endpoint entry", () => {
    const setupSource = fs.readFileSync(path.join(__dirname, "../../app/setup.tsx"), "utf8");
    const cloudCopy = getProviderSetupCopy({ id: "openai", label: "OpenAI / GPT", key_required: true });
    expect(setupSource).toContain("getProviderSetupCopy(providerSetupSource");
    expect(cloudCopy.body).toContain("xMilo will use the standard OpenAI / GPT endpoint automatically.");
    expect(cloudCopy.body).toContain("Paste your API key to continue.");
    expect(getProviderSetupCopy({ id: "ollama_cloud", label: "Ollama Cloud", key_required: true }).body).toContain("Paste your Ollama API key to continue.");
    expect(setupSource).not.toContain('placeholder="Base URL"');
    expect(setupSource).not.toContain("Base URL:");
  });

  it("uses guided Ollama setup with advanced manual server address fallback only", () => {
    const setupSource = fs.readFileSync(path.join(__dirname, "../../app/setup.tsx"), "utf8");
    const settingsSource = fs.readFileSync(path.join(__dirname, "../../app/settings.tsx"), "utf8");
    const helperSource = fs.readFileSync(path.join(__dirname, "./providerSetup.ts"), "utf8");
    expect(helperSource).toContain('base_url_entry_mode, "guided_local_server"');
    expect(helperSource).toContain('setup_guidance_code, "ollama_connection_target_required"');
    expect(getProviderSetupCopy({ id: "ollama", label: "Ollama Local" }).body).toBe(OLLAMA_CONNECTION_MESSAGE);
    expect(setupSource).toContain("Advanced: enter server address");
    expect(setupSource).toContain("server address");
    expect(settingsSource).toContain("Advanced: enter server address");
    expect(settingsSource).toContain("server address");
    expect(setupSource).not.toContain("API key (optional)");
    expect(settingsSource).not.toContain("Base URL, optional unless provider requires it");
  });

  it("groups Ollama provider choices before routing Cloud or Local setup", () => {
    const setupSource = fs.readFileSync(path.join(__dirname, "../../app/setup.tsx"), "utf8");
    const settingsSource = fs.readFileSync(path.join(__dirname, "../../app/settings.tsx"), "utf8");
    const helperSource = fs.readFileSync(path.join(__dirname, "./providerSetup.ts"), "utf8");
    const rows = buildProviderDisplayRows([
      { id: "openai", label: "OpenAI / GPT", default_model: "gpt-5.4", default_base_url: "https://api.openai.com/v1", key_required: true, custom_base_url_allowed: false, base_url_required: true, allowed_schemes: ["https"], connection_kind: "cloud_canonical", base_url_entry_mode: "hidden_canonical", advanced_manual_base_url_allowed: false, requires_connection_target: false, requires_reachability_probe: false },
      { id: "ollama", label: "Ollama Local", default_model: "llama3.2", default_base_url: "", key_required: false, custom_base_url_allowed: true, base_url_required: true, allowed_schemes: ["http", "https"], connection_kind: "local_server", base_url_entry_mode: "guided_local_server", advanced_manual_base_url_allowed: true, requires_connection_target: true, requires_reachability_probe: false, setup_guidance_code: "ollama_connection_target_required" },
      { id: "ollama_cloud", label: "Ollama Cloud", default_model: "gpt-oss:120b", default_base_url: "https://ollama.com", key_required: true, custom_base_url_allowed: false, base_url_required: true, allowed_schemes: ["https"], connection_kind: "cloud_canonical", base_url_entry_mode: "hidden_canonical", advanced_manual_base_url_allowed: false, requires_connection_target: false, requires_reachability_probe: false }
    ]);
    expect(rows.map((row) => row.label)).toEqual(["OpenAI / GPT", "Ollama"]);
    expect(setupSource).toContain("function handleChooseProviderDisplayRow");
    expect(setupSource).toContain("buildProviderDisplayRows(localProviderOptions).map");
    expect(setupSource).toContain("OLLAMA_CHOICE_CLOUD");
    expect(setupSource).toContain("onPress: () => enterByokProvider(cloud)");
    expect(setupSource).toContain("onPress: () => handleChooseByokProvider(local)");
    expect(settingsSource).toContain("function chooseProviderDisplayRow");
    expect(settingsSource).toContain("buildProviderDisplayRows(providerOptions).map");
    expect(settingsSource).toContain("OLLAMA_CHOICE_CLOUD");
    expect(settingsSource).toContain("onPress: () => chooseProviderOption(cloud)");
    expect(settingsSource).toContain("onPress: () => chooseProviderOption(local)");
    expect(helperSource).toContain("OLLAMA_SETUP_UNAVAILABLE_TITLE");
    expect(helperSource).toContain("xMilo could not load both Ollama setup options");
  });

  it("gates Ollama behind the advanced local model acceptance popup before guided setup", () => {
    const setupSource = fs.readFileSync(path.join(__dirname, "../../app/setup.tsx"), "utf8");
    const settingsSource = fs.readFileSync(path.join(__dirname, "../../app/settings.tsx"), "utf8");
    const helperSource = fs.readFileSync(path.join(__dirname, "./providerSetup.ts"), "utf8");
    expect(setupSource).toContain("function handleChooseByokProvider(option: LocalProviderOption)");
    expect(setupSource).toContain("if (!isGuidedLocalServerProvider(option))");
    expect(settingsSource).toContain("function chooseProviderOption(option: LocalProviderOption)");
    expect(settingsSource).toContain("if (!isGuidedLocalServerProvider(option))");
    expect(`${setupSource}\n${settingsSource}`).toContain("OLLAMA_ACCEPTANCE_TITLE");
    expect(helperSource).toContain("Advanced local model setup");
    expect(helperSource).toContain("Ollama Local uses a reachable Ollama server. It does not use Ollama Cloud or an ollama.com API key in this setup.");
    expect(helperSource).toContain("Use Ollama Local");
    expect(helperSource).toContain("Choose another provider");
    expect(setupSource).toContain("setByokProvider(\"\")");
    expect(setupSource).toContain('setStep("byok_choose_provider")');
  });

  it("removes the old generic base-url save failure from normal provider setup", () => {
    const setupSource = fs.readFileSync(path.join(__dirname, "../../app/setup.tsx"), "utf8");
    const settingsSource = fs.readFileSync(path.join(__dirname, "../../app/settings.tsx"), "utf8");
    const bridgeSource = fs.readFileSync(path.join(__dirname, "./bridge.ts"), "utf8");
    expect(setupSource).not.toContain("BYOK key not saved");
    expect(`${setupSource}\n${settingsSource}\n${bridgeSource}`).not.toContain("Local provider base URL is required before saving.");
    expect(bridgeSource).toContain("Ollama Local must be running on this phone or on a computer your phone can reach before xMilo can use it.");
    expect(bridgeSource).toContain("Check the Ollama Local server address and try again.");
  });

  it("does not rely only on strings for stale Ollama compatibility", () => {
    expect(getProviderSetupMode({ id: "ollama", label: "Ollama", key_required: false })).toBe("guided_local_server");
    expect(isGuidedLocalServerProvider({ id: "ollama", label: "Ollama", key_required: false })).toBe(true);
    expect(getProviderSetupCopy({ id: "ollama", label: "Ollama", key_required: false }).title).toBe("Connect to Ollama Local");
    expect(getProviderSetupCopy({ id: "ollama_cloud", label: "Ollama Cloud", key_required: true }).title).toBe("Enter your API key");
    expect(isGuidedLocalServerProvider({ id: "ollama_cloud", label: "Ollama Cloud", key_required: true })).toBe(false);
    expect(getProviderSetupCopy({ id: "ollama", label: "Ollama", key_required: false }).body).not.toContain("Paste your API key");
    expect(getProviderSetupCopy({ id: "ollama_cloud", label: "Ollama Cloud", key_required: true }).body).not.toContain("server address");
  });

  it("keeps app-store billing stack removed", () => {
    const packageSource = fs.readFileSync(path.join(__dirname, "../../package.json"), "utf8");
    const setupSource = fs.readFileSync(path.join(__dirname, "../../app/setup.tsx"), "utf8");
    const settingsSource = fs.readFileSync(path.join(__dirname, "../../app/settings.tsx"), "utf8");
    const combined = `${packageSource}\n${setupSource}\n${settingsSource}`;
    expect(combined).not.toMatch(/RevenueCat|react-native-purchases|react-native-purchases-ui|com\.android\.vending\.BILLING|amazon-appstore|com\.amazon\.device|restorePurchases|Paywall/i);
  });
});
