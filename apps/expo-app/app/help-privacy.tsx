import { useCallback, useEffect, useRef, useState } from "react";
import { useLocalSearchParams, useRouter } from "expo-router";
import * as Clipboard from "expo-clipboard";
import {
  ActivityIndicator,
  Alert,
  KeyboardAvoidingView,
  Linking,
  Modal,
  Platform,
  Pressable,
  ScrollView,
  StyleSheet,
  Text,
  TextInput,
  View
} from "react-native";

import { useApp } from "../src/state/AppContext";
import { authDeleteAccount, authLogout, SidecarRequestError, submitSettingsReport } from "../src/lib/bridge";
import { DELETE_ACCOUNT_URL, PRIVACY_POLICY_URL, SUPPORT_URL } from "../src/lib/config";
import { buildSettingsReportBundle } from "../src/lib/reportBundle";
import { enqueueSettingsReport } from "../src/lib/reportQueue";
import type { SettingsReportFailureClass } from "../src/lib/reportFailureClasses";
import { ensureSettingsReportTransportReady } from "../src/lib/reportTransport";

type SettingsReportBundle = Awaited<ReturnType<typeof buildSettingsReportBundle>>;

export default function HelpPrivacyScreen() {
  const router = useRouter();
  const params = useLocalSearchParams<{ reportEntry?: string }>();
  const { state, events } = useApp();
  const [reportModalVisible, setReportModalVisible] = useState(false);
  const [reportConfirmVisible, setReportConfirmVisible] = useState(false);
  const [reportComment, setReportComment] = useState("");
  const [reportStatus, setReportStatus] = useState("");
  const [reportBusy, setReportBusy] = useState(false);
  const [reportResult, setReportResult] = useState<
    | { kind: "sent"; reportId: string; copied: boolean }
    | { kind: "queued"; reason: "unreachable" | "not_ready" }
    | null
  >(null);
  const handledReportEntryRef = useRef("");

  const openReportFlow = useCallback(() => {
    setReportStatus("");
    setReportConfirmVisible(false);
    setReportResult(null);
    setReportModalVisible(true);
  }, []);

  useEffect(() => {
    if (params.reportEntry !== "angryShake") {
      handledReportEntryRef.current = "";
      return;
    }
    if (handledReportEntryRef.current === "angryShake") return;
    handledReportEntryRef.current = "angryShake";
    openReportFlow();
  }, [openReportFlow, params.reportEntry]);

  async function openExternalUrl(url: string, label: string) {
    try {
      await Linking.openURL(url);
    } catch (error: any) {
      Alert.alert(`${label} failed`, error?.message ?? "Unknown error");
    }
  }

  function closeReportFlow() {
    setReportConfirmVisible(false);
    setReportResult(null);
    setReportModalVisible(false);
    setReportStatus("");
    setReportBusy(false);
    setReportComment("");
  }

  async function sendSettingsReportFlow() {
    if (reportBusy) return;
    logSettingsReportBreadcrumb("settings_report_send_started");
    setReportBusy(true);
    setReportConfirmVisible(false);
    setReportStatus("Preparing redacted report...");
    let payload: SettingsReportBundle;
    try {
      payload = await buildSettingsReportBundle({
        userNote: reportComment,
        runtimeState: state,
        events
      });
    } catch {
      setReportStatus("Report could not be prepared on this device. Please try again.");
      setReportBusy(false);
      return;
    }

    setReportStatus("Preparing report service...");
    logSettingsReportBreadcrumb("settings_report_transport_check_started");
    const readiness = await ensureSettingsReportTransportReady();
    if (!readiness.ok) {
      const failureClass = readiness.failure_class ?? classifyTransportFailure(readiness.reason);
      logSettingsReportBreadcrumb("settings_report_transport_failed", { class: failureClass });
      await queueSettingsReport(payload, `report transport ${readiness.reason}`, "not_ready", failureClass);
      return;
    }

    try {
      setReportStatus("Sending report to xMilo team...");
      logSettingsReportBreadcrumb("settings_report_submit_attempted");
      const proof = await submitSettingsReport(payload);
      if (proof.accepted === true && proof.report_id) {
        logSettingsReportBreadcrumb("settings_report_proof_accepted", { report_id_present: true });
        setReportResult({ kind: "sent", reportId: proof.report_id, copied: false });
        setReportStatus("");
        setReportComment("");
        setReportBusy(false);
        return;
      }
      logSettingsReportBreadcrumb("settings_report_proof_missing", { class: "proof_missing_accepted_or_report_id" });
      throw new Error("report service did not return accepted proof");
    } catch (error: unknown) {
      const failureClass = classifySubmitFailure(error);
      const safeStatus = error instanceof SidecarRequestError && error.status > 0 ? String(error.status) : undefined;
      logSettingsReportBreadcrumb("settings_report_submit_failed", { class: failureClass, status: safeStatus });
      await queueSettingsReport(
        payload,
        (error as Error | undefined)?.message ?? "report service unavailable",
        "unreachable",
        failureClass
      );
    }
  }

  async function queueSettingsReport(
    payload: SettingsReportBundle,
    lastError: string,
    reason: "unreachable" | "not_ready",
    failureClass: SettingsReportFailureClass
  ) {
    try {
      await enqueueSettingsReport({ payload, lastError, failureClass });
      setReportResult({ kind: "queued", reason });
      setReportStatus("");
      logSettingsReportBreadcrumb("settings_report_queued", { class: failureClass });
    } catch {
      logSettingsReportBreadcrumb("settings_report_submit_failed", { class: "queue_write_failed" });
      setReportStatus("Report could not be prepared on this device. Please try again.");
    } finally {
      setReportBusy(false);
    }
  }

  function classifyTransportFailure(reason: string): SettingsReportFailureClass {
    if (reason === "runtime_host_unavailable") return "transport_start_failed";
    if (reason === "sidecar_unreachable") return "sidecar_health_unreachable";
    if (reason === "relay_session_unavailable") return "sidecar_auth_check_failed";
    if (reason === "backend_unavailable") return "relay_rejected_or_unreachable";
    return "unknown_report_send_failure";
  }

  function classifySubmitFailure(error: unknown): SettingsReportFailureClass {
    const text = String((error as Error | undefined)?.message ?? error ?? "").toLowerCase();
    if (text.includes("accepted proof")) return "proof_missing_accepted_or_report_id";
    if (error instanceof SidecarRequestError) {
      if (error.status >= 500 || error.status === 401 || error.status === 403) return "relay_rejected_or_unreachable";
      return "sidecar_report_submit_failed";
    }
    if (text.includes("relay") || text.includes("bad gateway") || text.includes("502") || text.includes("503")) {
      return "relay_rejected_or_unreachable";
    }
    if (text.includes("runtime") || text.includes("warming") || text.includes("network")) return "sidecar_report_submit_failed";
    return "unknown_report_send_failure";
  }

  function logSettingsReportBreadcrumb(event: string, fields?: Record<string, string | boolean | undefined>) {
    const parts = [`xMilo ${event}`];
    for (const [key, value] of Object.entries(fields ?? {})) {
      if (typeof value === "undefined") continue;
      parts.push(`${key}=${String(value)}`);
    }
    console.info(parts.join(" "));
  }

  async function copyReportId(reportId: string) {
    await Clipboard.setStringAsync(reportId);
    setReportResult((current) => current?.kind === "sent" && current.reportId === reportId ? { ...current, copied: true } : current);
  }

  async function signOutFlow() {
    try {
      await authLogout();
      Alert.alert("Signed out", "Local relay session cleared from this device. You can sign in again whenever you are ready.");
    } catch (error: any) {
      Alert.alert("Sign out failed", error?.message ?? "Unknown error");
    }
  }

  async function deleteAccountFlow() {
    try {
      await authDeleteAccount();
      Alert.alert(
        "Account deleted",
        "xMilo removed the current verified account data it could delete immediately and cleared this phone's local session."
      );
    } catch (error: any) {
      Alert.alert("Delete account failed", error?.message ?? "Unknown error");
    }
  }

  return (
    <>
      <ScrollView style={styles.screen} contentContainerStyle={styles.content} contentInsetAdjustmentBehavior="automatic">
        <View style={styles.headerRow}>
          <Pressable
            style={styles.backButton}
            onPress={() => router.back()}
            accessibilityRole="button"
            accessibilityLabel="Go back"
          >
            <Text style={styles.backButtonText}>‹</Text>
          </Pressable>
          <View style={styles.headerCopy}>
            <Text style={styles.title}>Help & privacy</Text>
            <Text style={styles.subtitle}>Support, reports, privacy links, and account actions.</Text>
          </View>
        </View>

        <View style={styles.card}>
          <Text style={styles.cardTitle}>Report and support</Text>
          <Text style={styles.cardBody}>
            Send a redacted app report or open the existing xMilo support pages.
          </Text>
          <Pressable style={styles.primaryButton} onPress={openReportFlow}>
            <Text style={styles.buttonText}>Send report to xMilo team</Text>
          </Pressable>
          <Pressable style={styles.secondaryButton} onPress={() => openExternalUrl(SUPPORT_URL, "Support page")}>
            <Text style={styles.buttonText}>Open support page</Text>
          </Pressable>
        </View>

        <View style={styles.card}>
          <Text style={styles.cardTitle}>Privacy</Text>
          <Text style={styles.cardBody}>
            Open the current privacy and account-deletion pages.
          </Text>
          <Pressable style={styles.secondaryButton} onPress={() => openExternalUrl(PRIVACY_POLICY_URL, "Privacy policy")}>
            <Text style={styles.buttonText}>Open privacy policy</Text>
          </Pressable>
          <Pressable style={styles.secondaryButton} onPress={() => openExternalUrl(DELETE_ACCOUNT_URL, "Delete-account page")}>
            <Text style={styles.buttonText}>Open delete-account page</Text>
          </Pressable>
        </View>

        <View style={styles.card}>
          <Text style={styles.cardTitle}>Account</Text>
          <Text style={styles.cardBody}>
            Manage this phone's xMilo session or delete the current verified account.
          </Text>
          <Pressable style={styles.secondaryButton} onPress={signOutFlow}>
            <Text style={styles.buttonText}>Sign out on this phone</Text>
          </Pressable>
          <Pressable style={styles.dangerButton} onPress={deleteAccountFlow}>
            <Text style={styles.buttonText}>Delete this xMilo account</Text>
          </Pressable>
        </View>
      </ScrollView>

      <Modal
        animationType="fade"
        transparent
        visible={reportModalVisible}
        onRequestClose={closeReportFlow}
      >
        <KeyboardAvoidingView
          behavior={Platform.OS === "ios" ? "padding" : "height"}
          keyboardVerticalOffset={24}
          style={styles.modalKeyboardAvoider}
        >
          <View style={styles.modalScrim}>
            <View style={[styles.modalCard, styles.reportModalCard]}>
              <Pressable style={styles.modalCloseButton} onPress={closeReportFlow} accessibilityRole="button" accessibilityLabel="Close report modal">
                <Text style={styles.modalCloseText}>X</Text>
              </Pressable>
              {reportResult?.kind === "sent" ? (
                <>
                  <Text style={styles.modalTitle}>Report sent to xMilo team</Text>
                  <Text style={styles.modalBody}>Your redacted report was sent successfully.</Text>
                  <Pressable style={styles.reportIdButton} onPress={() => copyReportId(reportResult.reportId)}>
                    <Text selectable style={styles.reportIdText}>{reportResult.reportId}</Text>
                  </Pressable>
                  {reportResult.copied ? <Text style={styles.copiedText}>Report ID copied</Text> : null}
                  <View style={styles.modalButtonRow}>
                    <Pressable style={styles.primaryActionButton} onPress={closeReportFlow}>
                      <Text style={styles.buttonText}>Done</Text>
                    </Pressable>
                  </View>
                </>
              ) : reportResult?.kind === "queued" ? (
                <>
                  <Text style={styles.modalTitle}>Report queued on this device</Text>
                  <Text style={styles.modalBody}>
                    {reportResult.reason === "not_ready"
                      ? "xMilo could not prepare the report service on this device. Your redacted report is saved on this phone and has not been sent yet."
                      : "xMilo could not reach the report service. Your redacted report is saved on this phone and will be sent when the connection is available. It has not been sent yet."}
                  </Text>
                  <View style={styles.modalButtonRow}>
                    <Pressable style={styles.primaryActionButton} onPress={closeReportFlow}>
                      <Text style={styles.buttonText}>I understand</Text>
                    </Pressable>
                  </View>
                </>
              ) : (
                <>
                  <Text style={styles.modalTitle}>Send report to xMilo team</Text>
                  <ScrollView
                    style={styles.reportWarningScroll}
                    contentContainerStyle={styles.reportWarningContent}
                    keyboardShouldPersistTaps="handled"
                  >
                    <Text style={styles.modalBody}>
                      A report may include today's conversation, recent app/runtime logs and diagnostics, and the optional comment you add below. A human on the xMilo team may review it.
                    </Text>
                    <Text style={styles.modalBody}>
                      Stored API keys, tokens, auth headers, provider configs, hidden prompts, and private tool payloads must not be intentionally included.
                    </Text>
                    <Text style={styles.modalBody}>
                      If you typed a secret, password, or API key into chat, rotate it after sending a report.
                    </Text>
                    <TextInput
                      multiline
                      placeholder="Optional comment for the xMilo team"
                      placeholderTextColor="#94A3B8"
                      style={styles.reportInput}
                      value={reportComment}
                      onChangeText={setReportComment}
                      maxLength={1200}
                      textAlignVertical="top"
                    />
                    {reportStatus ? <Text style={styles.reportStatusText}>{reportStatus}</Text> : null}
                  </ScrollView>
                  <View style={styles.modalButtonRow}>
                    <Pressable style={[styles.secondaryActionButton, reportBusy && styles.disabledButton]} onPress={closeReportFlow} disabled={reportBusy}>
                      <Text style={styles.buttonText}>Cancel</Text>
                    </Pressable>
                    <Pressable
                      style={[styles.primaryActionButton, reportBusy && styles.disabledButton]}
                      onPress={() => setReportConfirmVisible(true)}
                      disabled={reportBusy}
                    >
                      {reportBusy ? <ActivityIndicator color="#FFFFFF" /> : <Text style={styles.buttonText}>Send now</Text>}
                    </Pressable>
                  </View>
                </>
              )}
            </View>
          </View>
        </KeyboardAvoidingView>
      </Modal>
      <Modal
        animationType="fade"
        transparent
        visible={reportConfirmVisible}
        onRequestClose={() => setReportConfirmVisible(false)}
      >
        <View style={styles.modalScrim}>
          <View style={styles.modalCard}>
            <Pressable
              style={styles.modalCloseButton}
              onPress={() => setReportConfirmVisible(false)}
              accessibilityRole="button"
              accessibilityLabel="Close confirmation modal"
            >
              <Text style={styles.modalCloseText}>X</Text>
            </Pressable>
            <Text style={styles.modalTitle}>Confirm report</Text>
            <Text style={styles.modalBody}>Are you sure you want to send this report?</Text>
            <View style={styles.modalButtonRow}>
              <Pressable style={[styles.secondaryActionButton, reportBusy && styles.disabledButton]} onPress={() => setReportConfirmVisible(false)} disabled={reportBusy}>
                <Text style={styles.buttonText}>Cancel</Text>
              </Pressable>
              <Pressable style={[styles.primaryActionButton, reportBusy && styles.disabledButton]} onPress={sendSettingsReportFlow} disabled={reportBusy}>
                {reportBusy ? <ActivityIndicator color="#FFFFFF" /> : <Text style={styles.buttonText}>Yes, send</Text>}
              </Pressable>
            </View>
          </View>
        </View>
      </Modal>
    </>
  );
}

const styles = StyleSheet.create({
  screen: { flex: 1, backgroundColor: "#0B1020" },
  content: { padding: 16, gap: 16 },
  headerRow: { flexDirection: "row", alignItems: "center", gap: 12 },
  backButton: {
    width: 42,
    height: 42,
    borderRadius: 14,
    backgroundColor: "#13213D",
    borderWidth: 1,
    borderColor: "#334155",
    alignItems: "center",
    justifyContent: "center"
  },
  backButtonText: { color: "#E5E7EB", fontSize: 30, fontWeight: "700", lineHeight: 32 },
  headerCopy: { flex: 1 },
  title: { color: "#F8FAFC", fontSize: 24, fontWeight: "700" },
  subtitle: { color: "#94A3B8", marginTop: 4, lineHeight: 20 },
  card: {
    backgroundColor: "#111827",
    borderRadius: 18,
    padding: 16,
    borderWidth: 1,
    borderColor: "#1F2937"
  },
  cardTitle: { color: "#F8FAFC", fontSize: 18, fontWeight: "700" },
  cardBody: { color: "#CBD5E1", lineHeight: 21, marginTop: 8 },
  primaryButton: { marginTop: 16, backgroundColor: "#2563EB", borderRadius: 14, paddingVertical: 14, alignItems: "center" },
  secondaryButton: { marginTop: 12, backgroundColor: "#1F2937", borderRadius: 14, paddingVertical: 14, alignItems: "center" },
  dangerButton: { marginTop: 12, backgroundColor: "#3A1118", borderRadius: 14, paddingVertical: 14, alignItems: "center", borderWidth: 1, borderColor: "#7F1D1D" },
  disabledButton: { opacity: 0.55 },
  buttonText: { color: "#FFFFFF", fontWeight: "700" },
  modalKeyboardAvoider: { flex: 1 },
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
  modalCloseButton: {
    position: "absolute",
    top: 10,
    right: 10,
    width: 32,
    height: 32,
    borderRadius: 16,
    alignItems: "center",
    justifyContent: "center",
    backgroundColor: "#1F2937",
    borderWidth: 1,
    borderColor: "#334155",
    zIndex: 2
  },
  modalCloseText: { color: "#CBD5E1", fontSize: 14, fontWeight: "800" },
  modalTitle: { color: "#F8FAFC", fontSize: 20, fontWeight: "700" },
  modalBody: { color: "#CBD5E1", lineHeight: 21, marginTop: 10 },
  reportModalCard: { maxWidth: 420, maxHeight: "88%" },
  reportWarningScroll: { marginTop: 2, flexShrink: 1 },
  reportWarningContent: { paddingBottom: 16 },
  reportInput: {
    marginTop: 16,
    minHeight: 104,
    backgroundColor: "#0F172A",
    borderWidth: 1,
    borderColor: "#334155",
    borderRadius: 14,
    paddingHorizontal: 14,
    paddingVertical: 12,
    color: "#F8FAFC"
  },
  reportIdButton: {
    marginTop: 14,
    backgroundColor: "#0F172A",
    borderWidth: 1,
    borderColor: "#334155",
    borderRadius: 12,
    paddingHorizontal: 12,
    paddingVertical: 12
  },
  reportIdText: { color: "#BFDBFE", fontWeight: "800" },
  copiedText: { color: "#86EFAC", fontWeight: "700", marginTop: 10 },
  reportStatusText: { color: "#FCD34D", lineHeight: 20, marginTop: 12, fontWeight: "700" },
  modalButtonRow: { flexDirection: "row", gap: 12, justifyContent: "flex-end", marginTop: 16 },
  primaryActionButton: {
    minWidth: 112,
    backgroundColor: "#2563EB",
    borderRadius: 14,
    paddingHorizontal: 16,
    paddingVertical: 14,
    alignItems: "center"
  },
  secondaryActionButton: {
    minWidth: 96,
    backgroundColor: "#1F2937",
    borderRadius: 14,
    paddingHorizontal: 16,
    paddingVertical: 14,
    alignItems: "center"
  }
});
