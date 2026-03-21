import { Alert, Pressable, ScrollView, StyleSheet, Switch, Text, View } from "react-native";
import { useApp } from "../src/state/AppContext";
import { authCheck, getState, getStorageStats, resetTier } from "../src/lib/bridge";
import { configureRevenueCat, isRevenueCatConfigured, restoreRevenueCatPurchases } from "../src/lib/revenuecat";

export default function SettingsScreen() {
  const { state, storageStats, setStorageStats, wakeWordEnabled, setWakeWordEnabled } = useApp();

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

  return (
    <ScrollView style={styles.screen} contentContainerStyle={styles.content}>
      <View style={styles.card}>
        <Text style={styles.title}>Settings</Text>
        <Text style={styles.body}>
          This starter screen still focuses on bridge-driven actions first. Restore Purchases is now wired through RevenueCat. Log out, invite management, and deeper reset flows remain tracked separately.
        </Text>

        <Pressable style={styles.button} onPress={refreshStats}>
          <Text style={styles.buttonText}>Refresh storage stats</Text>
        </Pressable>
        <Pressable style={styles.secondaryButton} onPress={restorePurchases}>
          <Text style={styles.buttonText}>Restore purchases</Text>
        </Pressable>
        <Pressable style={styles.secondaryButton} onPress={wipeChatCache}>
          <Text style={styles.buttonText}>Reset chat cache only</Text>
        </Pressable>

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

        <View style={styles.statsCard}>
          <Text style={styles.statsTitle}>Storage</Text>
          <Text style={styles.statsRow}>SQLite bytes: {String(storageStats?.pico_sqlite_bytes ?? "unknown")}</Text>
          <Text style={styles.statsRow}>Task rows: {String(storageStats?.runtime_task_rows ?? "unknown")}</Text>
          <Text style={styles.statsRow}>Pending events: {String(storageStats?.pending_event_rows ?? "unknown")}</Text>
          <Text style={styles.statsRow}>Conversation rows: {String(storageStats?.conversation_tail_rows ?? "unknown")}</Text>
        </View>
      </View>
    </ScrollView>
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
});
