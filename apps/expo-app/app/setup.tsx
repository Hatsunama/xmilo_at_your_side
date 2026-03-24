import { useRouter } from "expo-router";
import { useEffect, useRef, useState } from "react";
import * as Clipboard from "expo-clipboard";
import {
  ActivityIndicator,
  Alert,
  Linking,
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
  verifyTwoFactor,
  getHealth,
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
  GITHUB_REPO_URL,
  LOCALHOST_TOKEN,
  RELAY_BASE_URL,
  SIDECAR_RELEASES_URL,
  TERMUX_BOOTSTRAP_COMMAND,
  resolveSidecarReleaseAsset
} from "../src/lib/config";
import {
  buildTermuxBootstrapCommand,
  getTermuxBootstrapStatus,
  hasNativeTermuxBootstrap,
  openTermuxApp,
  openTermuxBootstrapPermissionSettings,
  runTermuxBootstrap
} from "../src/lib/termuxBootstrap";

// ─── Step machine ─────────────────────────────────────────────────────────────
// checking           → silent boot check (already entitled? skip wizard)
// waiting_sidecar    → sidecar not reachable yet (show Termux install steps)
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
  | "invite_input";

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
      "xMilo asks you to install Termux and Termux:API so part of his magic can live locally on your phone."
  },
  {
    title: "We warn you first",
    body:
      "Android and Termux can show strong warning screens in red. That does not mean something is hidden. It means the system is asking for clear consent before local tools can run."
  },
  {
    title: "You stay in control",
    body:
      "Nothing should install or run silently. We show direct links, tell you why each step matters, and if you decline, xMilo simply cannot finish local setup."
  }
] as const;

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
  const [step, setStep] = useState<Step>("checking");
  const [email, setEmail] = useState("");
  const [inviteCode, setInviteCode] = useState("");
  const [deviceUserID, setDeviceUserID] = useState("");
  const [error, setError] = useState("");
  const [busy, setBusy] = useState(false);
  const [verifyAttempts, setVerifyAttempts] = useState(0);
  const [twoFactorCode, setTwoFactorCode] = useState("");
  const [recoveryCode, setRecoveryCode] = useState("");
  const [termuxInstalled, setTermuxInstalled] = useState(false);
  const [runCommandPermissionGranted, setRunCommandPermissionGranted] = useState(false);
  const [bootstrapBusy, setBootstrapBusy] = useState(false);
  const [bootstrapMessage, setBootstrapMessage] = useState("");
  const [trustPage, setTrustPage] = useState(0);
  const [accessConfig, setAccessConfig] = useState<AccessConfig>({
    access_mode: "code_only",
    access_code_only: true,
    trial_allowed: false,
    subscription_allowed: false,
    access_code_grant_days: 30
  });
  const sidecarRelease = resolveSidecarReleaseAsset();
  const sidecarAssetUrl = `${SIDECAR_RELEASES_URL}/latest/download/${sidecarRelease.assetName}`;
  const sidecarChecksumUrl = `${SIDECAR_RELEASES_URL}/latest/download/${sidecarRelease.checksumName}`;
  const runtimeBootstrapCommand = buildTermuxBootstrapCommand({
    bearerToken: LOCALHOST_TOKEN,
    relayBaseUrl: RELAY_BASE_URL,
    command: TERMUX_BOOTSTRAP_COMMAND
  });
  const subscriptionEntryVisible = Boolean(accessConfig.subscription_allowed && isRevenueCatConfigured());

  const pollRef = useRef<ReturnType<typeof setInterval> | null>(null);

  // ── Boot: check if already entitled, or wait for sidecar ─────────────────────
  useEffect(() => {
    async function boot() {
      try {
        const check = await authCheck();
        setAccessConfig(check);
        if (check.entitled) {
          router.replace("/");
          return;
        }
        const s = await getState();
        const runtimeId = s.runtime_id ?? "";
        const relayDeviceUserID = check.device_user_id ?? "";
        const resolvedDeviceUserID = relayDeviceUserID || runtimeId;
        setDeviceUserID(resolvedDeviceUserID);
        syncRevenueCat(resolvedDeviceUserID);
        setStep("email");
      } catch {
        setStep("waiting_sidecar");
        startSidecarPoll();
      }
    }
    boot();
    return () => stopPoll();
  }, []); // eslint-disable-line react-hooks/exhaustive-deps

  useEffect(() => {
    if (step !== "waiting_sidecar" || !hasNativeTermuxBootstrap()) return;
    let cancelled = false;

    getTermuxBootstrapStatus()
      .then((status) => {
        if (cancelled) return;
        setTermuxInstalled(status.termuxInstalled);
        setRunCommandPermissionGranted(status.runCommandPermissionGranted);
      })
      .catch(() => null);

    return () => {
      cancelled = true;
    };
  }, [step]);

  function startSidecarPoll() {
    stopPoll();
    pollRef.current = setInterval(async () => {
      try {
        await getHealth();
        stopPoll();
        const check = await authCheck().catch(() => null);
        if (check) {
          setAccessConfig(check);
          if (check.entitled) {
            router.replace("/");
            return;
          }
        }

        const s = await getState();
        const runtimeId = s.runtime_id ?? "";
        const relayDeviceUserID = check?.device_user_id ?? "";
        const resolvedDeviceUserID = relayDeviceUserID || runtimeId;
        setDeviceUserID(resolvedDeviceUserID);
        syncRevenueCat(resolvedDeviceUserID);
        setStep("email");
      } catch {
        // still not up
      }
    }, 2500);
  }

  function stopPoll() {
    if (pollRef.current) {
      clearInterval(pollRef.current);
      pollRef.current = null;
    }
  }

  async function openExternal(url: string) {
    try {
      await Linking.openURL(url);
    } catch (error: any) {
      Alert.alert("Magic path could not open", error?.message ?? "Unknown error");
    }
  }

  async function handleRunBootstrap() {
    if (!LOCALHOST_TOKEN) {
      Alert.alert(
        "Magic handoff is not ready yet",
        "This build is still missing the local trust token, so xMilo cannot safely pass the bootstrap command into Termux yet."
      );
      return;
    }

    try {
      setBootstrapBusy(true);
      setBootstrapMessage("Magic handoff started in Termux. Keep the phone awake while xMilo verifies the local runtime.");
      await runTermuxBootstrap(runtimeBootstrapCommand);
      Alert.alert(
        "Magic handoff started",
        "xMilo opened Termux, passed the relay path and local trust token, and asked it to begin with termux-wake-lock. Keep the phone awake the first time and wait here while xMilo checks the local runtime."
      );
    } catch (error: any) {
      setBootstrapMessage("");
      Alert.alert("Magic handoff failed", error?.message ?? "Unknown error");
    } finally {
      setBootstrapBusy(false);
    }
  }

  async function handleOpenTermuxPermissionSettings() {
    try {
      await openTermuxBootstrapPermissionSettings();
    } catch (error: any) {
      Alert.alert("Permission gate could not open", error?.message ?? "Unknown error");
    }
  }

  async function handleOpenTermuxApp() {
    try {
      await openTermuxApp();
    } catch (error: any) {
      Alert.alert("Termux could not open", error?.message ?? "Unknown error");
    }
  }

  async function copyBootstrapCommand() {
    try {
      await Clipboard.setStringAsync(TERMUX_BOOTSTRAP_COMMAND);
      Alert.alert("Setup spell copied", "The Termux bootstrap command is now on your clipboard.");
    } catch (error: any) {
      Alert.alert("Copy spell failed", error?.message ?? "Unknown error");
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
    setBusy(true);
    try {
      await authRegister(trimmed, deviceUserID);
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

    return (
      <ScrollView style={styles.scroll} contentContainerStyle={styles.padded}>
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
            <Pressable style={[styles.btn, styles.trustPrimary]} onPress={() => setTrustPage((current) => Math.min(TRUST_PAGES.length - 1, current + 1))}>
              <Text style={styles.btnLabel}>{trustComplete ? "I understand" : "Continue"}</Text>
            </Pressable>
          </View>
        </View>

        {trustComplete ? (
          <>
            <Text style={styles.title}>Before we begin</Text>
            <Text style={styles.body}>
              xMilo needs its local runtime installed on your phone. It runs inside
              Termux — a lightweight Linux environment you control entirely.
            </Text>

            <View style={styles.card}>
              <Text style={styles.cardTitle}>Magic requires trust</Text>
              <Text style={styles.body}>
                xMilo will ask you to install <Text style={styles.inlineAccent}>Termux</Text> and <Text style={styles.inlineAccent}>Termux:API</Text>, then may open Termux and Android permission settings so the local runtime can start.
              </Text>
              <Text style={styles.fine}>
                If you say no, xMilo stays safe, but local magic, on-device help, and background rituals will not finish setting up.
              </Text>
            </View>

            <View style={styles.card}>
              <Text style={styles.cardTitle}>Magic path from F-Droid</Text>
              <StepRow n="1" text="Install F-Droid" sub="A trusted open app store for this ritual. No Google account required." />
              <StepRow n="2" text="Install Termux from F-Droid" sub="This is the local workshop where xMilo's phone magic runs." />
              <StepRow n="3" text="Install Termux:API from F-Droid" sub="This gives xMilo the approved bridge for on-device signals and actions." />
              <View style={styles.actionStack}>
                <Pressable style={styles.linkButton} onPress={() => openExternal("https://f-droid.org/")}>
                  <Text style={styles.linkButtonText}>Open F-Droid</Text>
                </Pressable>
                <Pressable style={styles.linkButton} onPress={() => openExternal("https://f-droid.org/packages/com.termux/")}>
                  <Text style={styles.linkButtonText}>Open Termux page</Text>
                </Pressable>
                <Pressable style={styles.linkButton} onPress={() => openExternal("https://f-droid.org/packages/com.termux.api/")}>
                  <Text style={styles.linkButtonText}>Open Termux:API page</Text>
                </Pressable>
              </View>
            </View>

            <View style={styles.card}>
              <Text style={styles.cardTitle}>Magic artifact check</Text>
              <Text style={styles.body}>
                xMilo now treats GitHub Releases for this repo as the setup artifact authority. Your phone looks like{" "}
                <Text style={styles.inlineAccent}>{sidecarRelease.abiLabel}</Text>, so the expected sidecar binary is{" "}
                <Text style={styles.inlineAccent}>{sidecarRelease.assetName}</Text>.
              </Text>
              <Text style={styles.fine}>
                Detection source: {sidecarRelease.detectedFrom === "android_abis" ? "Android ABI report" : "safe fallback"}.
              </Text>
              <Text style={[styles.mono, { marginTop: 10 }]}>{sidecarAssetUrl}</Text>
              <Text style={[styles.mono, { marginTop: 8 }]}>{sidecarChecksumUrl}</Text>
              <Text style={[styles.body, { marginTop: 8 }]}>
                This artifact check keeps the setup honest. xMilo should fetch this exact file, verify the checksum, install it into Termux, and then return here for health verification.
              </Text>
              {hasNativeTermuxBootstrap() ? (
                <View style={styles.card}>
                  <Text style={styles.cardTitle}>Guided handoff into Termux</Text>
                  <Text style={styles.body}>
                    Once F-Droid, Termux, and Termux:API are installed, xMilo can ask Termux to run the bootstrap command directly. That Termux command begins with <Text style={styles.inlineAccent}>termux-wake-lock</Text> so the sidecar has a better chance of staying alive during first install.
                  </Text>
                  <Text style={styles.fine}>
                    Termux installed: {termuxInstalled ? "yes" : "not confirmed"} · Run-command permission: {runCommandPermissionGranted ? "granted" : "not granted yet"}
                  </Text>
                  {bootstrapMessage ? (
                    <Text style={[styles.fine, { color: CLR.success }]}>{bootstrapMessage}</Text>
                  ) : null}
                  {!LOCALHOST_TOKEN ? (
                    <Text style={[styles.fine, { color: CLR.warn }]}>
                      This build is still missing its local trust token, so app-driven bootstrap stays blocked until that setup piece is restored.
                    </Text>
                  ) : null}
                  <View style={styles.actionStack}>
                    {termuxInstalled ? (
                      <Pressable style={styles.linkButton} onPress={handleOpenTermuxApp}>
                        <Text style={styles.linkButtonText}>Open Termux workshop</Text>
                      </Pressable>
                    ) : null}
                    <Pressable style={styles.linkButton} onPress={handleOpenTermuxPermissionSettings}>
                      <Text style={styles.linkButtonText}>Open xMilo permission gate</Text>
                    </Pressable>
                    <Pressable
                      style={[
                        styles.linkButton,
                        (!termuxInstalled || !runCommandPermissionGranted || !LOCALHOST_TOKEN || bootstrapBusy) && styles.linkButtonDisabled
                      ]}
                      disabled={!termuxInstalled || !runCommandPermissionGranted || !LOCALHOST_TOKEN || bootstrapBusy}
                      onPress={handleRunBootstrap}
                    >
                      <Text style={styles.linkButtonText}>{bootstrapBusy ? "Starting magic handoff..." : "Begin local magic in Termux"}</Text>
                    </Pressable>
                  </View>
                </View>
              ) : null}
              <View style={styles.actionStack}>
                <Pressable style={styles.linkButton} onPress={() => openExternal(SIDECAR_RELEASES_URL)}>
                  <Text style={styles.linkButtonText}>Open releases</Text>
                </Pressable>
                <Pressable style={styles.linkButton} onPress={() => openExternal(sidecarAssetUrl)}>
                  <Text style={styles.linkButtonText}>Open binary asset</Text>
                </Pressable>
                <Pressable style={styles.linkButton} onPress={() => openExternal(sidecarChecksumUrl)}>
                  <Text style={styles.linkButtonText}>Open checksum</Text>
                </Pressable>
                <Pressable style={styles.linkButton} onPress={() => openExternal(GITHUB_REPO_URL)}>
                  <Text style={styles.linkButtonText}>Open repo</Text>
                </Pressable>
              </View>
            </View>

            <View style={styles.card}>
              <Text style={styles.cardTitle}>Extra magic later</Text>
              <Text style={styles.body}>
                Core setup only needs Termux and Termux:API. If xMilo later offers more advanced add-ons, each one should open with a direct link and a clear note about what that one piece unlocks.
              </Text>
              <Text style={styles.fine}>
                If the automatic handoff is unavailable, you can still copy the exact Termux setup spell below. xMilo will keep watching and continue once the local runtime becomes healthy.
              </Text>
              <Text style={[styles.mono, { marginTop: 10 }]}>{TERMUX_BOOTSTRAP_COMMAND}</Text>
              <View style={styles.actionStack}>
                <Pressable style={styles.linkButton} onPress={copyBootstrapCommand}>
                  <Text style={styles.linkButtonText}>Copy setup spell</Text>
                </Pressable>
              </View>
            </View>
          </>
        ) : null}

        <View style={styles.waitRow}>
          <ActivityIndicator color={CLR.accent} size="small" />
          <Text style={styles.muted}>  Waiting for xMilo runtime…</Text>
        </View>
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

        <Btn label="Send verification email" onPress={handleSendEmail} busy={busy} />

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
        <Text style={styles.title}>Enter access code</Text>
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
          onPress={() => { setError(""); setStep("access_choice"); }}
        >
          <Text style={styles.textBtnLabel}>← Back</Text>
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
