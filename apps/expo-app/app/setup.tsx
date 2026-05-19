import { useRouter } from "expo-router";
import { useCallback, useEffect, useRef, useState } from "react";
import {
  ActivityIndicator,
  Alert,
  BackHandler,
  AppState,
  NativeModules,
  PermissionsAndroid,
  Pressable,
  ScrollView,
  StyleSheet,
  Text,
  TextInput,
  View
} from "react-native";
import {
  authCheck,
  authRedeemInvite,
  authRegister,
  getLocalProviderOptions,
  verifyTwoFactor,
  getState,
  resolveLocalProviderConfig,
  type LocalProviderOption
} from "../src/lib/bridge";
import {
  configureRevenueCat,
  getCustomerInfo,
  hasRequiredEntitlement,
  isRevenueCatConfigured,
  PAYWALL_RESULT,
  presentSubscriptionPaywall
} from "../src/lib/revenuecat";
import {
  getXMiloclawRuntimeStatus,
  hasNativeXMiloclawRuntimeHost,
  openXMiloclawAccessibilitySettings,
  openXMiloclawBatteryOptimizationSettings,
  openXMiloclawDataSaverSettings,
  openXMiloclawNotificationSettings,
  openXMiloclawOverlaySettings,
  openXMiloclawSetupPermissionSettings,
  restartXMiloclawRuntimeHost,
  getXMiloclawSetupPermissionSnapshot,
  recheckXMiloclawSetupPermissionSnapshot,
  isSetupPermissionFinalReady,
  getXMiloclawLocalByokStatus,
  saveXMiloclawLocalByokApiKey,
  startXMiloclawRuntimeHost,
  type SetupPermissionSnapshot,
  type SetupPermissionSnapshotRow,
  type XMiloclawRuntimeStatus
} from "../src/lib/xmiloRuntimeHost";
import { hasSetupCompletedOnce, markSetupCompletedOnce } from "../src/lib/setupCompletion";
import { useApp } from "../src/state/AppContext";

type Step =
  | "checking"
  | "waiting_sidecar"
  | "email"
  | "email_sent"
  | "two_factor"
  | "access_choice"
  | "invite_input"
  | "byok_choose_provider"
  | "byok_enter_key";

type AccessConfig = {
  access_mode?: string;
  access_code_only?: boolean;
  trial_allowed?: boolean;
  subscription_allowed?: boolean;
  access_code_grant_days?: number;
};

const TRUST_PAGES = [
  {
    title: "Magic requires trust",
    body:
      "xMilo uses an app-owned hidden runtime host so first-run setup stays local on your phone."
  },
  {
    title: "We warn you first",
    body:
      "Android can show strong warning screens in red. That does not mean something is hidden. It means the system is asking for clear consent before local tools can run."
  },
  {
    title: "You stay in control",
    body:
      "Nothing should install or run silently. We show direct links, tell you why each step matters, and if you decline, xMilo simply cannot finish local setup."
  }
] as const;

type Page3RuntimeStep =
  | "hidden_runtime_host_path"
  | "allow_basic_permissions"
  | "additional_access_review"
  | "prepare_hidden_runtime_host"
  | "return_to_xmilo"
  | "re_check";

type Page3PermissionStatus = "granted" | "missing" | "unknown";

type SetupPermissionActionRow = "camera" | "microphone" | "media" | "location" | "physical_activity";
type SetupPermissionNativeAction = "request" | "open_settings" | "none";

type SetupPermissionRequestResult = {
  row?: string;
  result?: string;
  reason?: string;
  permissions_count?: number;
  all_granted?: boolean;
  any_granted?: boolean;
};

type SetupPermissionNativeModule = {
  requestSetupPermission?: (row: SetupPermissionActionRow) => Promise<SetupPermissionRequestResult>;
};

const setupPermissionNativeModule = NativeModules.XMiloclawRuntimeModule as SetupPermissionNativeModule | undefined;

type Page3Snapshot = {
  savedAtMillis: number;
  trustIntroDismissed: boolean;
  trustPage: number;
  runtimeStep: Page3RuntimeStep;
  runtimeStatus: XMiloclawRuntimeStatus | null;
  runtimeError: string;
  permissionChecklist: Record<string, Page3PermissionStatus>;
  reviewAtBottom: boolean;
  reviewAcknowledged: boolean;
  returnReason: string;
};

let lastPage3Snapshot: Page3Snapshot | null = null;

function getFreshPage3Snapshot(): Page3Snapshot | null {
  if (!lastPage3Snapshot) return null;
  const maxAgeMillis = 10 * 60 * 1000;
  if (Date.now() - lastPage3Snapshot.savedAtMillis > maxAgeMillis) return null;
  return lastPage3Snapshot;
}

function foregroundRuntimeComplete(status: XMiloclawRuntimeStatus | null): boolean {
  return status?.runtimeHostStarted === true && status?.foregroundServiceStarted === true;
}

type AllowBasicRowId =
  | "show_notifications"
  | "accessibility_service_enabled"
  | "allow_data_usage_data_saver"
  | "appear_on_top"
  | "unrestricted_battery_access";

type AllowBasicRowMeta = {
  id: AllowBasicRowId;
  label: string;
  androidMapping?: string;
};

type AdditionalAccessRowBehavior =
  | "clickable_runtime_request"
  | "clickable_settings_path"
  | "visible_non_clickable_status";

type AdditionalAccessRowMeta = {
  id: string;
  section: "required_now_checked_now" | "requested_later_features" | "internal_runtime_capabilities";
  label: string;
  behavior: AdditionalAccessRowBehavior;
  requiredForContinue: boolean;
  status: Page3PermissionStatus;
  actionHint?: string;
};

const CLR = {
  bg: "#0B1020",
  card: "#111827",
  border: "#1F2937",
  title: "#F8FAFC",
  body: "#CBD5E1",
  muted: "#6B7280",
  accent: "#6366F1",
  accentDim: "#312E81",
  success: "#10B981",
  warn: "#F59E0B",
  danger: "#EF4444",
  input: "#0F172A",
  inputBorder: "#374151"
};

export default function SetupScreen() {
  const router = useRouter();
  const { resetRuntimeTaskProof, refreshRuntimeState } = useApp();
  const [step, setStep] = useState<Step>("checking");
  const [email, setEmail] = useState("");
  const [inviteCode, setInviteCode] = useState("");
  const [deviceUserID, setDeviceUserID] = useState("");
  const [error, setError] = useState("");
  const [busy, setBusy] = useState(false);
  const [verifyAttempts, setVerifyAttempts] = useState(0);
  const [twoFactorCode, setTwoFactorCode] = useState("");
  const [recoveryCode, setRecoveryCode] = useState("");
  const [trustPage, setTrustPage] = useState(0);
  const [trustIntroDismissed, setTrustIntroDismissed] = useState(false);
  const [byokProvider, setByokProvider] = useState("");
  const [localProviderOptions, setLocalProviderOptions] = useState<LocalProviderOption[]>([]);
  const [providerOptionsBusy, setProviderOptionsBusy] = useState(false);
  const [byokKey, setByokKey] = useState("");
  const [byokBaseUrl, setByokBaseUrl] = useState("");
  const [byokModel, setByokModel] = useState("");
  const [byokBusy, setByokBusy] = useState(false);
  const [runtimeStep, setRuntimeStep] = useState<
    Page3RuntimeStep
  >("hidden_runtime_host_path");
  const [runtimeStatus, setRuntimeStatus] = useState<XMiloclawRuntimeStatus | null>(null);
  const [setupPermissionSnapshot, setSetupPermissionSnapshot] = useState<SetupPermissionSnapshot | null>(null);
  const [runtimeBusy, setRuntimeBusy] = useState(false);
  const [runtimeError, setRuntimeError] = useState("");
  const [reviewAtBottom, setReviewAtBottom] = useState(false);
  const [reviewAcknowledged, setReviewAcknowledged] = useState(false);
  const [returnReason, setReturnReason] = useState<string>("");
  const [permissionChecklist, setPermissionChecklist] = useState<Record<string, Page3PermissionStatus>>({});
  const [checklistUpdatedAtMillis, setChecklistUpdatedAtMillis] = useState<number>(0);
  const reviewLayoutHeightRef = useRef(0);
  const reviewContentHeightRef = useRef(0);
  const finalConfirmAutoRanRef = useRef(false);
  const [finalConfirmCheckedOnce, setFinalConfirmCheckedOnce] = useState(false);
  const [accessConfig, setAccessConfig] = useState<AccessConfig>({
    access_mode: "code_only",
    access_code_only: true,
    trial_allowed: false,
    subscription_allowed: false,
    access_code_grant_days: 30
  });
  const subscriptionEntryVisible = Boolean(accessConfig.subscription_allowed && isRevenueCatConfigured());
  const setupSeamSatisfied = useCallback((status: XMiloclawRuntimeStatus | null) => {
    if (!status) return false;
    return (
      status.notificationsGranted &&
      status.appearOnTopGranted &&
      status.batteryUnrestricted &&
      status.dataSaverUnrestricted &&
      status.accessibilityEnabled &&
      status.runtimeHostStarted &&
      status.bridgeConnected &&
      status.hostReady
    );
  }, []);

  const resetSetupToStep1 = useCallback(() => {
    lastPage3Snapshot = null;
    setTrustIntroDismissed(false);
    setTrustPage(0);
    setRuntimeStep("hidden_runtime_host_path");
    setRuntimeStatus(null);
    setRuntimeError("");
    setPermissionChecklist({});
    setReviewAtBottom(false);
    setReviewAcknowledged(false);
    setReturnReason("");
  }, []);

  function blockOnMissingSetupProof(message: string): false {
    setRuntimeStep("re_check");
    setRuntimeError(message);
    return false;
  }

  const continueAfterSeamSatisfied = useCallback(async () => {
    try {
      await markSetupCompletedOnce();
    } catch {
    }
    setStep("email");
    return true;
  }, []);

  useEffect(() => {
    async function boot() {
      const setupCompleted = await hasSetupCompletedOnce().catch(() => false);
      if (!setupCompleted) {
        resetSetupToStep1();
        setStep("waiting_sidecar");
        return;
      }

      let status: XMiloclawRuntimeStatus | null = null;
      try {
        status = await getXMiloclawRuntimeStatus();
      } catch {
        status = null;
      }

      if (!setupSeamSatisfied(status)) {
        resetSetupToStep1();
        if (status) setRuntimeStatus(status);
        setStep("waiting_sidecar");
        return;
      }

      try {
        const continued = await continueAfterSeamSatisfied();
        if (continued) {
          return;
        }
        setStep("waiting_sidecar");
      } catch {
        setStep("waiting_sidecar");
      }
    }
    boot();
  }, [continueAfterSeamSatisfied, resetSetupToStep1, setupSeamSatisfied]);

  useEffect(() => {
    if (step !== "waiting_sidecar") return;
    const orderedSteps = [
      "hidden_runtime_host_path",
      "allow_basic_permissions",
      "additional_access_review",
      "prepare_hidden_runtime_host",
      "return_to_xmilo",
      "re_check"
    ] as const;

    const sub = BackHandler.addEventListener("hardwareBackPress", () => {
      if (!trustIntroDismissed) {
        if (trustPage > 0) {
          setTrustPage((current) => Math.max(0, current - 1));
        }
        return true;
      }

      const idx = orderedSteps.indexOf(runtimeStep);
      if (idx > 0) {
        setRuntimeStep(orderedSteps[idx - 1]);
        setRuntimeError("");
        return true;
      }

      setTrustIntroDismissed(false);
      setTrustPage(0);
      setRuntimeStep("hidden_runtime_host_path");
      setRuntimeError("");
      return true;
    });

    return () => sub.remove();
  }, [runtimeStep, step, trustIntroDismissed, trustPage]);

  const refreshRuntimeStatus = useCallback(async () => {
    try {
      const status = await getXMiloclawRuntimeStatus();
      setRuntimeStatus(status);
      return status;
    } catch (error: any) {
      setRuntimeError(error?.message ?? "Could not read runtime host status.");
      return null;
    }
  }, []);

  const isProviderSetupRuntimeReady = useCallback((status: XMiloclawRuntimeStatus | null) => {
    return Boolean(status?.hostReady && status?.taskRouteSurfaceReady);
  }, []);

  const getFinalRuntimeMissing = useCallback((status: XMiloclawRuntimeStatus | null) => {
    const snapshot =
      status ?? {
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
        hostReady: false
      };

    const missing: Array<"Data Saver unrestricted" | "Foreground service started" | "Runtime files prepared" | "Sidecar process alive" | "Bridge connected" | "Task route surface ready" | "Host ready"> = [];
    if (!snapshot.dataSaverUnrestricted) missing.push("Data Saver unrestricted");
    if (!(snapshot.foregroundServiceStarted ?? snapshot.runtimeHostStarted)) missing.push("Foreground service started");
    if (!snapshot.runtimeFilesPrepared) missing.push("Runtime files prepared");
    if (!snapshot.sidecarProcessAlive) missing.push("Sidecar process alive");
    if (!snapshot.bridgeConnected) missing.push("Bridge connected");
    if (!snapshot.taskRouteSurfaceReady) missing.push("Task route surface ready");
    if (!snapshot.hostReady) missing.push("Host ready");
    return missing;
  }, []);

  const runtimeNotReadyReason = useCallback((status: XMiloclawRuntimeStatus | null) => {
    if (status?.lastError) return status.lastError;
    const missing = getFinalRuntimeMissing(status);
    return missing.length ? `Missing runtime readiness: ${missing.join(", ")}` : "Runtime host is not ready.";
  }, [getFinalRuntimeMissing]);

  const ensureProviderSetupRuntimeReady = useCallback(async () => {
    if (!hasNativeXMiloclawRuntimeHost()) {
      throw new Error("Native xMilo runtime host is not available in this build.");
    }

    const current = await getXMiloclawRuntimeStatus();
    setRuntimeStatus(current);
    if (isProviderSetupRuntimeReady(current)) {
      return current;
    }

    const started = await startXMiloclawRuntimeHost();
    setRuntimeStatus(started);
    if (isProviderSetupRuntimeReady(started)) {
      return started;
    }

    const restarted = await restartXMiloclawRuntimeHost();
    setRuntimeStatus(restarted);
    if (isProviderSetupRuntimeReady(restarted)) {
      return restarted;
    }

    throw new Error(runtimeNotReadyReason(restarted));
  }, [isProviderSetupRuntimeReady, runtimeNotReadyReason]);

  useEffect(() => {
    if (step !== "byok_choose_provider") return;
    let cancelled = false;
    async function loadProviderOptions() {
      setProviderOptionsBusy(true);
      setError("");
      try {
        await ensureProviderSetupRuntimeReady();
        const providers = await getLocalProviderOptions();
        if (!cancelled) {
          setLocalProviderOptions(providers);
        }
      } catch (error: any) {
        if (!cancelled) {
          setLocalProviderOptions([]);
          setError(error?.message ?? "Could not load local provider options from the sidecar.");
        }
      } finally {
        if (!cancelled) {
          setProviderOptionsBusy(false);
        }
      }
    }
    loadProviderOptions();
    return () => {
      cancelled = true;
    };
  }, [ensureProviderSetupRuntimeReady, step]);

  async function waitForByokReadinessAfterSave(resolved: {
    provider: string;
    model: string;
    key_required: boolean;
    base_url_required: boolean;
  }) {
    const localStatus = await getXMiloclawLocalByokStatus();
    if (localStatus.provider !== resolved.provider) {
      throw new Error("Saved provider did not match the sidecar-resolved provider.");
    }
    if (resolved.key_required && !localStatus.keyFileReady) {
      throw new Error("Saved local provider key was not ready after write.");
    }
    if (resolved.base_url_required && !localStatus.baseUrlReady) {
      throw new Error("Saved local provider base URL was not ready after write.");
    }
    if (resolved.model && localStatus.model !== resolved.model) {
      throw new Error("Saved local provider model did not match the sidecar-resolved model.");
    }

    let lastReason = "sidecar_not_checked";
    for (let attempt = 0; attempt < 5; attempt += 1) {
      const ready = await getXMiloclawRuntimeStatus();
      setRuntimeStatus(ready);
      const localReady =
        ready.hostReady === true &&
        ready.taskRouteSurfaceReady === true &&
        ready.llmMode === "local_byok" &&
        ready.byokProvider === resolved.provider &&
        ready.bringYourOwnKeyActive === true &&
        ready.localLlmTurnAllowed === true &&
        ready.firstTaskEligible === true;
      if (localReady) return;
      lastReason = `llm=${ready.llmMode ?? "unknown"} provider=${ready.byokProvider ?? "unknown"} local=${String(ready.localLlmTurnAllowed)} byok=${String(ready.bringYourOwnKeyActive)}`;
      await new Promise((resolve) => setTimeout(resolve, 400));
    }
    throw new Error(`Local BYOK readiness was not confirmed after save (${lastReason}).`);
  }

  const runFinalRuntimeConfirmation = useCallback(async () => {
    setRuntimeBusy(true);
    setRuntimeError("");
    setFinalConfirmCheckedOnce(false);
    try {
      const status = await refreshRuntimeStatus();
      const missing = getFinalRuntimeMissing(status);
      setFinalConfirmCheckedOnce(true);
      if (missing.length === 0) {
        await continueAfterSeamSatisfied();
      }
    } finally {
      setRuntimeBusy(false);
    }
  }, [continueAfterSeamSatisfied, getFinalRuntimeMissing, refreshRuntimeStatus]);

  useEffect(() => {
    if (step !== "waiting_sidecar") return;
    refreshRuntimeStatus();
  }, [refreshRuntimeStatus, step]);

  useEffect(() => {
    if (step !== "waiting_sidecar" || runtimeStep !== "re_check") {
      finalConfirmAutoRanRef.current = false;
      setFinalConfirmCheckedOnce(false);
      return;
    }

    if (finalConfirmAutoRanRef.current) return;
    finalConfirmAutoRanRef.current = true;
    runFinalRuntimeConfirmation();
  }, [runFinalRuntimeConfirmation, step, runtimeStep]);

  useEffect(() => {
    if (step !== "waiting_sidecar") return;
    const snap = getFreshPage3Snapshot();
    if (!snap) return;
    setTrustIntroDismissed(snap.trustIntroDismissed);
    setTrustPage(snap.trustPage);
    setRuntimeStep(snap.runtimeStep);
    setRuntimeStatus(snap.runtimeStatus);
    setRuntimeError(snap.runtimeError);
    setPermissionChecklist(snap.permissionChecklist);
    setReviewAtBottom(snap.reviewAtBottom);
    setReviewAcknowledged(snap.reviewAcknowledged);
    setReturnReason(snap.returnReason);
  }, [step]);

  useEffect(() => {
    if (step !== "waiting_sidecar") return;
    lastPage3Snapshot = {
      savedAtMillis: Date.now(),
      trustIntroDismissed,
      trustPage,
      runtimeStep,
      runtimeStatus,
      runtimeError,
      permissionChecklist,
      reviewAtBottom,
      reviewAcknowledged,
      returnReason
    };
  }, [
    step,
    trustIntroDismissed,
    trustPage,
    runtimeStep,
    runtimeStatus,
    runtimeError,
    permissionChecklist,
    reviewAtBottom,
    reviewAcknowledged,
    returnReason
  ]);

  useEffect(() => {
    if (step !== "waiting_sidecar") return;
    if (!trustIntroDismissed) {
      if (runtimeStep !== "hidden_runtime_host_path") setRuntimeStep("hidden_runtime_host_path");
      if (runtimeError) setRuntimeError("");
      if (trustPage < 0) setTrustPage(0);
      if (trustPage > TRUST_PAGES.length - 1) setTrustPage(TRUST_PAGES.length - 1);
    }
  }, [step, trustIntroDismissed, runtimeStep, runtimeError, trustPage]);

  const logSetupPermissionBreadcrumb = useCallback((event: string, fields?: Record<string, string | boolean | undefined>) => {
    const parts = ["XMILO_SETUP_PERMISSION_PROOF", `event=${event}`];
    for (const [key, value] of Object.entries(fields ?? {})) {
      if (typeof value === "undefined") continue;
      parts.push(`${key}=${String(value)}`);
    }
    console.info(parts.join(" "));
  }, []);

  function setupSnapshotRow(row: string, snapshot = setupPermissionSnapshot): SetupPermissionSnapshotRow | null {
    return snapshot?.rows.find((item) => item.row === row) ?? null;
  }

  function setupSnapshotStatus(row: string): Page3PermissionStatus {
    const item = setupSnapshotRow(row);
    if (!item) return "unknown";
    return item.complete ? "granted" : "missing";
  }

  function setupSnapshotAction(row: string): SetupPermissionNativeAction {
    const action = setupSnapshotRow(row)?.allowed_action;
    if (
      action === "request" ||
      action === "open_settings" ||
      action === "none"
    ) {
      return action;
    }
    return "none";
  }

  function setupSnapshotReason(row: string): string {
    return setupSnapshotRow(row)?.blocked_reason_key ?? "unknown";
  }

  function setupSnapshotHint(row: string): string {
    const item = setupSnapshotRow(row);
    if (!item) return "Permission state is not available yet.";
    if (item.complete) return item.accepted_text ?? "Requirement is complete.";
    return item.requirement_text ?? "Must complete this requirement";
  }

  const logSetupPermissionGate = useCallback((reviewed: boolean, complete: boolean) => {
    logSetupPermissionBreadcrumb("gate", {
      row: "foreground_runtime",
      gate_reviewed: reviewed,
      gate_permissions_complete: complete,
      gate_ready: reviewed && complete,
      source: "native_current"
    });
  }, [logSetupPermissionBreadcrumb]);

  useEffect(() => {
    if (step !== "waiting_sidecar" || runtimeStep !== "additional_access_review") return;
    for (const row of setupPermissionSnapshot?.rows ?? []) {
      logSetupPermissionBreadcrumb("action_state", {
        row: row.row,
        actionable: row.allowed_action !== "none",
        action: row.allowed_action,
        reason: row.blocked_reason_key ?? "none"
      });
      logSetupPermissionBreadcrumb("settings_correction_state", {
        row: row.row,
        prompt_state: row.prompt_state ?? "not_applicable",
        can_request_now: row.can_request_now === true
      });
    }
  }, [logSetupPermissionBreadcrumb, runtimeStep, setupPermissionSnapshot, step]);

  const computeChecklist = useCallback(async (status: XMiloclawRuntimeStatus | null) => {
    logSetupPermissionBreadcrumb("check_started", { source: "native_current" });
    const snapshot =
      status ?? {
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
        hostReady: false
      };

    const entries: Array<Promise<[string, Page3PermissionStatus]>> = [];

    const set = (id: string, value: Page3PermissionStatus) => {
      entries.push(Promise.resolve([id, value]));
    };

    set("show_notifications", snapshot.notificationsGranted ? "granted" : "missing");
    set("appear_on_top", snapshot.appearOnTopGranted ? "granted" : "missing");
    set("unrestricted_battery_access", snapshot.batteryUnrestricted ? "granted" : "missing");
    set("allow_data_usage_data_saver", snapshot.dataSaverUnrestricted ? "granted" : "missing");
    set("accessibility_service_enabled", snapshot.accessibilityEnabled ? "granted" : "missing");

    const results = await Promise.all(entries);
    const next: Record<string, Page3PermissionStatus> = {};
    for (const [id, value] of results) next[id] = value;

    const nativeSnapshot = await getXMiloclawSetupPermissionSnapshot().catch(() => null);
    setSetupPermissionSnapshot(nativeSnapshot);
    if (nativeSnapshot) {
      next.access_to_camera = nativeSnapshot.rows.find((row) => row.row === "camera")?.complete ? "granted" : "missing";
      next.access_to_microphone = nativeSnapshot.rows.find((row) => row.row === "microphone")?.complete ? "granted" : "missing";
      next.additional_access_location = nativeSnapshot.rows.find((row) => row.row === "location")?.complete ? "granted" : "missing";
      next.read_image_files = nativeSnapshot.rows.find((row) => row.row === "media")?.complete ? "granted" : "missing";
      next.access_to_photos_videos = nativeSnapshot.rows.find((row) => row.row === "media")?.complete ? "granted" : "missing";
      next.physical_activity = nativeSnapshot.rows.find((row) => row.row === "physical_activity")?.complete ? "granted" : "missing";
      next.run_foreground_service = nativeSnapshot.rows.find((row) => row.row === "foreground_runtime")?.complete ? "granted" : "missing";
      next.run_background_service = next.run_foreground_service;
      logSetupPermissionBreadcrumb("snapshot_consumed", {
        schema_version: String(nativeSnapshot.schema_version),
        permissions_complete: nativeSnapshot.gate.permissions_complete,
        foreground_runtime_complete: nativeSnapshot.gate.foreground_runtime_complete,
        final_ready_without_review: nativeSnapshot.gate.final_ready_without_review
      });
    }

    setPermissionChecklist(next);
    setChecklistUpdatedAtMillis(Date.now());
    return next;
  }, [logSetupPermissionBreadcrumb]);

  useEffect(() => {
    if (step !== "waiting_sidecar" || (runtimeStep !== "allow_basic_permissions" && runtimeStep !== "additional_access_review")) return;
    computeChecklist(runtimeStatus);
    const sub = AppState.addEventListener("change", (nextState) => {
      if (nextState === "active") {
        refreshRuntimeStatus().then((status) => computeChecklist(status));
      }
    });
    return () => sub.remove();
  }, [computeChecklist, refreshRuntimeStatus, runtimeStep, runtimeStatus, step]);

  useEffect(() => {
    if (step !== "waiting_sidecar" || runtimeStep !== "additional_access_review") return;
    const complete = setupPermissionSnapshot?.gate.final_ready_without_review === true;
    logSetupPermissionGate(reviewAtBottom, complete);
    if (!complete && reviewAcknowledged) {
      setReviewAcknowledged(false);
    }
  }, [
    logSetupPermissionGate,
    reviewAcknowledged,
    reviewAtBottom,
    runtimeStep,
    setupPermissionSnapshot,
    step
  ]);

  async function requestAndroidPermission(permission: string) {
    try {
      const result = await PermissionsAndroid.request(permission as any);
      return result === PermissionsAndroid.RESULTS.GRANTED;
    } catch {
      return false;
    }
  }

  function safeSetupPermissionRequestResult(value: unknown): string {
    return value === "completed" || value === "blocked" || value === "error" ? value : "unknown";
  }

  function safeSetupPermissionRequestReason(value: unknown): string {
    if (
      value === "granted" ||
      value === "denied" ||
      value === "blocked" ||
      value === "request_error" ||
      value === "activity_unavailable" ||
      value === "native_unavailable" ||
      value === "prompt_unavailable" ||
      value === "permanently_denied" ||
      value === "react_refresh" ||
      value === "permission_verification_required" ||
      value === "app_details_settings" ||
      value === "settings_error" ||
      value === "unknown"
    ) {
      return value;
    }
    return "unknown";
  }

  async function requestSetupPermissionRow(row: SetupPermissionActionRow, fallbackPermission?: string) {
    logSetupPermissionBreadcrumb("request_invoked", { row });
    try {
      if (setupPermissionNativeModule?.requestSetupPermission) {
        const result = await setupPermissionNativeModule.requestSetupPermission(row);
        logSetupPermissionBreadcrumb("request_result", {
          row,
          result: safeSetupPermissionRequestResult(result?.result),
          reason: safeSetupPermissionRequestReason(result?.reason)
        });
        return result?.all_granted === true;
      }
      if (fallbackPermission) {
        const granted = await requestAndroidPermission(fallbackPermission);
        logSetupPermissionBreadcrumb("request_result", {
          row,
          result: "completed",
          reason: granted ? "granted" : "native_unavailable"
        });
        return granted;
      }
      logSetupPermissionBreadcrumb("request_result", { row, result: "blocked", reason: "native_unavailable" });
      return false;
    } catch {
      logSetupPermissionBreadcrumb("request_result", { row, result: "error", reason: "request_error" });
      return false;
    }
  }

  async function dispatchSetupPermissionAction(row: SetupPermissionActionRow) {
    const action = setupSnapshotAction(row);
    const reason = setupSnapshotReason(row);
    if (action === "none") return;
    logSetupPermissionBreadcrumb("button_pressed", { row, action, reason });
    logSetupPermissionBreadcrumb("action_dispatched", { row, action, reason });
    if (action === "request") {
      await requestSetupPermissionRow(row);
      const status = await refreshRuntimeStatus();
      await computeChecklist(status);
      const latestSnapshot = await getXMiloclawSetupPermissionSnapshot().catch(() => null);
      if (latestSnapshot) setSetupPermissionSnapshot(latestSnapshot);
      return;
    }
    if (action === "open_settings") {
      try {
        const result = await openXMiloclawSetupPermissionSettings(row);
        logSetupPermissionBreadcrumb("permission_settings_opened", {
          row,
          result: result?.opened === true,
          reason: safeSetupPermissionRequestReason(result?.reason)
        });
      } catch {
        logSetupPermissionBreadcrumb("permission_settings_opened", { row, result: false, reason: "settings_error" });
        setRuntimeError("Android requires this permission to be fixed in system settings before setup can continue.");
      }
    }
  }

  async function recheckSetupPermissions() {
    logSetupPermissionBreadcrumb("recheck_permissions_pressed", { source: "manual" });
    logSetupPermissionBreadcrumb("recheck_permissions_started", { source: "manual" });
    setRuntimeBusy(true);
    setRuntimeError("");
    try {
      const previousRows = setupPermissionSnapshot?.rows ?? [];
      const status = await refreshRuntimeStatus();
      await computeChecklist(status);
      const latestSnapshot = await recheckXMiloclawSetupPermissionSnapshot().catch(() => null);
      if (latestSnapshot) setSetupPermissionSnapshot(latestSnapshot);
      const changedRows = (latestSnapshot?.rows ?? [])
        .filter((row) => previousRows.find((previous) => previous.row === row.row)?.complete !== row.complete)
        .map((row) => `${row.row}:${row.complete ? "complete" : "incomplete"}`)
        .join(",");
      logSetupPermissionBreadcrumb("recheck_permissions_result", {
        result: latestSnapshot ? "completed" : "blocked",
        permissions_complete: latestSnapshot?.gate.permissions_complete === true,
        foreground_runtime_complete: latestSnapshot?.gate.foreground_runtime_complete === true,
        final_ready_without_review: latestSnapshot?.gate.final_ready_without_review === true,
        rows_changed: changedRows || "none"
      });
    } catch {
      logSetupPermissionBreadcrumb("recheck_permissions_result", { result: "error", reason: "request_error" });
      setRuntimeError("Could not recheck permissions. Try again.");
    } finally {
      setRuntimeBusy(false);
    }
  }

  function syncRevenueCat(appUserID: string) {
    if (!appUserID || !isRevenueCatConfigured()) return;
    try {
      configureRevenueCat(appUserID);
    } catch {
    }
  }

  async function waitForRelayEntitlement() {
    for (let i = 0; i < 6; i += 1) {
      const result = await authCheck().catch(() => null);
      if (result?.entitled) {
        router.replace("/lair");
        return true;
      }
      await new Promise((resolve) => setTimeout(resolve, 2000));
    }
    return false;
  }

  async function handleSendEmail() {
    setError("");
    const trimmed = email.trim().toLowerCase();
    if (!trimmed || !trimmed.includes("@")) {
      setError("Enter a valid email address.");
      return;
    }
    let resolvedDeviceUserID = deviceUserID.trim();
    if (!resolvedDeviceUserID) {
      try {
        const s = await getState();
        resolvedDeviceUserID = String(s?.runtime_id ?? "").trim();
      } catch {
        resolvedDeviceUserID = "";
      }
    }
    if (!resolvedDeviceUserID) {
      setError("xMilo is still initializing your device ID. Wait a moment and try again.");
      return;
    }
    if (resolvedDeviceUserID !== deviceUserID) {
      setDeviceUserID(resolvedDeviceUserID);
    }
    setBusy(true);
    try {
      await authRegister(trimmed, resolvedDeviceUserID);
      setEmail(trimmed);
      setStep("email_sent");
    } catch (e: any) {
      setError(e?.message ?? "Could not send verification email. Try again.");
    } finally {
      setBusy(false);
    }
  }

  async function handleCheckVerified() {
    setError("");
    setBusy(true);
    setVerifyAttempts((n) => n + 1);
    try {
      const result = await authCheck();
      setAccessConfig(result);
      if (!result.email_verified) {
        setError("Not verified yet — click the link in your inbox first.");
      } else if (result.two_factor_enabled && !result.two_factor_ok) {
        setStep("two_factor");
      } else {
        setStep("access_choice");
      }
    } catch (e: any) {
      setError(e?.message ?? "Could not reach xMilo. Try again.");
    } finally {
      setBusy(false);
    }
  }

  async function handleVerifyTwoFactor() {
    setError("");
    if (!twoFactorCode.trim() && !recoveryCode.trim()) {
      setError("Enter an authenticator code or a recovery code.");
      return;
    }
    setBusy(true);
    try {
      await verifyTwoFactor(twoFactorCode.trim(), recoveryCode.trim());
      const result = await authCheck();
      setAccessConfig(result);
      setTwoFactorCode("");
      setRecoveryCode("");
      setStep("access_choice");
    } catch (e: any) {
      setError(e?.message ?? "2FA verification failed.");
    } finally {
      setBusy(false);
    }
  }

  async function handleStartTrial() {
    router.replace("/lair");
  }

  async function handleRedeemInvite() {
    setError("");
    const code = inviteCode.trim().toUpperCase();
    if (!code) {
      setError("Enter your access code.");
      return;
    }
    setBusy(true);
    try {
      const result = await authRedeemInvite(code, deviceUserID);
      if (result?.access_code_grant_days) {
        setAccessConfig((current) => ({ ...current, access_code_grant_days: result.access_code_grant_days }));
      }
      router.replace("/lair");
    } catch (e: any) {
      setError(e?.message ?? "Invalid or already used code.");
    } finally {
      setBusy(false);
    }
  }

  async function handleOpenSubscription() {
    setError("");
    if (!deviceUserID) {
      setError("xMilo runtime ID missing. Restart setup and try again.");
      return;
    }
    if (!isRevenueCatConfigured()) {
      setError("RevenueCat is not configured yet. Add EXPO_PUBLIC_RC_ANDROID_API_KEY first.");
      return;
    }

    setBusy(true);
    try {
      configureRevenueCat(deviceUserID);
      const result = await presentSubscriptionPaywall();

      if (result === PAYWALL_RESULT.PURCHASED || result === PAYWALL_RESULT.RESTORED) {
        const relayReady = await waitForRelayEntitlement();
        if (relayReady) return;

        const customerInfo = await getCustomerInfo().catch(() => null);
        if (customerInfo && hasRequiredEntitlement(customerInfo)) {
          setError("Purchase succeeded. xMilo is finalizing access — tap again in a moment.");
        } else {
          setError("Purchase did not complete.");
        }
        return;
      }

      if (result === PAYWALL_RESULT.ERROR) {
        setError("Subscription flow failed. Check your RevenueCat offering and Play Billing setup.");
      }
    } catch (e: any) {
      setError(e?.message ?? "Subscription flow failed.");
    } finally {
      setBusy(false);
    }
  }

  async function handleByokSaveAndContinue() {
    if (!byokProvider) return;

    const trimmedKey = byokKey.trim();
    const trimmedBaseUrl = byokBaseUrl.trim();
    const trimmedModel = byokModel.trim();

    setByokBusy(true);
    try {
      await ensureProviderSetupRuntimeReady();
      const resolved = await resolveLocalProviderConfig({
        provider: byokProvider,
        base_url: trimmedBaseUrl,
        model: trimmedModel,
        has_api_key: Boolean(trimmedKey),
        key_file_ready: false
      });
      if (!resolved.local_turn_allowed) {
        throw new Error(resolved.readiness_reason || "Local provider configuration is not ready.");
      }
      await saveXMiloclawLocalByokApiKey(resolved.provider, trimmedKey, resolved.base_url, resolved.model);
      resetRuntimeTaskProof();
      const restartStatus = await restartXMiloclawRuntimeHost();
      setRuntimeStatus(restartStatus);
      await waitForByokReadinessAfterSave(resolved);
      await refreshRuntimeState().catch(() => null);
      await markSetupCompletedOnce().catch(() => null);
      setByokKey("");
      setByokBaseUrl("");
      setByokModel("");
      router.replace("/lair");
    } catch (error: any) {
      Alert.alert("BYOK key not saved", error?.message ?? "Could not save local API key.");
    } finally {
      setByokBusy(false);
    }
  }

  if (step === "checking") {
    return (
      <View style={styles.center}>
        <ActivityIndicator color={CLR.accent} size="large" />
        <Text style={styles.muted}>Starting up…</Text>
      </View>
    );
  }

  if (step === "waiting_sidecar") {
    const trustSlide = TRUST_PAGES[trustPage];
    const trustComplete = trustPage >= TRUST_PAGES.length - 1;
    const nativeRuntimeAvailable = hasNativeXMiloclawRuntimeHost();
    const allowBasicInventory: AllowBasicRowMeta[] = [
      {
        id: "show_notifications",
        label: "Show notifications",
        androidMapping: "Android: POST_NOTIFICATIONS (app-owned proof surface)"
      },
      {
        id: "appear_on_top",
        label: "Appear on top",
        androidMapping: "Android: overlay permission (app-owned proof surface)"
      },
      {
        id: "unrestricted_battery_access",
        label: "Unrestricted battery access",
        androidMapping: "Android: Unrestricted battery access (app-owned proof surface)"
      },
      {
        id: "allow_data_usage_data_saver",
        label: "Allow data usage while Data Saver is on",
        androidMapping: "Android: Data Saver unrestricted data access (app-owned proof surface)"
      },
      {
        id: "accessibility_service_enabled",
        label: "Accessibility Service enabled",
        androidMapping: "Android: Accessibility Service (app-owned proof surface)"
      }
    ];

    const combineStatuses = (...statuses: Page3PermissionStatus[]): Page3PermissionStatus => {
      if (statuses.length > 0 && statuses.every((status) => status === "granted")) return "granted";
      if (statuses.some((status) => status === "missing")) return "missing";
      return "unknown";
    };

    const runtimeProof: XMiloclawRuntimeStatus =
      runtimeStatus ?? {
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
        hostReady: false
      };

    const additionalAccessRows: AdditionalAccessRowMeta[] = [
      {
        id: "show_notifications",
        section: "required_now_checked_now",
        label: "Show notifications",
        behavior: "clickable_runtime_request",
        requiredForContinue: true,
        status: runtimeProof.notificationsGranted ? "granted" : "missing",
        actionHint: "Tap to request Android notifications access."
      },
      {
        id: "appear_on_top",
        section: "required_now_checked_now",
        label: "Appear on top",
        behavior: "clickable_settings_path",
        requiredForContinue: true,
        status: runtimeProof.appearOnTopGranted ? "granted" : "missing",
        actionHint: "Tap to open Appear on top settings."
      },
      {
        id: "unrestricted_battery_access",
        section: "required_now_checked_now",
        label: "Unrestricted battery access",
        behavior: "clickable_settings_path",
        requiredForContinue: true,
        status: runtimeProof.batteryUnrestricted ? "granted" : "missing",
        actionHint: "Tap to open Battery optimization settings."
      },
      {
        id: "camera",
        section: "requested_later_features",
        label: "Camera permission state",
        behavior: "clickable_runtime_request",
        requiredForContinue: true,
        status: setupSnapshotStatus("camera"),
        actionHint: setupSnapshotHint("camera")
      },
      {
        id: "microphone",
        section: "requested_later_features",
        label: "Microphone permission state",
        behavior: "clickable_runtime_request",
        requiredForContinue: true,
        status: setupSnapshotStatus("microphone"),
        actionHint: setupSnapshotHint("microphone")
      },
      {
        id: "photos_videos",
        section: "requested_later_features",
        label: "Photos and videos",
        behavior: "clickable_runtime_request",
        requiredForContinue: true,
        status: setupSnapshotStatus("media"),
        actionHint: setupSnapshotHint("media")
      },
      {
        id: "location",
        section: "requested_later_features",
        label: "Location permission state",
        behavior: "clickable_runtime_request",
        requiredForContinue: true,
        status: setupSnapshotStatus("location"),
        actionHint: setupSnapshotHint("location")
      },
      {
        id: "physical_activity",
        section: "requested_later_features",
        label: "Physical activity",
        behavior: "clickable_runtime_request",
        requiredForContinue: true,
        status: setupSnapshotStatus("physical_activity"),
        actionHint: setupSnapshotHint("physical_activity")
      },
      {
        id: "app_owned_hidden_runtime_execution",
        section: "internal_runtime_capabilities",
        label: "Foreground runtime service",
        behavior: "clickable_settings_path",
        requiredForContinue: true,
        status: setupSnapshotStatus("foreground_runtime"),
        actionHint: setupSnapshotHint("foreground_runtime")
      },
      {
        id: "foreground_background_service_capability",
        section: "internal_runtime_capabilities",
        label: "Foreground/background service capability",
        behavior: "visible_non_clickable_status",
        requiredForContinue: true,
        status: setupSnapshotStatus("foreground_runtime"),
        actionHint: "Internal capability/state; no direct user-grant action on this screen."
      }
    ];

    const requiredNowMissing: string[] = [];
    if (!runtimeProof.notificationsGranted) requiredNowMissing.push("Show notifications");
    if (!runtimeProof.appearOnTopGranted) requiredNowMissing.push("Appear on top");
    if (!runtimeProof.batteryUnrestricted) requiredNowMissing.push("Unrestricted battery access");
    if (!runtimeProof.dataSaverUnrestricted) requiredNowMissing.push("Allow data usage while Data Saver is on");
    if (!runtimeProof.accessibilityEnabled) requiredNowMissing.push("Accessibility Service enabled");

    function setMissingMessage(prefix: string, missing: string[]) {
      if (missing.length === 0) {
        setRuntimeError("");
        return;
      }
      setRuntimeError(`${prefix}: ${missing.join(", ")}`);
    }

    async function handleTrustPrimary() {
      if (trustComplete) {
        setTrustIntroDismissed(true);
        setRuntimeStep("hidden_runtime_host_path");
        setRuntimeError("");
        await refreshRuntimeStatus();
        return;
      }
      setTrustPage((current) => Math.min(TRUST_PAGES.length - 1, current + 1));
    }

    async function handleOpenSetting(kind: "notifications" | "overlay" | "battery" | "data_saver" | "accessibility") {
      try {
        setRuntimeError("");
        if (kind === "notifications") {
          await openXMiloclawNotificationSettings();
        } else if (kind === "overlay") {
          await openXMiloclawOverlaySettings();
        } else if (kind === "battery") {
          await openXMiloclawBatteryOptimizationSettings();
        } else if (kind === "data_saver") {
          await openXMiloclawDataSaverSettings();
        } else {
          await openXMiloclawAccessibilitySettings();
        }
      } catch (error: any) {
        Alert.alert("Settings could not open", error?.message ?? "Unknown error");
      }
    }

    async function handleAdditionalAccessRowPress(row: AdditionalAccessRowMeta) {
      if (runtimeBusy || row.behavior === "visible_non_clickable_status") return;

      setRuntimeBusy(true);
      setRuntimeError("");
      try {
        if (row.id === "show_notifications") {
          await requestAndroidPermission("android.permission.POST_NOTIFICATIONS");
        } else if (row.id === "appear_on_top") {
          await handleOpenSetting("overlay");
        } else if (row.id === "unrestricted_battery_access") {
          await handleOpenSetting("battery");
        } else if (row.id === "camera") {
          await dispatchSetupPermissionAction("camera");
        } else if (row.id === "microphone") {
          await dispatchSetupPermissionAction("microphone");
        } else if (row.id === "photos_videos") {
          await dispatchSetupPermissionAction("media");
        } else if (row.id === "location") {
          await dispatchSetupPermissionAction("location");
        } else if (row.id === "physical_activity") {
          await dispatchSetupPermissionAction("physical_activity");
        } else if (row.id === "app_owned_hidden_runtime_execution") {
          const before = await refreshRuntimeStatus();
          if (!before?.runtimeHostStarted) {
            await startXMiloclawRuntimeHost();
          }
        }

                          const status = await refreshRuntimeStatus();
                          await computeChecklist(status);
                        } finally {
        setRuntimeBusy(false);
      }
    }

    async function handleFinishAllowBasicPermissions() {
      setRuntimeBusy(true);
      setRuntimeError("");
      try {
        const status = await refreshRuntimeStatus();
        if (!status) return;
        await computeChecklist(status);
        const missing: string[] = [];
        if (!status.notificationsGranted) missing.push("Show notifications");
        if (!status.appearOnTopGranted) missing.push("Appear on top");
        if (!status.batteryUnrestricted) missing.push("Unrestricted battery access");
        if (!status.dataSaverUnrestricted) missing.push("Allow data usage while Data Saver is on");
        if (!status.accessibilityEnabled) missing.push("Accessibility Service enabled");
        if (missing.length > 0) {
          setMissingMessage("Still missing", missing);
          logSetupPermissionBreadcrumb("finish_blocked", { stage: "basic_permissions", reason: "permissions_incomplete" });
          return;
        }
        setReviewAtBottom(false);
        setReviewAcknowledged(false);
        setRuntimeStep("additional_access_review");
      } finally {
        setRuntimeBusy(false);
      }
    }

    async function handleStartHiddenRuntimeHost() {
      setRuntimeBusy(true);
      setRuntimeError("");
      try {
        const status = await ensureProviderSetupRuntimeReady();
        if (!(status.foregroundServiceStarted ?? status.runtimeHostStarted)) {
          setRuntimeError(status.lastError || "Runtime host did not start.");
          return;
        }
        if (!status.hostReady) {
          setRuntimeError(status.lastError || "Runtime service started, but runtime files/process/readiness are not complete yet.");
        }
        setRuntimeStep("re_check");
      } catch (error: any) {
        setRuntimeError(error?.message ?? "Could not start hidden runtime host.");
      } finally {
        setRuntimeBusy(false);
      }
    }

    async function handleRestartHiddenRuntimeHost() {
      setRuntimeBusy(true);
      setRuntimeError("");
      try {
        const status = await restartXMiloclawRuntimeHost();
        setRuntimeStatus(status);
        setRuntimeStep("re_check");
      } catch (error: any) {
        setRuntimeError(error?.message ?? "Could not restart hidden runtime host.");
      } finally {
        setRuntimeBusy(false);
      }
    }

    return (
      <ScrollView style={styles.scroll} contentContainerStyle={styles.padded}>
        {!trustIntroDismissed ? (
          <View style={styles.trustCard}>
            <Text style={styles.trustKicker}>Magic requires trust</Text>
            <Text style={styles.trustTitle}>{trustSlide.title}</Text>
            <Text style={styles.trustBody}>{trustSlide.body}</Text>
            <Text style={styles.trustPager}>
              {trustPage + 1} / {TRUST_PAGES.length}
            </Text>
            <View style={styles.trustActions}>
              {trustPage > 0 ? (
                <Pressable style={styles.linkButton} onPress={() => setTrustPage((current) => Math.max(0, current - 1))}>
                  <Text style={styles.linkButtonText}>Back</Text>
                </Pressable>
              ) : null}
              <Pressable style={[styles.btn, styles.trustPrimary]} onPress={handleTrustPrimary}>
                <Text style={styles.btnLabel}>{trustComplete ? "I understand" : "Continue"}</Text>
              </Pressable>
            </View>
          </View>
        ) : null}

        {trustIntroDismissed ? (
          <>
            {!nativeRuntimeAvailable ? (
              <View style={styles.card}>
                <Text style={styles.cardTitle}>Hidden runtime host is missing</Text>
                <Text style={styles.body}>
                  This build does not include the native hidden runtime host module yet, so first-run setup cannot continue safely.
                </Text>
              </View>
            ) : null}

            {nativeRuntimeAvailable && runtimeStep === "hidden_runtime_host_path" ? (
              <View style={styles.card}>
                <Text style={styles.cardTitle}>Hidden runtime host path</Text>
                <Text style={styles.body}>
                  This setup uses an app-owned hidden runtime host. You will not be asked to copy commands or use a terminal.
                </Text>
                <Text style={styles.fine}>
                  If you decline required access, xMilo can still run, but on-device help and automation cannot be verified.
                </Text>
                <View style={styles.actionStack}>
                  <Pressable style={styles.linkButton} onPress={() => setRuntimeStep("allow_basic_permissions")}>
                    <Text style={styles.linkButtonText}>Continue</Text>
                  </Pressable>
                </View>
              </View>
            ) : null}

            {nativeRuntimeAvailable && runtimeStep === "allow_basic_permissions" ? (
              <View style={styles.card}>
                <Text style={styles.cardTitle}>Allow basic permissions</Text>
                <Text style={styles.body}>
                  This is the required startup checklist. Every item stays visible and will update after a re-check.
                </Text>
                <ScrollView
                  style={styles.checklistScroll}
                  contentContainerStyle={styles.checklistContent}
                  nestedScrollEnabled
                  showsVerticalScrollIndicator
                >
                  {allowBasicInventory.map((row) => {
                    const key = row.id;
                    const status = permissionChecklist[key] ?? "unknown";
                    const granted = status === "granted";
                    const actionableMissing = status === "missing";

                    const getRowAction = (rowId: string): "notifications" | "overlay" | "battery" | "data_saver" | "accessibility" | null => {
                      if (rowId === "show_notifications") return "notifications" as const;
                      if (rowId === "appear_on_top") return "overlay" as const;
                      if (rowId === "unrestricted_battery_access") return "battery" as const;
                      if (rowId === "allow_data_usage_data_saver") return "data_saver" as const;
                      if (rowId === "accessibility_service_enabled") return "accessibility" as const;
                      return null;
                    };

                    const actionKind = actionableMissing ? getRowAction(key) : null;
                    const rowPressable = Boolean(actionKind);

                    const onRowPress = async () => {
                      if (!actionKind || !actionableMissing || runtimeBusy) return;
                      setRuntimeError("");
                      try {
                        if (actionKind === "notifications") {
                          await requestAndroidPermission("android.permission.POST_NOTIFICATIONS");
                        } else if (actionKind === "overlay") {
                          await handleOpenSetting("overlay");
                        } else if (actionKind === "battery") {
                          await handleOpenSetting("battery");
                        } else if (actionKind === "data_saver") {
                          await handleOpenSetting("data_saver");
                        } else if (actionKind === "accessibility") {
                          await handleOpenSetting("accessibility");
                        }
                      } catch (error: any) {
                        Alert.alert("Could not open", error?.message ?? "Unknown error");
                      }
                    };

                    const labelStyle = granted ? styles.checkLabelGranted : styles.checkLabelMissing;
                    const badgeStyle = granted ? styles.checkBadgeGranted : styles.checkBadgeMissing;
                    const badgeText = granted ? "✓" : "•";

                    const RowContainer: any = rowPressable ? Pressable : View;
                    const rowProps = rowPressable
                      ? {
                          onPress: onRowPress,
                          accessibilityRole: "button",
                          disabled: runtimeBusy,
                          style: ({ pressed }: any) => [styles.checkRow, pressed && styles.checkRowPressed]
                        }
                      : { style: styles.checkRow };

                    return (
                      <RowContainer key={key} {...rowProps}>
                        <View style={{ flex: 1 }}>
                          <Text style={[styles.checkLabel, labelStyle]}>{row.label}</Text>
                          {row.androidMapping ? <Text style={styles.checkSubLabel}>{row.androidMapping}</Text> : null}
                        </View>
                        <View style={[styles.checkBadge, badgeStyle]}>
                          <Text style={styles.checkBadgeText}>{badgeText}</Text>
                        </View>
                      </RowContainer>
                    );
                  })}
                </ScrollView>

                {(() => {
                  const missingLines: string[] = [];
                  for (const row of allowBasicInventory) {
                    const status = permissionChecklist[row.id] ?? "unknown";
                    if (status === "granted") continue;
                    if (status === "unknown") {
                      missingLines.push(`${row.label} — UNKNOWN (cannot confirm from this build).`);
                      continue;
                    }
                    if (row.id === "show_notifications") {
                      missingLines.push(`${row.label} — tap the row to request, or allow in Notifications settings if prompted.`);
                    } else if (row.id === "appear_on_top") {
                      missingLines.push(`${row.label} — tap the row to open Appear-on-top settings.`);
                    } else if (row.id === "unrestricted_battery_access") {
                      missingLines.push(`${row.label} — tap the row to open App info → Battery → Unrestricted.`);
                    } else if (row.id === "allow_data_usage_data_saver") {
                      missingLines.push(`${row.label} — tap the row to open Data Saver settings.`);
                    } else if (row.id === "accessibility_service_enabled") {
                      missingLines.push(`${row.label} — tap the row to open Accessibility settings, then Installed apps → xMilo → turn on.`);
                    } else {
                      missingLines.push(`${row.label} — tap the row to act (request or open App settings).`);
                    }
                  }

                  const allRowsGranted = allowBasicInventory.every((row) => (permissionChecklist[row.id] ?? "unknown") === "granted");
                  const finishEnabled = allRowsGranted && !runtimeBusy;

                  return (
                    <>
                      <Text style={styles.missingNow}>
                        Missing now ({missingLines.length ? missingLines.length : "0"}):{" "}
                        {missingLines.length ? "\n- " + missingLines.join("\n- ") : "none"}
                      </Text>
                      <Text style={styles.fine}>Last re-check: {checklistUpdatedAtMillis ? new Date(checklistUpdatedAtMillis).toLocaleTimeString() : "—"}</Text>

                      <View style={styles.actionStack}>
                        <Pressable
                          style={[styles.linkButton, (!finishEnabled || runtimeBusy) && styles.linkButtonDisabled]}
                          disabled={!finishEnabled || runtimeBusy}
                          onPress={handleFinishAllowBasicPermissions}
                        >
                          <Text style={styles.linkButtonText}>{runtimeBusy ? "Checking..." : "Finish"}</Text>
                        </Pressable>
                      </View>
                    </>
                  );
                })()}
                {runtimeError ? <Text style={styles.error}>{runtimeError}</Text> : null}
              </View>
            ) : null}

            {nativeRuntimeAvailable && runtimeStep === "additional_access_review" ? (
              <View style={styles.card}>
                <Text style={styles.cardTitle}>Additional access review</Text>
                <Text style={styles.body}>
                  Disclosure only. Scroll to the bottom and acknowledge before continuing.
                </Text>
                {(() => {
                  const requiredRowsMissing = additionalAccessRows
                    .filter((row) => row.requiredForContinue)
                    .filter((row) => row.status !== "granted")
                    .map((row) => `${row.label} — ${row.actionHint || "tap the row and re-check."}`);
                  const nativeGateReady = setupPermissionSnapshot?.gate.final_ready_without_review === true;
                  const continueEnabled = reviewAcknowledged && isSetupPermissionFinalReady(setupPermissionSnapshot, reviewAtBottom) && !runtimeBusy;
                  const acknowledgeEnabled = reviewAtBottom && nativeGateReady && !runtimeBusy;

                  const renderSectionRows = (
                    section: AdditionalAccessRowMeta["section"],
                    title: string
                  ) => (
                    <>
                      <Text style={styles.reviewHeading}>{title}</Text>
                      {additionalAccessRows
                        .filter((row) => row.section === section)
                        .map((row) => {
                          const rowPressable = row.behavior !== "visible_non_clickable_status";
                          const granted = row.status === "granted";
                          const RowContainer: any = rowPressable ? Pressable : View;
                          const rowProps = rowPressable
                            ? {
                                onPress: () => handleAdditionalAccessRowPress(row),
                                accessibilityRole: "button",
                                disabled: runtimeBusy,
                                style: ({ pressed }: any) => [styles.checkRow, granted && styles.checkRowGranted, pressed && styles.checkRowPressed]
                              }
                            : { style: [styles.checkRow, granted && styles.checkRowGranted] };

                          return (
                            <RowContainer key={row.id} {...rowProps}>
                              <View style={{ flex: 1 }}>
                                <Text style={[styles.checkLabel, granted ? styles.checkLabelGranted : styles.checkLabelMissing]}>
                                  {row.label}
                                </Text>
                                {row.actionHint ? (
                                  <Text
                                    style={
                                      granted
                                        ? styles.checkSubLabel
                                        : styles.checkSubLabelWarn
                                    }
                                  >
                                    {row.actionHint}
                                  </Text>
                                ) : null}
                              </View>
                              <View style={[styles.checkBadge, granted ? styles.checkBadgeGranted : styles.checkBadgeMissing]}>
                                <Text style={styles.checkBadgeText}>{granted ? "✓" : "•"}</Text>
                              </View>
                            </RowContainer>
                          );
                        })}
                    </>
                  );

                  return (
                    <>
                <ScrollView
                  style={styles.reviewScroll}
                  contentContainerStyle={styles.reviewContent}
                  nestedScrollEnabled
                  showsVerticalScrollIndicator
                  onLayout={(event) => {
                    reviewLayoutHeightRef.current = event.nativeEvent.layout.height;
                  }}
                  onContentSizeChange={(_w, h) => {
                    reviewContentHeightRef.current = h;
                    const fitsWithoutScroll = reviewLayoutHeightRef.current > 0 && h <= reviewLayoutHeightRef.current + 1;
                    if (fitsWithoutScroll) {
                      setReviewAtBottom(true);
                    }
                  }}
                  onScroll={(event) => {
                    const tolerance = 48;
                    const { layoutMeasurement, contentOffset, contentSize } = event.nativeEvent;
                    const distanceFromBottom = contentSize.height - (layoutMeasurement.height + contentOffset.y);
                    if (Math.ceil(distanceFromBottom) <= tolerance) setReviewAtBottom(true);
                  }}
                  onScrollEndDrag={(event) => {
                    const tolerance = 48;
                    const { layoutMeasurement, contentOffset, contentSize } = event.nativeEvent;
                    const distanceFromBottom = contentSize.height - (layoutMeasurement.height + contentOffset.y);
                    if (Math.ceil(distanceFromBottom) <= tolerance) setReviewAtBottom(true);
                  }}
                  onMomentumScrollEnd={(event) => {
                    const tolerance = 48;
                    const { layoutMeasurement, contentOffset, contentSize } = event.nativeEvent;
                    const distanceFromBottom = contentSize.height - (layoutMeasurement.height + contentOffset.y);
                    if (Math.ceil(distanceFromBottom) <= tolerance) setReviewAtBottom(true);
                  }}
                  scrollEventThrottle={50}
                >
                  {renderSectionRows("required_now_checked_now", "Required now and checked now")}
                  {renderSectionRows("requested_later_features", "May be requested later for specific features")}
                  {renderSectionRows("internal_runtime_capabilities", "Internal app/runtime capabilities")}
                </ScrollView>

                <View style={{ height: 12 }} />
                <Pressable
                  style={[
                    styles.ackPill,
                    (!acknowledgeEnabled || runtimeBusy) && styles.linkButtonDisabled,
                    reviewAcknowledged && styles.ackPillSelected
                  ]}
                  disabled={!acknowledgeEnabled || runtimeBusy}
                  onPress={() => setReviewAcknowledged(true)}
                >
                  <Text style={[styles.ackPillText, reviewAcknowledged && styles.ackPillTextSelected]}>
                    I understand
                  </Text>
                </Pressable>

                <View style={styles.actionStack}>
                  <Pressable
                    style={[
                      styles.linkButton,
                      (!continueEnabled || runtimeBusy) && styles.linkButtonDisabled
                    ]}
                    disabled={!continueEnabled || runtimeBusy}
                    onPress={async () => {
                      setRuntimeBusy(true);
                      try {
                        const status = await refreshRuntimeStatus();
                        await computeChecklist(status);
                        const latestSnapshot = await getXMiloclawSetupPermissionSnapshot().catch(() => setupPermissionSnapshot);
                        if (latestSnapshot) setSetupPermissionSnapshot(latestSnapshot);
                        const missingAfterRefresh: string[] = [];
                        const latestRows = latestSnapshot?.rows ?? [];
                        if (!latestRows.find((row) => row.row === "camera")?.complete) missingAfterRefresh.push("Camera access needs While using the app.");
                        if (!latestRows.find((row) => row.row === "microphone")?.complete) missingAfterRefresh.push("Microphone access needs While using the app.");
                        if (!latestRows.find((row) => row.row === "media")?.complete) missingAfterRefresh.push("Photos and videos need full access, not limited access.");
                        if (!latestRows.find((row) => row.row === "location")?.complete) missingAfterRefresh.push("Location needs Precise and While using the app.");
                        if (!latestRows.find((row) => row.row === "physical_activity")?.complete) missingAfterRefresh.push("Physical activity access is required.");
                        if (latestSnapshot?.gate.foreground_runtime_complete !== true) missingAfterRefresh.push("Foreground runtime service must be running.");
                        if (!reviewAtBottom) {
                          logSetupPermissionBreadcrumb("finish_blocked", { stage: "additional_access", reason: "review_incomplete" });
                          setRuntimeError("Review the full page before continuing.");
                          return;
                        }
                        if (missingAfterRefresh.length > 0) {
                          const blockedReason = latestSnapshot?.gate.foreground_runtime_complete !== true
                            ? "foreground_runtime_incomplete"
                            : "permissions_incomplete";
                          logSetupPermissionBreadcrumb("finish_blocked", { stage: "additional_access", reason: blockedReason });
                          setReviewAcknowledged(false);
                          setRuntimeError(`Still missing: ${missingAfterRefresh.join(", ")}`);
                          return;
                        }
                        if (!isSetupPermissionFinalReady(latestSnapshot, reviewAtBottom)) {
                          logSetupPermissionBreadcrumb("finish_blocked", { stage: "additional_access", reason: "permissions_incomplete" });
                          setReviewAcknowledged(false);
                          setRuntimeError(`Still missing: ${(latestSnapshot?.gate.blocked_reasons ?? ["native permission proof"]).join(", ")}`);
                          return;
                        }
                        logSetupPermissionBreadcrumb("additional_access_finish_ready", { stage: "additional_access" });
                        setRuntimeError("");
                        setRuntimeStep("prepare_hidden_runtime_host");
                      } finally {
                        setRuntimeBusy(false);
                      }
                    }}
                  >
                    <Text style={styles.linkButtonText}>Continue</Text>
                  </Pressable>
                  <Pressable
                    style={[styles.linkButton, runtimeBusy && styles.linkButtonDisabled]}
                    disabled={runtimeBusy}
                    onPress={recheckSetupPermissions}
                  >
                    <Text style={styles.linkButtonText}>{runtimeBusy ? "Rechecking permissions..." : "Recheck permissions"}</Text>
                  </Pressable>
                </View>
                {requiredRowsMissing.length > 0 ? (
                  <Text style={styles.missingNow}>Still missing now{"\n- " + requiredRowsMissing.join("\n- ")}</Text>
                ) : null}
                    </>
                  );
                })()}
              </View>
            ) : null}

            {nativeRuntimeAvailable && runtimeStep === "prepare_hidden_runtime_host" ? (
              <View style={styles.card}>
                <Text style={styles.cardTitle}>Prepare hidden runtime host</Text>
                <Text style={styles.body}>
                  Start the app-owned hidden runtime host and verify the on-device proof surfaces.
                </Text>
                <Text style={styles.fine}>
                  Service: {(runtimeProof.foregroundServiceStarted ?? runtimeProof.runtimeHostStarted) ? "PASS" : "MISSING"} · Files:{" "}
                  {runtimeProof.runtimeFilesPrepared ? "PASS" : "MISSING"} · Process: {runtimeProof.sidecarProcessAlive ? "PASS" : "MISSING"} · Bridge:{" "}
                  {runtimeProof.bridgeConnected ? "PASS" : "MISSING"} · Ready route: {runtimeProof.taskRouteSurfaceReady ? "PASS" : "MISSING"} · Host ready:{" "}
                  {runtimeProof.hostReady ? "PASS" : "MISSING"}
                </Text>
                <Text style={styles.fine}>
                  Process exit: {runtimeProof.lastProcessExitCode ?? "—"} · stderr: {runtimeProof.firstSafeStderrCategory || "none"} · stdout:{" "}
                  {runtimeProof.firstSafeStdoutCategory || "none"}
                </Text>
                <Text style={styles.fine}>
                  LLM: {runtimeProof.llmMode || "unknown"} · BYOK provider: {runtimeProof.byokProvider || "—"} · First task:{" "}
                  {runtimeProof.firstTaskEligible ? "ELIGIBLE" : "NOT READY"} · Local turn:{" "}
                  {runtimeProof.localLlmTurnAllowed ? "YES" : "NO"} · Subscription: {runtimeProof.subscriptionEntitled ? "YES" : "NO"}
                </Text>
                {runtimeProof.lastProcessErrorSummary ? <Text style={styles.fine}>Process summary: {runtimeProof.lastProcessErrorSummary}</Text> : null}
                {runtimeError ? <Text style={styles.error}>{runtimeError}</Text> : null}
                <View style={styles.actionStack}>
                  <Pressable
                    style={[styles.linkButton, runtimeBusy && styles.linkButtonDisabled]}
                    disabled={runtimeBusy}
                    onPress={handleStartHiddenRuntimeHost}
                  >
                    <Text style={styles.linkButtonText}>{runtimeBusy ? "Starting..." : "Start hidden runtime host"}</Text>
                  </Pressable>
                </View>
              </View>
            ) : null}

            {nativeRuntimeAvailable && runtimeStep === "return_to_xmilo" ? (
              <View style={styles.card}>
                <Text style={styles.cardTitle}>Return to xMilo</Text>
                <Text style={styles.body}>{returnReason || "Return to xMilo and re-check to continue."}</Text>
                {runtimeError ? <Text style={styles.error}>{runtimeError}</Text> : null}
                <View style={styles.actionStack}>
                  <Pressable style={styles.linkButton} onPress={() => setRuntimeStep("re_check")}>
                    <Text style={styles.linkButtonText}>Re-check</Text>
                  </Pressable>
                </View>
              </View>
            ) : null}

            {nativeRuntimeAvailable && runtimeStep === "re_check" ? (
              <View style={styles.card}>
                <Text style={styles.cardTitle}>Finishing setup</Text>
                <Text style={styles.body}>xMilo is confirming the hidden runtime host connection.</Text>

                <View style={{ gap: 10, marginTop: 6 }}>
                  <View style={styles.checkRow}>
                    <Text style={[styles.checkLabel, (runtimeProof.foregroundServiceStarted ?? runtimeProof.runtimeHostStarted) ? styles.checkLabelGranted : styles.checkLabelMissing]}>
                      Foreground service started
                    </Text>
                    <View
                      style={[
                        styles.checkBadge,
                        (runtimeProof.foregroundServiceStarted ?? runtimeProof.runtimeHostStarted) ? styles.checkBadgeGranted : styles.checkBadgeMissing
                      ]}
                    >
                      <Text style={styles.checkBadgeText}>{(runtimeProof.foregroundServiceStarted ?? runtimeProof.runtimeHostStarted) ? "✓" : "—"}</Text>
                    </View>
                  </View>

                  <View style={styles.checkRow}>
                    <Text style={[styles.checkLabel, runtimeProof.runtimeFilesPrepared ? styles.checkLabelGranted : styles.checkLabelMissing]}>
                      Runtime files prepared
                    </Text>
                    <View
                      style={[
                        styles.checkBadge,
                        runtimeProof.runtimeFilesPrepared ? styles.checkBadgeGranted : styles.checkBadgeMissing
                      ]}
                    >
                      <Text style={styles.checkBadgeText}>{runtimeProof.runtimeFilesPrepared ? "✓" : "—"}</Text>
                    </View>
                  </View>

                  <View style={styles.checkRow}>
                    <Text style={[styles.checkLabel, runtimeProof.sidecarProcessAlive ? styles.checkLabelGranted : styles.checkLabelMissing]}>
                      Sidecar process alive
                    </Text>
                    <View
                      style={[
                        styles.checkBadge,
                        runtimeProof.sidecarProcessAlive ? styles.checkBadgeGranted : styles.checkBadgeMissing
                      ]}
                    >
                      <Text style={styles.checkBadgeText}>{runtimeProof.sidecarProcessAlive ? "✓" : "—"}</Text>
                    </View>
                  </View>

                  <View style={styles.checkRow}>
                    <Text style={[styles.checkLabel, runtimeProof.bridgeConnected ? styles.checkLabelGranted : styles.checkLabelMissing]}>
                      Bridge health
                    </Text>
                    <View
                      style={[
                        styles.checkBadge,
                        runtimeProof.bridgeConnected ? styles.checkBadgeGranted : styles.checkBadgeMissing
                      ]}
                    >
                      <Text style={styles.checkBadgeText}>{runtimeProof.bridgeConnected ? "✓" : "—"}</Text>
                    </View>
                  </View>

                  <View style={styles.checkRow}>
                    <Text style={[styles.checkLabel, runtimeProof.taskRouteSurfaceReady ? styles.checkLabelGranted : styles.checkLabelMissing]}>
                      Task route surface ready
                    </Text>
                    <View
                      style={[
                        styles.checkBadge,
                        runtimeProof.taskRouteSurfaceReady ? styles.checkBadgeGranted : styles.checkBadgeMissing
                      ]}
                    >
                      <Text style={styles.checkBadgeText}>{runtimeProof.taskRouteSurfaceReady ? "✓" : "—"}</Text>
                    </View>
                  </View>

                  <View style={styles.checkRow}>
                    <Text style={[styles.checkLabel, runtimeProof.hostReady ? styles.checkLabelGranted : styles.checkLabelMissing]}>
                      Host ready
                    </Text>
                    <View style={[styles.checkBadge, runtimeProof.hostReady ? styles.checkBadgeGranted : styles.checkBadgeMissing]}>
                      <Text style={styles.checkBadgeText}>{runtimeProof.hostReady ? "✓" : "—"}</Text>
                    </View>
                  </View>
                </View>

                <Text style={styles.fine}>
                  Stage: {runtimeProof.lastRuntimeStage || "unknown"} · Health: {runtimeProof.lastHealthCode ?? "—"} {runtimeProof.lastHealthCategory || "unknown"} · Ready:{" "}
                  {runtimeProof.lastReadyCode ?? "—"} {runtimeProof.lastReadyCategory || "unknown"}
                </Text>
                <Text style={styles.fine}>
                  Process exit: {runtimeProof.lastProcessExitCode ?? "—"} · Uptime: {runtimeProof.lastProcessUptimeMillis ?? "—"} ms · stderr:{" "}
                  {runtimeProof.firstSafeStderrCategory || "none"} · stdout: {runtimeProof.firstSafeStdoutCategory || "none"}
                </Text>
                <Text style={styles.fine}>
                  LLM: {runtimeProof.llmMode || "unknown"} · BYOK provider: {runtimeProof.byokProvider || "—"} · BYOK active:{" "}
                  {runtimeProof.bringYourOwnKeyActive ? "YES" : "NO"} · Phase 9 key access:{" "}
                  {runtimeProof.phase9ApiKeyAccess ? "YES" : "NO"} · First task: {runtimeProof.firstTaskEligible ? "ELIGIBLE" : "NOT READY"} · Relay turn:{" "}
                  {runtimeProof.relayLlmTurnAllowed ? "YES" : "NO"} · Local turn: {runtimeProof.localLlmTurnAllowed ? "YES" : "NO"} · Subscription:{" "}
                  {runtimeProof.subscriptionEntitled ? "YES" : "NO"}
                </Text>
                {runtimeProof.lastProcessErrorSummary ? <Text style={styles.fine}>Process summary: {runtimeProof.lastProcessErrorSummary}</Text> : null}

                {runtimeError ? <Text style={styles.error}>{runtimeError}</Text> : null}

                {finalConfirmCheckedOnce && getFinalRuntimeMissing(runtimeStatus).length > 0 ? (
                  <>
                    <Text style={styles.missingNow}>Still missing now</Text>
                    <Text style={styles.fine}>{"- " + getFinalRuntimeMissing(runtimeStatus).join("\n- ")}</Text>
                    <View style={styles.actionStack}>
                      <Pressable
                        style={[styles.linkButton, runtimeBusy && styles.linkButtonDisabled]}
                        disabled={runtimeBusy}
                        onPress={runFinalRuntimeConfirmation}
                      >
                        <Text style={styles.linkButtonText}>Try again</Text>
                      </Pressable>
                    </View>
                  </>
                ) : null}
              </View>
            ) : null}
          </>
        ) : null}

        <View style={styles.waitRow}>
          <ActivityIndicator color={CLR.accent} size="small" />
          <Text style={styles.muted}>  Waiting for xMilo setup…</Text>
        </View>
      </ScrollView>
    );
  }

  if (step === "byok_choose_provider") {
    return (
      <ScrollView style={styles.scroll} contentContainerStyle={styles.padded}>
        <Text style={styles.title}>Choose provider</Text>
        <Text style={styles.body}>Your provider key stays local on this phone. Hosted access stays separate.</Text>
        {!!error && <Text style={styles.error}>{error}</Text>}

        <View style={styles.card}>
          {providerOptionsBusy ? <ActivityIndicator color={CLR.accent} /> : null}
          {localProviderOptions.map((option) => (
            <Pressable
              key={option.id}
              style={[styles.linkButton, { marginTop: 10 }]}
              onPress={() => {
                setError("");
                setByokProvider(option.id);
                setByokKey("");
                setByokBaseUrl("");
                setByokModel("");
                setStep("byok_enter_key");
              }}
            >
              <Text style={styles.linkButtonText}>{option.label}</Text>
            </Pressable>
          ))}
        </View>

        <Pressable style={styles.textBtn} onPress={() => setStep("email")}>
          <Text style={styles.textBtnLabel}>← Back</Text>
        </Pressable>
      </ScrollView>
    );
  }

  if (step === "byok_enter_key") {
    const providerConfig = byokProvider ? localProviderOptions.find((option) => option.id === byokProvider) : null;
    const keyRequired = providerConfig?.key_required ?? false;
    const showBaseUrlInput = Boolean(providerConfig?.custom_base_url_allowed || (providerConfig?.base_url_required && !providerConfig.default_base_url));
    return (
      <ScrollView style={styles.scroll} contentContainerStyle={styles.padded}>
        <Text style={styles.title}>{keyRequired ? "Enter your API key" : "Configure local provider"}</Text>
        <Text style={styles.body}>
          Provider: <Text style={styles.inlineAccent}>{providerConfig?.label ?? "—"}</Text>
        </Text>
        <Text style={styles.body}>Provider rules are resolved by the local sidecar before the key is saved.</Text>

        <TextInput
          style={styles.input}
          placeholder={keyRequired ? "API key" : "API key (optional)"}
          placeholderTextColor={CLR.muted}
          value={byokKey}
          onChangeText={setByokKey}
          autoCapitalize="none"
          autoCorrect={false}
          secureTextEntry
        />

        {showBaseUrlInput ? (
          <TextInput
            style={styles.input}
            placeholder="Base URL"
            placeholderTextColor={CLR.muted}
            value={byokBaseUrl}
            onChangeText={setByokBaseUrl}
            autoCapitalize="none"
            autoCorrect={false}
            keyboardType="url"
          />
        ) : (
          <Text style={styles.fine}>Base URL: {providerConfig?.default_base_url || "sidecar default"}</Text>
        )}

        <Text style={styles.fine}>Model: {providerConfig?.default_model || "sidecar default"}</Text>

        <Btn label="Save & continue" onPress={handleByokSaveAndContinue} busy={byokBusy} />

        <Pressable style={styles.textBtn} onPress={() => setStep("byok_choose_provider")}>
          <Text style={styles.textBtnLabel}>← Back</Text>
        </Pressable>
      </ScrollView>
    );
  }

  if (step === "email") {
    return (
      <ScrollView style={styles.scroll} contentContainerStyle={styles.padded}>
        <Text style={styles.title}>Welcome to xMilo</Text>
        <Text style={styles.body}>
          Enter your email to get started. We'll send a one-click verification
          link — no password, no friction.
        </Text>

        <TextInput
          style={styles.input}
          placeholder="you@example.com"
          placeholderTextColor={CLR.muted}
          value={email}
          onChangeText={setEmail}
          autoCapitalize="none"
          autoCorrect={false}
          keyboardType="email-address"
          returnKeyType="done"
          onSubmitEditing={handleSendEmail}
        />

        {!!error && <Text style={styles.error}>{error}</Text>}

        <Btn label="Use xMilo hosted access" onPress={handleSendEmail} busy={busy} />
        <Btn
          label="Use my own API key"
          onPress={() => {
            setError("");
            setByokProvider("");
            setByokKey("");
            setByokBaseUrl("");
            setByokModel("");
            setStep("byok_choose_provider");
          }}
          busy={false}
        />

        <Text style={styles.fine}>
          Your email is used only for account verification and critical service
          notices. No marketing email.
        </Text>
      </ScrollView>
    );
  }

  if (step === "email_sent") {
    return (
      <ScrollView style={styles.scroll} contentContainerStyle={styles.padded}>
        <Text style={styles.title}>Check your inbox</Text>
        <Text style={styles.body}>
          We sent a verification link to{"\n"}
          <Text style={{ color: CLR.accent }}>{email}</Text>
          {"\n\n"}
          Open the email on this phone and tap the link. Then come back here.
        </Text>

        {verifyAttempts > 0 && !error && (
          <Text style={[styles.body, { color: CLR.warn }]}>
            ⚠ Link not clicked yet, or check your spam folder.
          </Text>
        )}

        {!!error && <Text style={styles.error}>{error}</Text>}

        <Btn
          label="I verified my email ✓"
          onPress={handleCheckVerified}
          busy={busy}
        />

        <Pressable
          style={styles.textBtn}
          onPress={() => { setError(""); setStep("email"); }}
        >
          <Text style={styles.textBtnLabel}>← Use a different email</Text>
        </Pressable>
      </ScrollView>
    );
  }

  if (step === "access_choice") {
    return (
      <ScrollView style={styles.scroll} contentContainerStyle={styles.padded}>
        <Text style={styles.title}>How do you want to start?</Text>
        <Text style={styles.body}>
          {accessConfig.access_code_only
            ? `Email verified. Launch access is code-only right now. Redeem an access code to begin.`
            : "Email verified. Choose your access."}
        </Text>

        <ChoiceCard
          title="Redeem access code"
          sub={`Use a giveaway or website-issued code to unlock ${String(accessConfig.access_code_grant_days ?? 30)} days of access during launch.`}
          accent={CLR.accent}
          onPress={() => { setError(""); setStep("invite_input"); }}
        />

        {accessConfig.trial_allowed ? (
          <ChoiceCard
            title="Free trial — 12 hours"
            sub="Start now. Timer begins on your first real task."
            accent={CLR.success}
            onPress={handleStartTrial}
          />
        ) : null}

        {subscriptionEntryVisible ? (
          <ChoiceCard
            title="Subscribe  ·  $19.99 / month"
            sub="Unlimited access billed via Google Play."
            accent={CLR.warn}
            onPress={handleOpenSubscription}
            busy={busy}
          />
        ) : (
          <Text style={styles.fine}>
            {accessConfig.subscription_allowed
              ? "Subscriptions are allowed by the relay, but this build is not configured for the Google Play purchase path yet. For now, use a trial or access code."
              : "Public subscriptions stay hidden during launch. For now, get your access code separately and bring it back here."}
          </Text>
        )}
      </ScrollView>
    );
  }

  if (step === "two_factor") {
    return (
      <ScrollView style={styles.scroll} contentContainerStyle={styles.padded}>
        <Text style={styles.title}>One more lock to open</Text>
        <Text style={styles.body}>
          This verified email already has 2FA enabled. To trust this phone, enter the current authenticator code or one unused recovery code.
        </Text>

        <TextInput
          style={styles.input}
          placeholder="Authenticator code"
          placeholderTextColor={CLR.muted}
          value={twoFactorCode}
          onChangeText={setTwoFactorCode}
          keyboardType="number-pad"
          autoCorrect={false}
          returnKeyType="done"
        />

        <TextInput
          style={styles.input}
          placeholder="Recovery code"
          placeholderTextColor={CLR.muted}
          value={recoveryCode}
          onChangeText={setRecoveryCode}
          autoCapitalize="characters"
          autoCorrect={false}
          returnKeyType="done"
        />

        {!!error && <Text style={styles.error}>{error}</Text>}

        <Btn label="Verify this phone" onPress={handleVerifyTwoFactor} busy={busy} />

        <Text style={styles.fine}>
          This replaces the previously trusted phone for 2FA-sensitive actions like website access and future account recovery.
        </Text>
      </ScrollView>
    );
  }

  if (step === "invite_input") {
    return (
      <ScrollView style={styles.scroll} contentContainerStyle={styles.padded}>
        <Text style={styles.title}>Enter code or Skip for now</Text>
        <Text style={styles.body}>
          Access codes are single-use and currently grant {String(accessConfig.access_code_grant_days ?? 30)} days of full access during launch.
        </Text>

        <TextInput
          style={[styles.input, { letterSpacing: 3, textTransform: "uppercase" }]}
          placeholder="XXXXXXXX"
          placeholderTextColor={CLR.muted}
          value={inviteCode}
          onChangeText={setInviteCode}
          autoCapitalize="characters"
          autoCorrect={false}
          returnKeyType="done"
          onSubmitEditing={handleRedeemInvite}
        />

        {!!error && <Text style={styles.error}>{error}</Text>}

        <Btn label="Redeem code" onPress={handleRedeemInvite} busy={busy} />

        <Pressable
          style={styles.textBtn}
          onPress={() => router.replace("/lair")}
        >
          <Text style={styles.textBtnLabel}>Skip for now →</Text>
        </Pressable>
      </ScrollView>
    );
  }

  return null;
}

function StepRow({ n, text, sub }: { n: string; text: string; sub: string }) {
  return (
    <View style={styles.stepRow}>
      <View style={styles.stepBadge}>
        <Text style={styles.stepNum}>{n}</Text>
      </View>
      <View style={{ flex: 1 }}>
        <Text style={styles.stepText}>{text}</Text>
        <Text style={styles.stepSub}>{sub}</Text>
      </View>
    </View>
  );
}

function ChoiceCard({
  title,
  sub,
  accent,
  onPress,
  busy = false
}: {
  title: string;
  sub: string;
  accent: string;
  onPress: () => void;
  busy?: boolean;
}) {
  return (
    <Pressable
      style={({ pressed }) => [
        styles.choiceCard,
        { borderLeftColor: accent, opacity: pressed || busy ? 0.65 : 1 }
      ]}
      onPress={onPress}
      disabled={busy}
    >
      <View style={{ flexDirection: "row", alignItems: "center", gap: 8 }}>
        <Text style={[styles.choiceTitle, { color: accent, flex: 1 }]}>{title}</Text>
        {busy && <ActivityIndicator size="small" color={accent} />}
      </View>
      <Text style={styles.choiceSub}>{sub}</Text>
    </Pressable>
  );
}

function Btn({
  label,
  onPress,
  busy
}: {
  label: string;
  onPress: () => void;
  busy: boolean;
}) {
  return (
    <Pressable
      style={({ pressed }) => [
        styles.btn,
        { opacity: pressed || busy ? 0.65 : 1 }
      ]}
      onPress={onPress}
      disabled={busy}
    >
      {busy
        ? <ActivityIndicator color="#fff" />
        : <Text style={styles.btnLabel}>{label}</Text>
      }
    </Pressable>
  );
}

const styles = StyleSheet.create({
  scroll: { flex: 1, backgroundColor: CLR.bg },
  padded: { padding: 20, gap: 16, paddingBottom: 48 },
  center: {
    flex: 1, backgroundColor: CLR.bg,
    alignItems: "center", justifyContent: "center", gap: 12
  },
  title: { color: CLR.title, fontSize: 26, fontWeight: "700", lineHeight: 32 },
  body:  { color: CLR.body, fontSize: 15, lineHeight: 23 },
  muted: { color: CLR.muted, fontSize: 14, textAlign: "center" },
  fine:  { color: CLR.muted, fontSize: 12, lineHeight: 18, marginTop: 4 },
  error: { color: CLR.danger, fontSize: 14, lineHeight: 20 },

  card: {
    backgroundColor: CLR.card,
    borderRadius: 16, padding: 16, gap: 12,
    borderWidth: 1, borderColor: CLR.border
  },
  trustCard: {
    backgroundColor: "#111827",
    borderRadius: 24,
    padding: 24,
    gap: 18,
    borderWidth: 1,
    borderColor: "#334155"
  },
  trustKicker: {
    color: "#FCD34D",
    fontSize: 14,
    fontWeight: "800",
    letterSpacing: 1.2,
    textTransform: "uppercase"
  },
  trustTitle: {
    color: CLR.title,
    fontSize: 34,
    fontWeight: "800",
    lineHeight: 40
  },
  trustBody: {
    color: "#E2E8F0",
    fontSize: 20,
    lineHeight: 30,
    fontWeight: "600"
  },
  trustPager: {
    color: CLR.muted,
    fontSize: 13,
    fontWeight: "700"
  },
  trustActions: {
    flexDirection: "row",
    gap: 10,
    alignItems: "center"
  },
  trustPrimary: {
    flex: 1
  },
  cardTitle: { color: CLR.title, fontSize: 16, fontWeight: "600" },
  inlineAccent: { color: CLR.accent, fontWeight: "700" },
  actionStack: { gap: 10, marginTop: 4 },
  linkButton: {
    backgroundColor: "#1F2937",
    borderRadius: 12,
    paddingVertical: 12,
    paddingHorizontal: 14,
    borderWidth: 1,
    borderColor: "#334155"
  },
  linkButtonDisabled: {
    opacity: 0.45
  },
  linkButtonText: { color: "#E2E8F0", fontWeight: "600" },
  checklistScroll: {
    maxHeight: 360,
    borderRadius: 14,
    borderWidth: 1,
    borderColor: CLR.border,
    backgroundColor: "#0B1224"
  },
  checklistContent: {
    padding: 14,
    gap: 10
  },
  checkRow: {
    flexDirection: "row",
    alignItems: "center",
    justifyContent: "space-between",
    gap: 12,
    paddingVertical: 10,
    paddingHorizontal: 12,
    borderRadius: 12,
    backgroundColor: "#0F172A",
    borderWidth: 1,
    borderColor: "#1F2937"
  },
  checkRowPressed: {
    backgroundColor: "#0B1224",
    borderColor: "#334155"
  },
  checkRowGranted: {
    backgroundColor: "#052E22",
    borderColor: "#10B981"
  },
  checkLabel: {
    flex: 1,
    fontSize: 13,
    lineHeight: 18
  },
  checkSubLabel: {
    color: "#64748B",
    fontSize: 12,
    lineHeight: 16,
    marginTop: 4
  },
  checkSubLabelWarn: {
    color: CLR.warn,
    fontSize: 12,
    lineHeight: 16,
    marginTop: 4
  },
  checkLabelGranted: {
    color: "#BBF7D0"
  },
  checkLabelMissing: {
    color: "#94A3B8"
  },
  checkBadge: {
    width: 24,
    height: 24,
    borderRadius: 12,
    alignItems: "center",
    justifyContent: "center",
    borderWidth: 1
  },
  checkBadgeGranted: {
    backgroundColor: "#052E22",
    borderColor: "#10B981"
  },
  checkBadgeMissing: {
    backgroundColor: "#0B1224",
    borderColor: "#334155"
  },
  checkBadgeText: {
    color: "#E2E8F0",
    fontSize: 14,
    fontWeight: "900"
  },
  reviewScroll: {
    maxHeight: 320,
    borderRadius: 14,
    borderWidth: 1,
    borderColor: CLR.border,
    backgroundColor: "#0B1224"
  },
  reviewContent: {
    padding: 14,
    gap: 10
  },
  reviewHeading: {
    color: CLR.title,
    fontSize: 14,
    fontWeight: "800",
    marginTop: 4
  },
  reviewItem: {
    color: CLR.body,
    fontSize: 13,
    lineHeight: 20
  },
  ackPill: {
    borderRadius: 999,
    borderWidth: 1,
    borderColor: "#334155",
    backgroundColor: "#0B1224",
    paddingVertical: 12,
    paddingHorizontal: 14
  },
  ackPillSelected: {
    borderColor: CLR.success,
    backgroundColor: "#052E22"
  },
  ackPillText: {
    color: CLR.body,
    fontSize: 13,
    fontWeight: "600",
    textAlign: "center"
  },
  ackPillTextSelected: {
    color: "#BBF7D0"
  },
  missingNow: {
    color: CLR.warn,
    fontSize: 13,
    lineHeight: 20,
    marginTop: 12,
    marginBottom: 8
  },

  mono: {
    color: "#A5F3FC", fontFamily: "monospace", fontSize: 13,
    backgroundColor: "#0F172A", padding: 10, borderRadius: 8, lineHeight: 20
  },

  stepRow: { flexDirection: "row", gap: 12, alignItems: "flex-start" },
  stepBadge: {
    width: 28, height: 28, borderRadius: 14,
    backgroundColor: CLR.accentDim,
    alignItems: "center", justifyContent: "center"
  },
  stepNum:  { color: CLR.title, fontWeight: "700", fontSize: 14 },
  stepText: { color: CLR.title, fontSize: 15, fontWeight: "500" },
  stepSub:  { color: CLR.muted, fontSize: 13, lineHeight: 18, marginTop: 2 },

  waitRow: {
    flexDirection: "row", alignItems: "center",
    justifyContent: "center", marginTop: 8
  },

  input: {
    backgroundColor: CLR.input,
    borderWidth: 1, borderColor: CLR.inputBorder,
    borderRadius: 12, paddingHorizontal: 16, paddingVertical: 13,
    color: CLR.title, fontSize: 16
  },

  btn: {
    backgroundColor: CLR.accent, borderRadius: 12,
    paddingVertical: 14, alignItems: "center",
    justifyContent: "center", minHeight: 50
  },
  btnLabel: { color: "#fff", fontSize: 16, fontWeight: "600" },

  textBtn:      { alignItems: "center", paddingVertical: 10 },
  textBtnLabel: { color: CLR.accent, fontSize: 15 },

  choiceCard: {
    backgroundColor: CLR.card, borderRadius: 14, padding: 16, gap: 4,
    borderWidth: 1, borderColor: CLR.border, borderLeftWidth: 4
  },
  choiceTitle: { fontSize: 16, fontWeight: "600" },
  choiceSub:   { color: CLR.body, fontSize: 14, lineHeight: 20 }
});
