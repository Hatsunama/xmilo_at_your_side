import { useRouter } from "expo-router";
import { useEffect, useRef, useState } from "react";
import {
  ActivityIndicator,
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

// ─── Step machine ─────────────────────────────────────────────────────────────
// checking           → silent boot check (already entitled? skip wizard)
// waiting_sidecar    → sidecar not reachable yet (show Termux install steps)
// email              → collect email address
// email_sent         → "check your inbox" + "I verified" polling
// access_choice      → trial / invite / subscribe
// invite_input       → enter invite code
type Step =
  | "checking"
  | "waiting_sidecar"
  | "email"
  | "email_sent"
  | "access_choice"
  | "invite_input";

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

  const pollRef = useRef<ReturnType<typeof setInterval> | null>(null);

  // ── Boot: check if already entitled, or wait for sidecar ─────────────────────
  useEffect(() => {
    async function boot() {
      try {
        const check = await authCheck();
        if (check.entitled) {
          router.replace("/");
          return;
        }
        const s = await getState();
        const runtimeId = s.runtime_id ?? "";
        setDeviceUserID(runtimeId);
        syncRevenueCat(runtimeId);
        setStep("email");
      } catch {
        setStep("waiting_sidecar");
        startSidecarPoll();
      }
    }
    boot();
    return () => stopPoll();
  }, []); // eslint-disable-line react-hooks/exhaustive-deps

  function startSidecarPoll() {
    stopPoll();
    pollRef.current = setInterval(async () => {
      try {
        await getHealth();
        stopPoll();
        try {
          const check = await authCheck();
          if (check.entitled) {
            router.replace("/");
            return;
          }
        } catch {
          // not entitled yet, continue to email step
        }
        const s = await getState();
        const runtimeId = s.runtime_id ?? "";
        setDeviceUserID(runtimeId);
        syncRevenueCat(runtimeId);
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
      if (result.entitled) {
        setStep("access_choice");
      } else {
        setError("Not verified yet — click the link in your inbox first.");
      }
    } catch (e: any) {
      setError(e?.message ?? "Could not reach xMilo. Try again.");
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
      setError("Enter your invite code.");
      return;
    }
    setBusy(true);
    try {
      await authRedeemInvite(code, deviceUserID);
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
    return (
      <ScrollView style={styles.scroll} contentContainerStyle={styles.padded}>
        <Text style={styles.title}>Before we begin</Text>
        <Text style={styles.body}>
          xMilo needs its local runtime installed on your phone. It runs inside
          Termux — a lightweight Linux environment you control entirely.
        </Text>

        <View style={styles.card}>
          <Text style={styles.cardTitle}>Install from F-Droid (3 steps)</Text>
          <StepRow n="1" text="Install F-Droid" sub="Free, open app store. No Google account required." />
          <StepRow n="2" text="Install Termux from F-Droid" sub="Don't use the Play Store version — it's outdated." />
          <StepRow n="3" text="Install Termux:API from F-Droid" sub="Same session, same source as Termux." />
        </View>

        <View style={styles.card}>
          <Text style={styles.cardTitle}>Then open Termux and run</Text>
          <Text style={styles.mono}>
            curl -fsSL https://xmiloatyourside.com/install.sh | bash
          </Text>
          <Text style={[styles.body, { marginTop: 8 }]}>
            This downloads the xMilo runtime and starts it. Come back here when
            it's done — this screen will continue automatically.
          </Text>
        </View>

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
        <Text style={styles.body}>Email verified. Choose your access.</Text>

        <ChoiceCard
          title="Free trial — 12 hours"
          sub="Start now. Timer begins on your first real task."
          accent={CLR.success}
          onPress={handleStartTrial}
        />

        <ChoiceCard
          title="Redeem invite code"
          sub="Have a beta invite? Unlock 5 days of full access."
          accent={CLR.accent}
          onPress={() => { setError(""); setStep("invite_input"); }}
        />

        <ChoiceCard
          title="Subscribe  ·  $19.99 / month"
          sub="Unlimited access billed via Google Play."
          accent={CLR.warn}
          onPress={handleOpenSubscription}
          busy={busy}
        />
      </ScrollView>
    );
  }

  if (step === "invite_input") {
    return (
      <ScrollView style={styles.scroll} contentContainerStyle={styles.padded}>
        <Text style={styles.title}>Enter invite code</Text>
        <Text style={styles.body}>
          Invite codes are single-use and grant 5 days of full access.
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
  cardTitle: { color: CLR.title, fontSize: 16, fontWeight: "600" },

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
