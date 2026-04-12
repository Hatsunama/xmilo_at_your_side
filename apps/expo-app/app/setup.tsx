import { useRouter } from "expo-router";
import { useEffect, useRef, useState } from "react";
import {
  ActivityIndicator,
  Alert,
  BackHandler,
  AppState,
  Linking,
  PermissionsAndroid,
  Pressable,
  ScrollView,
  StyleSheet,
  Text,
  TextInput,
  View
} from "react-native";
import * as SecureStore from "expo-secure-store";
import {
  authCheck,
  authRedeemInvite,
  authRegister,
  verifyTwoFactor,
  getState
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
  openXMiloclawNotificationSettings,
  openXMiloclawOverlaySettings,
  restartXMiloclawRuntimeHost,
  startXMiloclawRuntimeHost,
  type XMiloclawRuntimeStatus
} from "../src/lib/xmiloRuntimeHost";
import { hasSetupCompletedOnce, markSetupCompletedOnce } from "../src/lib/setupCompletion";

// ─── Step machine ─────────────────────────────────────────────────────────────
// checking           → silent boot check (already entitled? skip wizard)
// waiting_sidecar    → hidden runtime-host startup flow
// email              → collect email address
// email_sent         → "check your inbox" + "I verified" polling
// access_choice      → access code only during launch, public choices later
// invite_input       → enter access code
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

type ByokProvider = "Grok" | "OpenAI" | "Claude";

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

type Page3RowClass =
  | "user_granted_checkable"
  | "required_platform_capability"
  | "required_but_not_yet_truthfully_mappable"
  | "replace_with_exact_android_surface";

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

type AllowBasicRowId =
  | "run_commands_terminal_environment"
  | "modify_delete_shared_storage"
  | "read_shared_storage"
  | "read_audio_files"
  | "show_notifications"
  | "accessibility_service_enabled"
  | "read_locations_from_media_collection"
  | "read_video_files"
  | "read_image_files"
  | "read_user_selected_images_videos"
  | "view_network_connections"
  | "full_network_access"
  | "prevent_phone_sleeping"
  | "control_vibration"
  | "run_foreground_service"
  | "run_background_service"
  | "ask_ignore_battery_optimizations"
  | "set_alarm"
  | "send_text_message"
  | "make_phone_call"
  | "access_to_call_logs"
  | "access_to_camera"
  | "access_to_content"
  | "access_to_files_media"
  | "access_to_health_fitness_wellness"
  | "access_to_location_all_the_time"
  | "access_to_microphone"
  | "access_to_music_audio"
  | "access_to_notifications"
  | "access_to_phone"
  | "access_to_photos_videos"
  | "access_to_sms"
  | "allow_background_data_usage"
  | "appear_on_top"
  | "unrestricted_battery_access";

type AllowBasicRowMeta = {
  id: AllowBasicRowId;
  label: string;
  rowClass: Page3RowClass;
  androidMapping?: string;
  blockingReason?: string;
  recommendedReplacementLabel?: string;
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

const BYOK_SECURESTORE_SELECTED_PROVIDER_KEY = "xmilo.byok.selectedProvider";
const BYOK_SECURESTORE_API_KEY_PREFIX = "xmilo.byok.apiKey.";

export default function SetupScreen() {
  const router = useRouter();
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
  const [byokProvider, setByokProvider] = useState<ByokProvider | "">("");
  const [byokKey, setByokKey] = useState("");
  const [byokBusy, setByokBusy] = useState(false);
  const [runtimeStep, setRuntimeStep] = useState<
    Page3RuntimeStep
  >("hidden_runtime_host_path");
  const [runtimeStatus, setRuntimeStatus] = useState<XMiloclawRuntimeStatus | null>(null);
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

  function setupSeamSatisfied(status: XMiloclawRuntimeStatus | null) {
    if (!status) return false;
    return (
      status.notificationsGranted &&
      status.appearOnTopGranted &&
      status.batteryUnrestricted &&
      status.accessibilityEnabled &&
      status.runtimeHostStarted &&
      status.bridgeConnected &&
      status.hostReady
    );
  }

  function resetSetupToStep1() {
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
  }

  function blockOnMissingSetupProof(message: string): false {
    setRuntimeStep("re_check");
    setRuntimeError(message);
    return false;
  }

  async function continueAfterSeamSatisfied() {
    // Startup canon: this seam is satisfied by app-owned runtime-host + bridge verification.
    // Deeper proof surfaces like `/auth/check` and `/state` are part of later, post-access phases.
    try {
      await markSetupCompletedOnce();
    } catch {
      // Keep setup usable even if the local "completed once" marker can't be written right now.
    }
    // Post-seam: enter the final startup access gate (code or skip).
    // This avoids the non-canon email/verification flow and avoids `/auth/check` at this stage.
    setStep("email");
    return true;
  }

  // ── Boot: check if already entitled, or wait for sidecar ─────────────────────
  useEffect(() => {
    async function boot() {
      const setupCompleted = await hasSetupCompletedOnce().catch(() => false);
      if (!setupCompleted) {
        resetSetupToStep1();
        setStep("waiting_sidecar");
        return;
      }

      // Start with setup-seam truth, then entitlement/auth after the seam is satisfied.
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
    return () => {};
  }, []); // eslint-disable-line react-hooks/exhaustive-deps

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

  async function refreshRuntimeStatus() {
    try {
      const status = await getXMiloclawRuntimeStatus();
      setRuntimeStatus(status);
      return status;
    } catch (error: any) {
      setRuntimeError(error?.message ?? "Could not read runtime host status.");
      return null;
    }
  }

  function getFinalRuntimeMissing(status: XMiloclawRuntimeStatus | null) {
    const snapshot =
      status ?? {
        notificationsGranted: false,
        appearOnTopGranted: false,
        batteryUnrestricted: false,
        accessibilityEnabled: false,
        runtimeHostStarted: false,
        bridgeConnected: false,
        hostReady: false
      };

    const missing: Array<"Runtime host started" | "Bridge connected" | "Host ready"> = [];
    if (!snapshot.runtimeHostStarted) missing.push("Runtime host started");
    if (!snapshot.bridgeConnected) missing.push("Bridge connected");
    if (!snapshot.hostReady) missing.push("Host ready");
    return missing;
  }

  async function runFinalRuntimeConfirmation() {
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
  }

  useEffect(() => {
    if (step !== "waiting_sidecar") return;
    refreshRuntimeStatus();
  }, [step]);

  useEffect(() => {
    if (step !== "waiting_sidecar" || runtimeStep !== "re_check") {
      finalConfirmAutoRanRef.current = false;
      setFinalConfirmCheckedOnce(false);
      return;
    }

    if (finalConfirmAutoRanRef.current) return;
    finalConfirmAutoRanRef.current = true;
    runFinalRuntimeConfirmation();
  }, [step, runtimeStep]); // eslint-disable-line react-hooks/exhaustive-deps

  // Page-3 stability/anti-regression: keep an in-memory snapshot so settings hops, app foregrounding,
  // or in-app route churn can't silently re-stack trust + page-3 cards or clear the page-3 checklist
  // within the same running process session.
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

  useEffect(() => {
    if (step !== "waiting_sidecar" || runtimeStep !== "allow_basic_permissions") return;
    computeChecklist(runtimeStatus);
    const sub = AppState.addEventListener("change", (nextState) => {
      if (nextState === "active") {
        refreshRuntimeStatus().then((status) => computeChecklist(status));
      }
    });
    return () => sub.remove();
  }, [runtimeStep, runtimeStatus, step]);

  async function computeChecklist(status: XMiloclawRuntimeStatus | null) {
    const snapshot =
      status ?? {
        notificationsGranted: false,
        appearOnTopGranted: false,
        batteryUnrestricted: false,
        accessibilityEnabled: false,
        runtimeHostStarted: false,
        bridgeConnected: false,
        hostReady: false
      };

    async function checkAndroidPermission(permission: string) {
      try {
        const ok = await PermissionsAndroid.check(permission as any);
        return ok ? "granted" : "missing";
      } catch {
        return "unknown";
      }
    }

    const entries: Array<Promise<[string, Page3PermissionStatus]>> = [];

    const set = (id: string, value: Page3PermissionStatus) => {
      entries.push(Promise.resolve([id, value]));
    };

    const checkPerm = (id: string, permission: string) => {
      entries.push(checkAndroidPermission(permission).then((value) => [id, value]));
    };

    set("show_notifications", snapshot.notificationsGranted ? "granted" : "missing");
    set("access_to_notifications", snapshot.notificationsGranted ? "granted" : "missing");
    set("appear_on_top", snapshot.appearOnTopGranted ? "granted" : "missing");
    set("unrestricted_battery_access", snapshot.batteryUnrestricted ? "granted" : "missing");
    set("ask_ignore_battery_optimizations", snapshot.batteryUnrestricted ? "granted" : "missing");
    set("accessibility_service_enabled", snapshot.accessibilityEnabled ? "granted" : "missing");
    set("run_commands_terminal_environment", snapshot.runtimeHostStarted ? "granted" : "missing");

    // Shared storage/media mapping (Android version dependent).
    // Note: WRITE_EXTERNAL_STORAGE is deprecated and not grantable on modern Android targets; we still check it
    // to truthfully reflect that the current row label is not satisfiable without a canon-level replacement.
    checkPerm("modify_delete_shared_storage", "android.permission.WRITE_EXTERNAL_STORAGE");
    checkPerm("read_shared_storage", "android.permission.READ_EXTERNAL_STORAGE");
    checkPerm("read_audio_files", "android.permission.READ_MEDIA_AUDIO");
    checkPerm("read_video_files", "android.permission.READ_MEDIA_VIDEO");
    checkPerm("read_image_files", "android.permission.READ_MEDIA_IMAGES");
    checkPerm("read_user_selected_images_videos", "android.permission.READ_MEDIA_VISUAL_USER_SELECTED");
    checkPerm("read_locations_from_media_collection", "android.permission.ACCESS_MEDIA_LOCATION");

    checkPerm("view_network_connections", "android.permission.ACCESS_NETWORK_STATE");
    checkPerm("full_network_access", "android.permission.INTERNET");
    checkPerm("prevent_phone_sleeping", "android.permission.WAKE_LOCK");
    checkPerm("control_vibration", "android.permission.VIBRATE");
    checkPerm("run_foreground_service", "android.permission.FOREGROUND_SERVICE");
    checkPerm("run_background_service", "android.permission.FOREGROUND_SERVICE");
    // Exact alarms are a special app-op on modern Android; PermissionsAndroid.check is not a full truth source here.
    // Keep blocking until a truthful AlarmManager.canScheduleExactAlarms() check is wired.
    set("set_alarm", "unknown");

    checkPerm("send_text_message", "android.permission.SEND_SMS");
    checkPerm("access_to_sms", "android.permission.READ_SMS");
    checkPerm("make_phone_call", "android.permission.CALL_PHONE");
    checkPerm("access_to_phone", "android.permission.READ_PHONE_STATE");
    checkPerm("access_to_call_logs", "android.permission.READ_CALL_LOG");

    checkPerm("access_to_camera", "android.permission.CAMERA");
    checkPerm("access_to_microphone", "android.permission.RECORD_AUDIO");
    entries.push(
      Promise.all([
        checkAndroidPermission("android.permission.ACCESS_FINE_LOCATION"),
        checkAndroidPermission("android.permission.ACCESS_BACKGROUND_LOCATION")
      ]).then(([fine, background]) => [
        "access_to_location_all_the_time",
        fine === "granted" && background === "granted" ? "granted" : fine === "missing" || background === "missing" ? "missing" : "unknown"
      ])
    );
    entries.push(checkAndroidPermission("android.permission.ACCESS_FINE_LOCATION").then((value) => ["additional_access_location", value]));

    // Inventory truth repairs (derived/umbrella rows)
    // access_to_music_audio -> mirrors audio file read
    // access_to_photos_videos -> mirrors image+video read (or shared storage read as legacy fallback)
    // access_to_content/access_to_files_media -> mirrors any media/shared-storage read signal
    // health/background-data remain blocking until a truthful mapping exists
    set("access_to_health_fitness_wellness", "unknown");
    set("allow_background_data_usage", "unknown");

    const results = await Promise.all(entries);
    const next: Record<string, Page3PermissionStatus> = {};
    for (const [id, value] of results) next[id] = value;

    // Derived/umbrella labels (best-effort mapping without faking grants)
    const anyMediaGranted =
      next.modify_delete_shared_storage === "granted" ||
      next.read_shared_storage === "granted" ||
      next.read_audio_files === "granted" ||
      next.read_image_files === "granted" ||
      next.read_video_files === "granted" ||
      next.read_user_selected_images_videos === "granted";

    next.access_to_music_audio = next.read_audio_files ?? "unknown";
    next.access_to_photos_videos =
      next.read_image_files === "granted" && next.read_video_files === "granted"
        ? "granted"
        : next.read_image_files === "missing" || next.read_video_files === "missing"
          ? "missing"
          : next.read_shared_storage === "granted"
            ? "granted"
            : "unknown";

    next.access_to_content = anyMediaGranted ? "granted" : next.read_shared_storage === "missing" ? "missing" : "unknown";
    next.access_to_files_media = anyMediaGranted ? "granted" : next.read_shared_storage === "missing" ? "missing" : "unknown";

    setPermissionChecklist(next);
    setChecklistUpdatedAtMillis(Date.now());
    return next;
  }

  async function requestAndroidPermission(permission: string) {
    try {
      const result = await PermissionsAndroid.request(permission as any);
      return result === PermissionsAndroid.RESULTS.GRANTED;
    } catch {
      return false;
    }
  }

  function syncRevenueCat(appUserID: string) {
    if (!appUserID || !isRevenueCatConfigured()) return;
    try {
      configureRevenueCat(appUserID);
    } catch {
      // keep setup usable even when RevenueCat is not configured yet
    }
  }

  async function waitForRelayEntitlement() {
    for (let i = 0; i < 6; i += 1) {
      const result = await authCheck().catch(() => null);
      if (result?.entitled) {
        router.replace("/");
        return true;
      }
      await new Promise((resolve) => setTimeout(resolve, 2000));
    }
    return false;
  }

  // ── Handlers ─────────────────────────────────────────────────────────────────

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
    // Trial clock starts on the first real /llm/turn call — nothing to do here.
    router.replace("/");
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
      router.replace("/");
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
    if (!trimmedKey) {
      Alert.alert("BYOK key not saved");
      return;
    }

    setByokBusy(true);
    try {
      await SecureStore.setItemAsync(BYOK_SECURESTORE_SELECTED_PROVIDER_KEY, byokProvider);
      await SecureStore.setItemAsync(`${BYOK_SECURESTORE_API_KEY_PREFIX}${byokProvider}`, trimmedKey);
      await markSetupCompletedOnce().catch(() => null);
      setByokKey("");
      // Continue straight into the Lair shell without leaving a modal over the live scene
      // and without preserving setup in the back stack.
      router.replace("/lair");
    } catch {
      Alert.alert("BYOK key not saved");
    } finally {
      setByokBusy(false);
    }
  }

  // ── Screens ───────────────────────────────────────────────────────────────────

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
        rowClass: "user_granted_checkable",
        androidMapping: "Android: POST_NOTIFICATIONS (app-owned proof surface)"
      },
      {
        id: "appear_on_top",
        label: "Appear on top",
        rowClass: "user_granted_checkable",
        androidMapping: "Android: overlay permission (app-owned proof surface)"
      },
      {
        id: "unrestricted_battery_access",
        label: "Unrestricted battery access",
        rowClass: "user_granted_checkable",
        androidMapping: "Android: Unrestricted battery access (app-owned proof surface)"
      },
      {
        id: "accessibility_service_enabled",
        label: "Accessibility Service enabled",
        rowClass: "user_granted_checkable",
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
        accessibilityEnabled: false,
        runtimeHostStarted: false,
        bridgeConnected: false,
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
        id: "camera_microphone",
        section: "requested_later_features",
        label: "Camera / Microphone",
        behavior: "clickable_runtime_request",
        requiredForContinue: false,
        status: combineStatuses(permissionChecklist.access_to_camera ?? "unknown", permissionChecklist.access_to_microphone ?? "unknown"),
        actionHint: "Tap to request camera and microphone permissions."
      },
      {
        id: "photos_videos",
        section: "requested_later_features",
        label: "Photos / Videos",
        behavior: "clickable_runtime_request",
        requiredForContinue: false,
        status:
          permissionChecklist.access_to_photos_videos ??
          combineStatuses(
            permissionChecklist.read_image_files ?? "unknown",
            permissionChecklist.read_video_files ?? "unknown"
          ),
        actionHint: "Tap to request media read permissions."
      },
      {
        id: "location",
        section: "requested_later_features",
        label: "Location",
        behavior: "clickable_runtime_request",
        requiredForContinue: false,
        status: permissionChecklist.additional_access_location ?? "unknown",
        actionHint: "Tap to request precise location permission."
      },
      {
        id: "phone",
        section: "requested_later_features",
        label: "Phone",
        behavior: "clickable_runtime_request",
        requiredForContinue: false,
        status: permissionChecklist.access_to_phone ?? "unknown",
        actionHint: "Tap to request phone-state permission."
      },
      {
        id: "sms",
        section: "requested_later_features",
        label: "SMS",
        behavior: "clickable_runtime_request",
        requiredForContinue: false,
        status: permissionChecklist.access_to_sms ?? "unknown",
        actionHint: "Tap to request SMS read permission."
      },
      {
        id: "app_owned_hidden_runtime_execution",
        section: "internal_runtime_capabilities",
        label: "App-owned hidden runtime execution",
        behavior: "clickable_settings_path",
        requiredForContinue: false,
        status: runtimeProof.runtimeHostStarted ? "granted" : "missing",
        actionHint: runtimeProof.runtimeHostStarted
          ? "Already running."
          : "Tap to start the app-owned hidden runtime host."
      },
      {
        id: "foreground_background_service_capability",
        section: "internal_runtime_capabilities",
        label: "Foreground/background service capability",
        behavior: "visible_non_clickable_status",
        requiredForContinue: false,
        status:
          hasNativeXMiloclawRuntimeHost()
            ? combineStatuses(
                permissionChecklist.run_foreground_service ?? "unknown",
                permissionChecklist.run_background_service ?? "unknown"
              )
            : "unknown",
        actionHint: "Internal capability/state; no direct user-grant action on this screen."
      }
    ];

    const requiredNowMissing: string[] = [];
    if (!runtimeProof.notificationsGranted) requiredNowMissing.push("Show notifications");
    if (!runtimeProof.appearOnTopGranted) requiredNowMissing.push("Appear on top");
    if (!runtimeProof.batteryUnrestricted) requiredNowMissing.push("Unrestricted battery access");
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

    async function handleOpenSetting(kind: "notifications" | "overlay" | "battery" | "accessibility") {
      try {
        setRuntimeError("");
        if (kind === "notifications") {
          await openXMiloclawNotificationSettings();
        } else if (kind === "overlay") {
          await openXMiloclawOverlaySettings();
        } else if (kind === "battery") {
          await openXMiloclawBatteryOptimizationSettings();
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
        } else if (row.id === "camera_microphone") {
          await requestAndroidPermission("android.permission.CAMERA");
          await requestAndroidPermission("android.permission.RECORD_AUDIO");
        } else if (row.id === "photos_videos") {
          await PermissionsAndroid.requestMultiple([
            "android.permission.READ_MEDIA_IMAGES",
            "android.permission.READ_MEDIA_VIDEO"
          ] as any);
        } else if (row.id === "location") {
          await requestAndroidPermission("android.permission.ACCESS_FINE_LOCATION");
        } else if (row.id === "phone") {
          await requestAndroidPermission("android.permission.READ_PHONE_STATE");
        } else if (row.id === "sms") {
          await requestAndroidPermission("android.permission.READ_SMS");
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
        if (!status.accessibilityEnabled) missing.push("Accessibility Service enabled");
        if (missing.length > 0) {
          setMissingMessage("Still missing", missing);
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
        const status = await startXMiloclawRuntimeHost();
        setRuntimeStatus(status);
        if (!status.runtimeHostStarted) {
          setRuntimeError(status.lastError || "Runtime host did not start.");
          return;
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

                    const getRowAction = (rowId: string): "notifications" | "overlay" | "battery" | "accessibility" | "request_permission" | null => {
                      if (rowId === "show_notifications") return "notifications" as const;
                      if (rowId === "appear_on_top") return "overlay" as const;
                      if (rowId === "unrestricted_battery_access") return "battery" as const;
                      if (rowId === "accessibility_service_enabled") return "accessibility" as const;
                      if (
                        rowId === "read_audio_files" ||
                        rowId === "read_image_files" ||
                        rowId === "read_video_files" ||
                        rowId === "read_user_selected_images_videos" ||
                        rowId === "read_locations_from_media_collection" ||
                        rowId === "access_to_camera" ||
                        rowId === "access_to_microphone" ||
                        rowId === "send_text_message" ||
                        rowId === "access_to_sms" ||
                        rowId === "make_phone_call" ||
                        rowId === "access_to_phone" ||
                        rowId === "access_to_call_logs"
                      ) {
                        return "request_permission";
                      }
                      return null;
                    };

                    // Only allow a row click if it is explicitly missing (not unknown, not granted) and we have a real action.
                    const actionKind = actionableMissing ? getRowAction(key) : null;
                    const rowPressable = Boolean(actionKind);

                    const onRowPress = async () => {
                      if (!actionKind || !actionableMissing || runtimeBusy) return;
                      setRuntimeError("");
                      try {
                        if (actionKind === "notifications") {
                          await requestAndroidPermission("android.permission.POST_NOTIFICATIONS");
                        } else if (actionKind === "request_permission") {
                          const byId: Record<string, string> = {
                            read_audio_files: "android.permission.READ_MEDIA_AUDIO",
                            read_image_files: "android.permission.READ_MEDIA_IMAGES",
                            read_video_files: "android.permission.READ_MEDIA_VIDEO",
                            read_user_selected_images_videos: "android.permission.READ_MEDIA_VISUAL_USER_SELECTED",
                            read_locations_from_media_collection: "android.permission.ACCESS_MEDIA_LOCATION",
                            access_to_camera: "android.permission.CAMERA",
                            access_to_microphone: "android.permission.RECORD_AUDIO",
                            send_text_message: "android.permission.SEND_SMS",
                            access_to_sms: "android.permission.READ_SMS",
                            make_phone_call: "android.permission.CALL_PHONE",
                            access_to_phone: "android.permission.READ_PHONE_STATE",
                            access_to_call_logs: "android.permission.READ_CALL_LOG"
                          };
                          const perm = byId[key];
                          if (!perm) return;
                          await requestAndroidPermission(perm);
                        } else if (actionKind === "overlay") {
                          await handleOpenSetting("overlay");
                        } else if (actionKind === "battery") {
                          await handleOpenSetting("battery");
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
                          {row.rowClass === "required_but_not_yet_truthfully_mappable" || row.rowClass === "replace_with_exact_android_surface" ? (
                            <Text style={styles.checkSubLabelWarn}>
                              BLOCKING: {row.blockingReason || "No truthful mapping in this build."}
                            </Text>
                          ) : null}
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
                    const prefix = row.rowClass === "required_but_not_yet_truthfully_mappable" || row.rowClass === "replace_with_exact_android_surface"
                      ? "BLOCKING"
                      : status.toUpperCase();
                    if (row.rowClass === "replace_with_exact_android_surface") {
                      missingLines.push(
                        `${row.label} — ${prefix}. Needs canon-level exact Android surface label. Suggested: ${row.recommendedReplacementLabel || "TBD"}`
                      );
                      continue;
                    }
                    if (row.rowClass === "required_but_not_yet_truthfully_mappable") {
                      missingLines.push(`${row.label} — ${prefix}. ${row.blockingReason || "No truthful mapping/check/action in this build."}`);
                      continue;
                    }
                    if (status === "unknown") {
                      missingLines.push(`${row.label} — UNKNOWN (cannot confirm from this build).`);
                      continue;
                    }
                    // status === missing and checkable
                    if (row.id === "show_notifications" || row.id === "access_to_notifications") {
                      missingLines.push(`${row.label} — tap the row to request, or allow in Notifications settings if prompted.`);
                    } else if (row.id === "appear_on_top") {
                      missingLines.push(`${row.label} — tap the row to open Appear-on-top settings.`);
                    } else if (row.id === "unrestricted_battery_access" || row.id === "ask_ignore_battery_optimizations") {
                      missingLines.push(`${row.label} — tap the row to open App info → Battery → Unrestricted.`);
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
                          style={[styles.linkButton, runtimeBusy && styles.linkButtonDisabled]}
                          disabled={runtimeBusy}
                          onPress={async () => {
                            setRuntimeBusy(true);
                            try {
                              const status = await refreshRuntimeStatus();
                              await computeChecklist(status);
                            } finally {
                              setRuntimeBusy(false);
                            }
                          }}
                        >
                          <Text style={styles.linkButtonText}>{runtimeBusy ? "Re-checking..." : "Recheck"}</Text>
                        </Pressable>
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
                    .map((row) => row.label);
                  const continueEnabled = reviewAtBottom && reviewAcknowledged && requiredRowsMissing.length === 0 && !runtimeBusy;

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
                                style: ({ pressed }: any) => [styles.checkRow, pressed && styles.checkRowPressed]
                              }
                            : { style: styles.checkRow };

                          return (
                            <RowContainer key={row.id} {...rowProps}>
                              <View style={{ flex: 1 }}>
                                <Text style={[styles.checkLabel, granted ? styles.checkLabelGranted : styles.checkLabelMissing]}>
                                  {row.label}
                                </Text>
                                {row.actionHint ? (
                                  <Text
                                    style={
                                      row.behavior === "visible_non_clickable_status"
                                        ? styles.checkSubLabelWarn
                                        : styles.checkSubLabel
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
                    (!reviewAtBottom || runtimeBusy) && styles.linkButtonDisabled,
                    reviewAcknowledged && styles.ackPillSelected
                  ]}
                  disabled={!reviewAtBottom || runtimeBusy}
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
                    onPress={() => {
                      if (requiredRowsMissing.length > 0) {
                        setRuntimeError(`Still missing: ${requiredRowsMissing.join(", ")}`);
                        return;
                      }
                      setRuntimeError("");
                      setRuntimeStep("prepare_hidden_runtime_host");
                    }}
                  >
                    <Text style={styles.linkButtonText}>Continue</Text>
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
                  Runtime host started: {runtimeProof.runtimeHostStarted ? "PASS" : "MISSING"} · Bridge connected:{" "}
                  {runtimeProof.bridgeConnected ? "PASS" : "MISSING"} · Host ready: {runtimeProof.hostReady ? "PASS" : "MISSING"}
                </Text>
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
                    <Text style={[styles.checkLabel, runtimeProof.runtimeHostStarted ? styles.checkLabelGranted : styles.checkLabelMissing]}>
                      Runtime host started
                    </Text>
                    <View
                      style={[
                        styles.checkBadge,
                        runtimeProof.runtimeHostStarted ? styles.checkBadgeGranted : styles.checkBadgeMissing
                      ]}
                    >
                      <Text style={styles.checkBadgeText}>{runtimeProof.runtimeHostStarted ? "✓" : "—"}</Text>
                    </View>
                  </View>

                  <View style={styles.checkRow}>
                    <Text style={[styles.checkLabel, runtimeProof.bridgeConnected ? styles.checkLabelGranted : styles.checkLabelMissing]}>
                      Bridge connected
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
                    <Text style={[styles.checkLabel, runtimeProof.hostReady ? styles.checkLabelGranted : styles.checkLabelMissing]}>
                      Host ready
                    </Text>
                    <View style={[styles.checkBadge, runtimeProof.hostReady ? styles.checkBadgeGranted : styles.checkBadgeMissing]}>
                      <Text style={styles.checkBadgeText}>{runtimeProof.hostReady ? "✓" : "—"}</Text>
                    </View>
                  </View>
                </View>

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
    const options: ByokProvider[] = ["Grok", "OpenAI", "Claude"];

    return (
      <ScrollView style={styles.scroll} contentContainerStyle={styles.padded}>
        <Text style={styles.title}>Choose provider</Text>

        <View style={styles.card}>
          {options.map((provider) => (
            <Pressable
              key={provider}
              style={[styles.linkButton, { marginTop: 10 }]}
              onPress={() => {
                setError("");
                setByokProvider(provider);
                setStep("byok_enter_key");
              }}
            >
              <Text style={styles.linkButtonText}>{provider}</Text>
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
    return (
      <ScrollView style={styles.scroll} contentContainerStyle={styles.padded}>
        <Text style={styles.title}>Enter your API key</Text>
        <Text style={styles.body}>
          Provider: <Text style={styles.inlineAccent}>{byokProvider || "—"}</Text>
        </Text>

        <TextInput
          style={styles.input}
          placeholder="API key"
          placeholderTextColor={CLR.muted}
          value={byokKey}
          onChangeText={setByokKey}
          autoCapitalize="none"
          autoCorrect={false}
          secureTextEntry
        />

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
          onPress={() => router.replace("/")}
        >
          <Text style={styles.textBtnLabel}>Skip for now →</Text>
        </Pressable>
      </ScrollView>
    );
  }

  return null;
}

// ─── Sub-components ───────────────────────────────────────────────────────────

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

// ─── Styles ───────────────────────────────────────────────────────────────────

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
