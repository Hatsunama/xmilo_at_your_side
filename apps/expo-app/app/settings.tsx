import { useEffect, useState } from "react";
import * as Clipboard from "expo-clipboard";
import { Alert, Linking, Modal, Pressable, ScrollView, StyleSheet, Switch, Text, TextInput, View } from "react-native";
import * as Notifications from "expo-notifications";
import QRCode from "react-native-qrcode-svg";
import { useApp } from "../src/state/AppContext";
import {
  authCheck,
  authDeleteAccount,
  authLogout,
  authRedeemInvite,
  beginTwoFactorSetup,
  confirmTwoFactorSetup,
  createWebsiteHandoff,
  disableTwoFactor,
  getState,
  getStorageStats,
  getTwoFactorStatus,
  regenerateTwoFactorRecoveryCodes,
  resetTier,
  verifyTwoFactor
} from "../src/lib/bridge";
import { DELETE_ACCOUNT_URL, PRIVACY_POLICY_URL, SUPPORT_URL } from "../src/lib/config";
import { configureRevenueCat, isRevenueCatConfigured, restoreRevenueCatPurchases } from "../src/lib/revenuecat";

export default function SettingsScreen() {
  const {
    state,
    storageStats,
    setStorageStats,
    speakAloudEnabled,
    setSpeakAloudEnabled,
    wakeWordEnabled,
    setWakeWordEnabled
  } = useApp();
  const [accessCode, setAccessCode] = useState("");
  const [notificationEnabled, setNotificationEnabled] = useState(false);
  const [twoFactorCode, setTwoFactorCode] = useState("");
  const [recoveryCode, setRecoveryCode] = useState("");
  const [twoFactorSecret, setTwoFactorSecret] = useState("");
  const [twoFactorOtpUrl, setTwoFactorOtpUrl] = useState("");
  const [websiteHandoffUrl, setWebsiteHandoffUrl] = useState("");
  const [websiteQrVisible, setWebsiteQrVisible] = useState(false);
  const [twoFactorStatus, setTwoFactorStatus] = useState({
    verified_email: "",
    email_verified: false,
    two_factor_enabled: false,
    two_factor_ok: false,
    website_handoff_ready: false
  });
  const [accessConfig, setAccessConfig] = useState({
    access_code_only: true,
    subscription_allowed: false,
    access_code_grant_days: 30
  });
  const revenueCatPurchasePathVisible = Boolean(accessConfig.subscription_allowed && isRevenueCatConfigured());

  useEffect(() => {
    authCheck()
      .then((result) => {
        setAccessConfig({
          access_code_only: result.access_code_only ?? true,
          subscription_allowed: result.subscription_allowed ?? false,
          access_code_grant_days: result.access_code_grant_days ?? 30
        });
      })
      .catch(() => null);

    Notifications.getPermissionsAsync()
      .then((permissions) => {
        setNotificationEnabled(permissions.granted);
      })
      .catch(() => null);

    getTwoFactorStatus()
      .then((status) => {
        setTwoFactorStatus({
          verified_email: status?.verified_email ?? "",
          email_verified: status?.email_verified ?? false,
          two_factor_enabled: status?.two_factor_enabled ?? false,
          two_factor_ok: status?.two_factor_ok ?? false,
          website_handoff_ready: status?.website_handoff_ready ?? false
        });
      })
      .catch(() => null);
  }, []);

  async function refreshStats() {
    try {
      const data = await getStorageStats();
      setStorageStats(data);
    } catch (error: any) {
      Alert.alert("Storage stats failed", error?.message ?? "Unknown error");
    }
  }

  async function wipeChatCache() {
    try {
      await resetTier("chat_cache_only");
      await refreshStats();
    } catch (error: any) {
      Alert.alert("Reset failed", error?.message ?? "Unknown error");
    }
  }

  async function waitForRelayEntitlement() {
    for (let i = 0; i < 6; i += 1) {
      const result = await authCheck().catch(() => null);
      if (result?.entitled) {
        return true;
      }
      await new Promise((resolve) => setTimeout(resolve, 2000));
    }
    return false;
  }

  async function ensureRuntimeId() {
    if (state.runtime_id && state.runtime_id !== "app-shell") {
      return state.runtime_id;
    }
    const latest = await getState();
    return latest.runtime_id ?? "";
  }

  async function restorePurchases() {
    try {
      if (!isRevenueCatConfigured()) {
        Alert.alert("RevenueCat not configured", "Add EXPO_PUBLIC_RC_ANDROID_API_KEY first.");
        return;
      }

      const runtimeId = await ensureRuntimeId();
      if (!runtimeId) {
        Alert.alert("Restore failed", "xMilo runtime ID is missing.");
        return;
      }

      configureRevenueCat(runtimeId);
      const result = await restoreRevenueCatPurchases();

      if (!result.entitled) {
        Alert.alert("No purchase found", "No active xMilo Pro entitlement was restored.");
        return;
      }

      const relayReady = await waitForRelayEntitlement();
      if (relayReady) {
        Alert.alert("Restored", "Your xMilo Pro access is active.");
        return;
      }

      Alert.alert("Restored", "RevenueCat restored the purchase. Relay access is still finalizing.");
    } catch (error: any) {
      Alert.alert("Restore failed", error?.message ?? "Unknown error");
    }
  }

  async function redeemAccessCode() {
    try {
      const runtimeId = await ensureRuntimeId();
      const code = accessCode.trim().toUpperCase();
      if (!runtimeId) {
        Alert.alert("Redeem failed", "xMilo runtime ID is missing.");
        return;
      }
      if (!code) {
        Alert.alert("Redeem failed", "Enter your access code first.");
        return;
      }

      const result = await authRedeemInvite(code, runtimeId);
      setAccessConfig((current) => ({
        ...current,
        access_code_grant_days: result?.access_code_grant_days ?? current.access_code_grant_days
      }));
      setAccessCode("");
      Alert.alert(
        "Access code redeemed",
        `xMilo unlocked ${String(result?.access_code_days ?? accessConfig.access_code_grant_days)} days for this launch stage.`
      );
    } catch (error: any) {
      Alert.alert("Redeem failed", error?.message ?? "Unknown error");
    }
  }

  async function requestNotificationAccess() {
    try {
      const current = await Notifications.getPermissionsAsync();
      if (current.granted) {
        setNotificationEnabled(true);
        Alert.alert("Already allowed", "Local magical notices are already allowed on this device.");
        return;
      }

      const next = await Notifications.requestPermissionsAsync();
      if (!next.granted) {
        setNotificationEnabled(false);
        Alert.alert("Permission still off", "xMilo cannot send local notices until you allow notifications.");
        return;
      }

      setNotificationEnabled(true);
      Alert.alert("Notifications enabled", "xMilo can now surface local notices for safe reminders and status updates.");
    } catch (error: any) {
      Alert.alert("Notification setup failed", error?.message ?? "Unknown error");
    }
  }

  async function sendTestNotification() {
    try {
      const permissions = await Notifications.getPermissionsAsync();
      if (!permissions.granted) {
        Alert.alert("Notifications are off", "Turn on local notices first so xMilo can send a test ping.");
        return;
      }

      await Notifications.scheduleNotificationAsync({
        content: {
          title: "xMilo test notice",
          body: "This is a local test notification from the current app shell.",
          sound: "default"
        },
        trigger: null
      });
      setNotificationEnabled(true);
      Alert.alert("Test notice sent", "If Android still blocks it, reopen app settings and restore notification permission there.");
    } catch (error: any) {
      Alert.alert("Test failed", error?.message ?? "Unknown error");
    }
  }

  async function openAppSettings() {
    try {
      await Linking.openSettings();
    } catch (error: any) {
      Alert.alert("Open settings failed", error?.message ?? "Unknown error");
    }
  }

  async function openExternalUrl(url: string, label: string) {
    try {
      await Linking.openURL(url);
    } catch (error: any) {
      Alert.alert(`${label} failed`, error?.message ?? "Unknown error");
    }
  }

  function resetIdentityStateForLocalSignOut() {
    setTwoFactorStatus({
      verified_email: "",
      email_verified: false,
      two_factor_enabled: false,
      two_factor_ok: false,
      website_handoff_ready: false
    });
    setAccessCode("");
  }

  async function signOutFlow() {
    try {
      await authLogout();
      resetIdentityStateForLocalSignOut();
      Alert.alert("Signed out", "Local relay session cleared from this device. You can sign in again whenever you are ready.");
    } catch (error: any) {
      Alert.alert("Sign out failed", error?.message ?? "Unknown error");
    }
  }

  async function deleteAccountFlow() {
    try {
      await authDeleteAccount();
      resetIdentityStateForLocalSignOut();
      Alert.alert(
        "Account deleted",
        "xMilo removed the current verified account data it could delete immediately and cleared this phone's local session."
      );
    } catch (error: any) {
      Alert.alert("Delete account failed", error?.message ?? "Unknown error");
    }
  }

  async function startTwoFactorSetupFlow() {
    try {
      const result = await beginTwoFactorSetup();
      setTwoFactorSecret(result?.secret ?? "");
      setTwoFactorOtpUrl(result?.otp_url ?? "");
      Alert.alert("2FA setup started", "Add the secret to your authenticator app, then come back and confirm with the current code.");
    } catch (error: any) {
      Alert.alert("2FA setup failed", error?.message ?? "Unknown error");
    }
  }

  async function copyTwoFactorSecret() {
    try {
      if (!twoFactorSecret) {
        Alert.alert("No secret yet", "Start 2FA setup first.");
        return;
      }
      await Clipboard.setStringAsync(twoFactorSecret);
      Alert.alert("Copied", "The authenticator secret is now on your clipboard.");
    } catch (error: any) {
      Alert.alert("Copy failed", error?.message ?? "Unknown error");
    }
  }

  async function confirmTwoFactor() {
    try {
      if (!twoFactorCode.trim()) {
        Alert.alert("Code required", "Enter the current authenticator code first.");
        return;
      }
      const result = await confirmTwoFactorSetup(twoFactorCode.trim());
      setTwoFactorCode("");
      setTwoFactorStatus((current) => ({
        ...current,
        two_factor_enabled: true,
        two_factor_ok: true,
        website_handoff_ready: true
      }));
      const recoveryCodes = Array.isArray(result?.recovery_codes) ? result.recovery_codes.join("\n") : "No recovery codes returned.";
      Alert.alert("2FA enabled", `Save these recovery codes somewhere safe:\n\n${recoveryCodes}`);
    } catch (error: any) {
      Alert.alert("Confirm failed", error?.message ?? "Unknown error");
    }
  }

  async function verifyThisPhone() {
    try {
      if (!twoFactorCode.trim() && !recoveryCode.trim()) {
        Alert.alert("Verification required", "Enter an authenticator code or a recovery code.");
        return;
      }
      await verifyTwoFactor(twoFactorCode.trim(), recoveryCode.trim());
      setTwoFactorCode("");
      setRecoveryCode("");
      setTwoFactorStatus((current) => ({
        ...current,
        two_factor_ok: true,
        website_handoff_ready: true
      }));
      Alert.alert("Phone verified", "This phone is now trusted for 2FA-sensitive xMilo actions.");
    } catch (error: any) {
      Alert.alert("Verification failed", error?.message ?? "Unknown error");
    }
  }

  async function regenerateRecoveryCodes() {
    try {
      const result = await regenerateTwoFactorRecoveryCodes();
      const recoveryCodes = Array.isArray(result?.recovery_codes) ? result.recovery_codes.join("\n") : "No recovery codes returned.";
      Alert.alert("Recovery codes refreshed", recoveryCodes);
    } catch (error: any) {
      Alert.alert("Regenerate failed", error?.message ?? "Unknown error");
    }
  }

  async function disableTwoFactorFlow() {
    try {
      if (!twoFactorCode.trim() && !recoveryCode.trim()) {
        Alert.alert("Verification required", "Enter an authenticator code or recovery code to disable 2FA.");
        return;
      }
      await disableTwoFactor(twoFactorCode.trim(), recoveryCode.trim());
      setTwoFactorCode("");
      setRecoveryCode("");
      setTwoFactorSecret("");
      setTwoFactorOtpUrl("");
      setTwoFactorStatus((current) => ({
        ...current,
        two_factor_enabled: false,
        two_factor_ok: false,
        website_handoff_ready: current.email_verified
      }));
      Alert.alert("2FA disabled", "Authenticator lock removed from this email.");
    } catch (error: any) {
      Alert.alert("Disable failed", error?.message ?? "Unknown error");
    }
  }

  async function openWebsiteWithHandoff() {
    try {
      const result = await createWebsiteHandoff();
      const handoffUrl = result?.handoff_url;
      if (!handoffUrl) {
        Alert.alert("Website handoff failed", "No handoff URL was returned.");
        return;
      }
      await Clipboard.setStringAsync(handoffUrl);
      await Linking.openURL(handoffUrl);
    } catch (error: any) {
      Alert.alert("Website handoff failed", error?.message ?? "Unknown error");
    }
  }

  async function showWebsiteHandoffQr() {
    try {
      const result = await createWebsiteHandoff();
      const handoffUrl = result?.handoff_url;
      if (!handoffUrl) {
        Alert.alert("QR setup failed", "No handoff URL was returned.");
        return;
      }
      setWebsiteHandoffUrl(handoffUrl);
      setWebsiteQrVisible(true);
    } catch (error: any) {
      Alert.alert("QR setup failed", error?.message ?? "Unknown error");
    }
  }

  async function copyWebsiteHandoffUrl() {
    try {
      if (!websiteHandoffUrl) {
        Alert.alert("Nothing to copy", "Generate a website handoff first.");
        return;
      }
      await Clipboard.setStringAsync(websiteHandoffUrl);
      Alert.alert("Copied", "The website handoff link is now on your clipboard.");
    } catch (error: any) {
      Alert.alert("Copy failed", error?.message ?? "Unknown error");
    }
  }

  return (
    <>
      <ScrollView style={styles.screen} contentContainerStyle={styles.content}>
        <View style={styles.card}>
        <Text style={styles.title}>Settings</Text>
        <Text style={styles.body}>
          This starter screen still focuses on bridge-driven actions first. Log out, access-code management, and deeper reset flows remain tracked separately.
        </Text>

        <View style={styles.launchCard}>
          <Text style={styles.launchTitle}>Launch access</Text>
          <Text style={styles.launchBody}>
            {accessConfig.access_code_only
              ? `xMilo is currently in access-code-only launch mode. Codes grant ${String(accessConfig.access_code_grant_days)} days during this stage.`
              : "Public access is enabled. Access codes still stack normally on top of standard access."}
          </Text>
          <TextInput
            style={styles.codeInput}
            placeholder="ENTER ACCESS CODE"
            placeholderTextColor="#6B7280"
            value={accessCode}
            onChangeText={setAccessCode}
            autoCapitalize="characters"
            autoCorrect={false}
          />
          <Pressable style={styles.secondaryButton} onPress={redeemAccessCode}>
            <Text style={styles.buttonText}>Redeem access code</Text>
          </Pressable>
        </View>

        <Pressable style={styles.button} onPress={refreshStats}>
          <Text style={styles.buttonText}>Refresh storage stats</Text>
        </Pressable>
        {revenueCatPurchasePathVisible ? (
          <Pressable style={styles.secondaryButton} onPress={restorePurchases}>
            <Text style={styles.buttonText}>Restore purchases</Text>
          </Pressable>
        ) : null}
        <Pressable style={styles.secondaryButton} onPress={wipeChatCache}>
          <Text style={styles.buttonText}>Reset chat cache only</Text>
        </Pressable>

        <View style={styles.toggleCard}>
          <View style={styles.toggleRow}>
            <View style={styles.toggleLabels}>
              <Text style={styles.toggleTitle}>Speak aloud</Text>
              <Text style={styles.toggleSubtitle}>
                Lets Milo use app-side voice cues when wake-word or nightly ritual events call for them.
              </Text>
            </View>
            <Switch
              value={speakAloudEnabled}
              onValueChange={setSpeakAloudEnabled}
              trackColor={{ false: "#374151", true: "#5B3FA6" }}
              thumbColor={speakAloudEnabled ? "#C8B8FF" : "#6B7280"}
            />
          </View>
          <Text style={styles.toggleHint}>
            {speakAloudEnabled
              ? "App-side spoken cues are allowed when Milo has something real to announce."
              : "Milo will stay quiet in the app shell, while still keeping visual and vibration cues when available."}
          </Text>
        </View>

        <View style={styles.toggleCard}>
          <View style={styles.toggleRow}>
            <View style={styles.toggleLabels}>
              <Text style={styles.toggleTitle}>Voice Wake Word</Text>
              <Text style={styles.toggleSubtitle}>
                Say <Text style={styles.wakePhrase}>"xMilo"</Text> to wake me up
              </Text>
            </View>
            <Switch
              value={wakeWordEnabled}
              onValueChange={setWakeWordEnabled}
              trackColor={{ false: "#374151", true: "#5B3FA6" }}
              thumbColor={wakeWordEnabled ? "#C8B8FF" : "#6B7280"}
            />
          </View>
          {wakeWordEnabled ? (
            <Text style={styles.toggleHint}>🎙 Listening for "xMilo"…</Text>
          ) : null}
        </View>

        <View style={styles.toggleCard}>
          <View style={styles.toggleLabels}>
            <Text style={styles.toggleTitle}>Local notifications</Text>
            <Text style={styles.toggleSubtitle}>
              This is only the app-level notice layer. Full phone-wide listener magic still belongs to the later native service phase.
            </Text>
          </View>
          <Pressable style={styles.secondaryButton} onPress={requestNotificationAccess}>
            <Text style={styles.buttonText}>Turn on local notices</Text>
          </Pressable>
          <Pressable style={styles.secondaryButton} onPress={sendTestNotification}>
            <Text style={styles.buttonText}>Send test notice</Text>
          </Pressable>
          <Pressable style={styles.secondaryButton} onPress={openAppSettings}>
            <Text style={styles.buttonText}>Open app settings</Text>
          </Pressable>
          <Text style={styles.toggleHint}>
            {notificationEnabled
              ? "Local notices are currently allowed."
              : "Local notices are not confirmed yet. If Android later turns them off, reopen app settings and turn them back on for the magic to work."}
          </Text>
        </View>

        <View style={styles.toggleCard}>
          <View style={styles.toggleLabels}>
            <Text style={styles.toggleTitle}>2 AM upkeep ritual</Text>
            <Text style={styles.toggleSubtitle}>
              At 2:00 AM local time, Milo checks for fresh app releases and seals the day into the archive. If he is still busy with a task, he waits and finishes the ritual right after.
            </Text>
          </View>
          <Text style={styles.toggleHint}>
            Termux vocal and vibration cues are used for this ritual when the local runtime is awake, so the phone can nudge you even with the app in the background.
          </Text>
          <Text style={styles.toggleHint}>
            The app shell also mirrors this with local vibration, notices, and spoken cues when allowed, so the ritual still feels alive if the castle is open.
          </Text>
        </View>

        <View style={styles.toggleCard}>
          <View style={styles.toggleLabels}>
            <Text style={styles.toggleTitle}>Two-factor lock</Text>
            <Text style={styles.toggleSubtitle}>
              Verified email: {twoFactorStatus.verified_email || "not verified yet"}
            </Text>
          </View>
          <Text style={styles.toggleHint}>
            {twoFactorStatus.two_factor_enabled
              ? twoFactorStatus.two_factor_ok
                ? "2FA is enabled and this phone is trusted."
                : "2FA is enabled, but this phone still needs to be verified."
              : "2FA is not enabled yet."}
          </Text>
          {!twoFactorStatus.two_factor_enabled ? (
            <>
              <Pressable style={styles.secondaryButton} onPress={startTwoFactorSetupFlow}>
                <Text style={styles.buttonText}>Start 2FA setup</Text>
              </Pressable>
              {twoFactorSecret ? (
                <>
                  <Text style={[styles.toggleHint, { marginTop: 12 }]}>Authenticator secret</Text>
                  <Text style={styles.codeInput}>{twoFactorSecret}</Text>
                  {twoFactorOtpUrl ? <Text style={styles.toggleHint}>otpauth URL ready for manual import.</Text> : null}
                  <Pressable style={styles.secondaryButton} onPress={copyTwoFactorSecret}>
                    <Text style={styles.buttonText}>Copy secret</Text>
                  </Pressable>
                  <TextInput
                    style={styles.codeInput}
                    placeholder="Confirm authenticator code"
                    placeholderTextColor="#6B7280"
                    value={twoFactorCode}
                    onChangeText={setTwoFactorCode}
                    keyboardType="number-pad"
                  />
                  <Pressable style={styles.secondaryButton} onPress={confirmTwoFactor}>
                    <Text style={styles.buttonText}>Enable 2FA</Text>
                  </Pressable>
                </>
              ) : null}
            </>
          ) : (
            <>
              <TextInput
                style={styles.codeInput}
                placeholder="Authenticator code"
                placeholderTextColor="#6B7280"
                value={twoFactorCode}
                onChangeText={setTwoFactorCode}
                keyboardType="number-pad"
              />
              <TextInput
                style={styles.codeInput}
                placeholder="Recovery code"
                placeholderTextColor="#6B7280"
                value={recoveryCode}
                onChangeText={setRecoveryCode}
                autoCapitalize="characters"
                autoCorrect={false}
              />
              {!twoFactorStatus.two_factor_ok ? (
                <Pressable style={styles.secondaryButton} onPress={verifyThisPhone}>
                  <Text style={styles.buttonText}>Verify this phone</Text>
                </Pressable>
              ) : null}
              <Pressable style={styles.secondaryButton} onPress={regenerateRecoveryCodes}>
                <Text style={styles.buttonText}>Regenerate recovery codes</Text>
              </Pressable>
              <Pressable style={styles.secondaryButton} onPress={disableTwoFactorFlow}>
                <Text style={styles.buttonText}>Disable 2FA</Text>
              </Pressable>
            </>
          )}
        </View>

        <View style={styles.toggleCard}>
          <View style={styles.toggleLabels}>
            <Text style={styles.toggleTitle}>Website handoff</Text>
            <Text style={styles.toggleSubtitle}>
              The website stays separate and only trusts a short-lived signed handoff from app/relay.
            </Text>
          </View>
          <Pressable style={styles.secondaryButton} onPress={openWebsiteWithHandoff}>
            <Text style={styles.buttonText}>Open website with handoff</Text>
          </Pressable>
          <Pressable style={styles.secondaryButton} onPress={showWebsiteHandoffQr}>
            <Text style={styles.buttonText}>Show website sign-in QR</Text>
          </Pressable>
          <Text style={styles.toggleHint}>
            {twoFactorStatus.website_handoff_ready
              ? "Ready. This phone can mint a short-lived website session handoff."
              : "Not ready yet. Finish verified email and any required 2FA first."}
          </Text>
        </View>

        <View style={styles.toggleCard}>
          <View style={styles.toggleLabels}>
            <Text style={styles.toggleTitle}>Privacy and help</Text>
            <Text style={styles.toggleSubtitle}>
              Review xMilo policy pages, support path, and the public account-deletion request page.
            </Text>
          </View>
          <Pressable style={styles.secondaryButton} onPress={() => openExternalUrl(PRIVACY_POLICY_URL, "Privacy policy")}>
            <Text style={styles.buttonText}>Open privacy policy</Text>
          </Pressable>
          <Pressable style={styles.secondaryButton} onPress={() => openExternalUrl(SUPPORT_URL, "Support page")}>
            <Text style={styles.buttonText}>Open support page</Text>
          </Pressable>
          <Pressable style={styles.secondaryButton} onPress={() => openExternalUrl(DELETE_ACCOUNT_URL, "Delete-account page")}>
            <Text style={styles.buttonText}>Open delete-account page</Text>
          </Pressable>
          <Pressable style={styles.secondaryButton} onPress={signOutFlow}>
            <Text style={styles.buttonText}>Sign out on this phone</Text>
          </Pressable>
          <Pressable style={styles.secondaryButton} onPress={deleteAccountFlow}>
            <Text style={styles.buttonText}>Delete this xMilo account</Text>
          </Pressable>
        </View>

        <View style={styles.statsCard}>
          <Text style={styles.statsTitle}>Storage</Text>
          <Text style={styles.statsRow}>SQLite bytes: {String(storageStats?.pico_sqlite_bytes ?? "unknown")}</Text>
          <Text style={styles.statsRow}>Task rows: {String(storageStats?.runtime_task_rows ?? "unknown")}</Text>
          <Text style={styles.statsRow}>Pending events: {String(storageStats?.pending_event_rows ?? "unknown")}</Text>
          <Text style={styles.statsRow}>Conversation rows: {String(storageStats?.conversation_tail_rows ?? "unknown")}</Text>
        </View>
        </View>
      </ScrollView>

      <Modal
        animationType="fade"
        transparent
        visible={websiteQrVisible}
        onRequestClose={() => setWebsiteQrVisible(false)}
      >
        <View style={styles.modalScrim}>
          <View style={styles.modalCard}>
            <Text style={styles.modalTitle}>Website sign-in QR</Text>
            <Text style={styles.modalBody}>
              This QR carries the same short-lived signed handoff used by the website link. Scan it from the website login page or with a desktop session flow.
            </Text>
            {websiteHandoffUrl ? (
              <View style={styles.qrPanel}>
                <QRCode value={websiteHandoffUrl} size={210} />
              </View>
            ) : null}
            <Pressable style={styles.secondaryButton} onPress={copyWebsiteHandoffUrl}>
              <Text style={styles.buttonText}>Copy handoff link</Text>
            </Pressable>
            <Pressable style={styles.secondaryButton} onPress={() => setWebsiteQrVisible(false)}>
              <Text style={styles.buttonText}>Close</Text>
            </Pressable>
          </View>
        </View>
      </Modal>
    </>
  );
}

const styles = StyleSheet.create({
  screen: { flex: 1, backgroundColor: "#0B1020" },
  content: { padding: 16 },
  card: { backgroundColor: "#111827", borderRadius: 18, padding: 16, borderWidth: 1, borderColor: "#1F2937" },
  title: { color: "#F8FAFC", fontSize: 22, fontWeight: "700" },
  body: { color: "#CBD5E1", lineHeight: 22, marginTop: 8 },
  button: { marginTop: 16, backgroundColor: "#2563EB", borderRadius: 14, paddingVertical: 14, alignItems: "center" },
  secondaryButton: { marginTop: 12, backgroundColor: "#1F2937", borderRadius: 14, paddingVertical: 14, alignItems: "center" },
  buttonText: { color: "#FFFFFF", fontWeight: "700" },
  launchCard: { marginTop: 16, backgroundColor: "#0F172A", borderRadius: 14, padding: 14, borderWidth: 1, borderColor: "#312E81" },
  launchTitle: { color: "#C4B5FD", fontWeight: "700", fontSize: 15 },
  launchBody: { color: "#CBD5E1", lineHeight: 21, marginTop: 6 },
  codeInput: {
    marginTop: 12,
    backgroundColor: "#111827",
    borderWidth: 1,
    borderColor: "#374151",
    borderRadius: 12,
    paddingHorizontal: 14,
    paddingVertical: 12,
    color: "#F8FAFC",
    letterSpacing: 2
  },
  statsCard: { marginTop: 18, backgroundColor: "#0F172A", borderRadius: 14, padding: 14, gap: 6 },
  statsTitle: { color: "#F8FAFC", fontWeight: "700" },
  statsRow: { color: "#CBD5E1" },
  toggleCard: { marginTop: 18, backgroundColor: "#0F172A", borderRadius: 14, padding: 14 },
  toggleRow: { flexDirection: "row", alignItems: "center", justifyContent: "space-between" },
  toggleLabels: { flex: 1, marginRight: 12 },
  toggleTitle: { color: "#F8FAFC", fontWeight: "700", fontSize: 15 },
  toggleSubtitle: { color: "#94A3B8", marginTop: 2, fontSize: 13 },
  wakePhrase: { color: "#C8B8FF", fontWeight: "700" },
  toggleHint: { color: "#7C6FA0", fontSize: 12, marginTop: 10 },
  modalScrim: {
    flex: 1,
    backgroundColor: "rgba(11,16,32,0.82)",
    alignItems: "center",
    justifyContent: "center",
    padding: 20
  },
  modalCard: {
    width: "100%",
    maxWidth: 360,
    backgroundColor: "#111827",
    borderRadius: 20,
    borderWidth: 1,
    borderColor: "#312E81",
    padding: 20
  },
  modalTitle: { color: "#F8FAFC", fontSize: 20, fontWeight: "700" },
  modalBody: { color: "#CBD5E1", lineHeight: 21, marginTop: 10 },
  qrPanel: {
    marginTop: 18,
    marginBottom: 6,
    backgroundColor: "#FFFFFF",
    borderRadius: 18,
    padding: 18,
    alignItems: "center",
    justifyContent: "center"
  }
});
