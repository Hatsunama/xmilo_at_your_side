import { useEffect, useRef, useState } from "react";
import { useRouter } from "expo-router";
import { Alert, Pressable, ScrollView, StyleSheet, Text, View } from "react-native";

import {
  attemptRuntimeRecovery,
  initRuntimeRecoveryState,
  refreshRuntimeRecoveryState,
  RuntimeRecoveryState,
} from "../src/lib/runtimeRecovery";
import { hasNativeXMiloclawRuntimeHost } from "../src/lib/xmiloRuntimeHost";

type RuntimeRecoveryResult = NonNullable<RuntimeRecoveryState["last_result"]>["result"];

function formatResult(result?: RuntimeRecoveryResult) {
  if (!result) return "None yet";
  switch (result) {
    case "restart_verified":
      return "Restart verified";
    case "restart_failed":
      return "Restart failed";
    case "restart_rate_limited":
      return "Rate limited";
    case "restart_needs_operator":
      return "Needs operator";
    default:
      return result;
  }
}

export default function RuntimeRecoveryScreen() {
  const router = useRouter();
  const [state, setState] = useState<RuntimeRecoveryState>(() => initRuntimeRecoveryState());
  const stateRef = useRef(state);
  const nativeRuntimeAvailable = hasNativeXMiloclawRuntimeHost();

  useEffect(() => {
    stateRef.current = state;
  }, [state]);

  async function refreshNow() {
    const base = stateRef.current;
    const next = await refreshRuntimeRecoveryState(base).catch(() => base);
    setState(next);
  }

  async function attemptRestart() {
    if (!nativeRuntimeAvailable) {
      Alert.alert("Unavailable", "This build does not include the app-owned hidden runtime host.");
      return;
    }

    setState((current) => ({ ...current, attempting: true }));
    try {
      const { next } = await attemptRuntimeRecovery({ ...stateRef.current, attempting: true });
      setState(next);
    } catch (error: any) {
      setState((current) => ({
        ...current,
        attempting: false,
        last_result: {
          result: "restart_failed",
          at: new Date().toISOString(),
          note: error?.message ?? "unknown error",
        },
      }));
    }
  }

  useEffect(() => {
    let alive = true;
    refreshNow().catch(() => null);
    const interval = setInterval(() => {
      if (!alive) return;
      refreshNow().catch(() => null);
    }, 4000);
    return () => {
      alive = false;
      clearInterval(interval);
    };
  }, []);

  const lastCheck = state.last_check;
  const statusLabel =
    state.status === "healthy" ? "Healthy" : state.status === "unready" ? "Unready" : state.status === "down" ? "Down" : "Unknown";
  const canTryRestart = nativeRuntimeAvailable && !state.attempting && (state.status === "down" || state.status === "unready");

  return (
    <ScrollView style={styles.screen} contentContainerStyle={styles.content}>
      <View style={styles.card}>
        <Text style={styles.title}>Hidden Runtime Recovery</Text>
        <Text style={styles.body}>
          This screen uses the app-owned hidden runtime host only. No Termux path is active here.
        </Text>

        <View style={styles.row}>
          <Text style={styles.k}>Status</Text>
          <Text style={styles.v}>{statusLabel}</Text>
        </View>
        <View style={styles.row}>
          <Text style={styles.k}>Last check</Text>
          <Text style={styles.v}>{lastCheck?.checked_at ? new Date(lastCheck.checked_at).toLocaleString() : "—"}</Text>
        </View>
        <View style={styles.row}>
          <Text style={styles.k}>Runtime host</Text>
          <Text style={styles.v}>{lastCheck ? (lastCheck.runtime_host_started ? "Running" : "Stopped") : "—"}</Text>
        </View>
        <View style={styles.row}>
          <Text style={styles.k}>Bridge</Text>
          <Text style={styles.v}>{lastCheck ? (lastCheck.bridge_connected ? "Connected" : "Not connected") : "—"}</Text>
        </View>
        <View style={styles.row}>
          <Text style={styles.k}>Ready</Text>
          <Text style={styles.v}>{lastCheck ? (lastCheck.host_ready ? "Yes" : "No") : "—"}</Text>
        </View>
        <View style={styles.row}>
          <Text style={styles.k}>Attempts used</Text>
          <Text style={styles.v}>{String(state.attempts_used ?? 0)}</Text>
        </View>
        <View style={styles.row}>
          <Text style={styles.k}>Last result</Text>
          <Text style={styles.v}>{formatResult(state.last_result?.result)}</Text>
        </View>
        {state.last_result?.note ? <Text style={styles.hint}>Note: {state.last_result.note}</Text> : null}
        {state.next_allowed_at ? (
          <Text style={styles.hint}>Next allowed attempt: {new Date(state.next_allowed_at).toLocaleString()}</Text>
        ) : null}

        <Pressable style={styles.secondaryButton} onPress={refreshNow} disabled={state.attempting}>
          <Text style={styles.buttonText}>{state.attempting ? "Refreshing..." : "Refresh checks"}</Text>
        </Pressable>
        <Pressable style={[styles.button, !canTryRestart && styles.disabled]} onPress={attemptRestart} disabled={!canTryRestart}>
          <Text style={styles.buttonText}>{state.attempting ? "Attempting restart..." : "Attempt restart"}</Text>
        </Pressable>
        <Pressable style={styles.linkButton} onPress={() => router.back()}>
          <Text style={styles.linkText}>Back</Text>
        </Pressable>
      </View>
    </ScrollView>
  );
}

const styles = StyleSheet.create({
  screen: { flex: 1, backgroundColor: "#0B1020" },
  content: { padding: 16, gap: 14 },
  card: { backgroundColor: "#111827", borderRadius: 18, padding: 16, borderWidth: 1, borderColor: "#1F2937" },
  title: { color: "#F8FAFC", fontSize: 22, fontWeight: "700" },
  body: { color: "#CBD5E1", lineHeight: 22, marginTop: 8 },
  row: { flexDirection: "row", justifyContent: "space-between", alignItems: "center", marginTop: 10, gap: 10 },
  k: { color: "#94A3B8", fontSize: 13 },
  v: { color: "#F8FAFC", fontWeight: "700", fontSize: 13, textAlign: "right", flexShrink: 1 },
  hint: { color: "#7C6FA0", fontSize: 12, marginTop: 10 },
  button: { marginTop: 16, backgroundColor: "#2563EB", borderRadius: 14, paddingVertical: 14, alignItems: "center" },
  secondaryButton: { marginTop: 12, backgroundColor: "#1F2937", borderRadius: 14, paddingVertical: 14, alignItems: "center" },
  buttonText: { color: "#FFFFFF", fontWeight: "700" },
  linkButton: { marginTop: 12, alignItems: "center", paddingVertical: 10 },
  linkText: { color: "#93C5FD", fontWeight: "700" },
  disabled: { opacity: 0.5 },
});
