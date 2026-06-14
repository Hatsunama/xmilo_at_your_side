import { useState } from "react";
import { Stack, useRouter } from "expo-router";
import { SafeAreaProvider } from "react-native-safe-area-context";
import { Modal, Pressable, StatusBar, StyleSheet, Text, View } from "react-native";
import { GestureHandlerRootView } from "react-native-gesture-handler";
import * as Clipboard from "expo-clipboard";
import * as Notifications from "expo-notifications";
import { AppProvider } from "../src/state/AppContext";
import { useAngryShakeReportShortcut } from "../src/hooks/useAngryShakeReportShortcut";
import { useSettingsReportQueueDrain } from "../src/hooks/useSettingsReportQueueDrain";
import type { SettingsReportProof } from "../src/lib/bridge";
import { useWakeWord } from "../src/hooks/useWakeWord";
import { useNightlyRitualCues } from "../src/hooks/useNightlyRitualCues";

Notifications.setNotificationHandler({
  handleNotification: async () => ({
    shouldPlaySound: true,
    shouldSetBadge: false,
    shouldShowBanner: true,
    shouldShowList: true
  })
});

function WakeWordController() {
  useWakeWord();
  return null;
}

function NightlyRitualCueController() {
  useNightlyRitualCues();
  return null;
}

function SettingsReportQueueDrainController() {
  const [sentProof, setSentProof] = useState<SettingsReportProof | null>(null);
  const [copied, setCopied] = useState(false);

  useSettingsReportQueueDrain({
    onSent: (proofs) => {
      const proof = proofs.find((item) => item.accepted === true && Boolean(item.report_id));
      if (!proof) return;
      setCopied(false);
      setSentProof(proof);
    }
  });

  async function copyReportId() {
    if (!sentProof?.report_id) return;
    await Clipboard.setStringAsync(sentProof.report_id);
    setCopied(true);
  }

  function close() {
    setSentProof(null);
    setCopied(false);
  }

  return (
    <Modal
      animationType="fade"
      transparent
      visible={Boolean(sentProof?.report_id)}
      onRequestClose={close}
    >
      <View style={styles.modalScrim}>
        <View style={styles.modalCard}>
          <Pressable
            style={styles.modalCloseButton}
            onPress={close}
            accessibilityRole="button"
            accessibilityLabel="Close queued report sent message"
          >
            <Text style={styles.modalCloseText}>X</Text>
          </Pressable>
          <Text style={styles.modalTitle}>Queued report sent</Text>
          <Text style={styles.modalBody}>A saved report was sent to the xMilo team.</Text>
          <Pressable style={styles.reportIdButton} onPress={copyReportId}>
            <Text selectable style={styles.reportIdText}>{sentProof?.report_id ?? ""}</Text>
          </Pressable>
          {copied ? <Text style={styles.copiedText}>Report ID copied</Text> : null}
          <View style={styles.modalButtonRow}>
            <Pressable style={styles.primaryActionButton} onPress={close}>
              <Text style={styles.buttonText}>Done</Text>
            </Pressable>
          </View>
        </View>
      </View>
    </Modal>
  );
}

function AngryShakeReportController() {
  const router = useRouter();
  const angryShake = useAngryShakeReportShortcut();

  function sendReport() {
    angryShake.close();
    router.push({ pathname: "/help-privacy", params: { reportEntry: "angryShake" } });
  }

  return (
    <Modal
      animationType="fade"
      transparent
      visible={angryShake.visible}
      onRequestClose={angryShake.close}
    >
      <View style={styles.modalScrim}>
        <View style={styles.modalCard}>
          <Pressable
            style={styles.modalCloseButton}
            onPress={angryShake.close}
            accessibilityRole="button"
            accessibilityLabel="Close report shortcut"
          >
            <Text style={styles.modalCloseText}>X</Text>
          </Pressable>
          <Text style={styles.modalTitle}>Something went wrong?</Text>
          <Text style={styles.modalBody}>Want to report this interaction to the xMilo team?</Text>
          <View style={styles.modalButtonRow}>
            <Pressable style={styles.secondaryActionButton} onPress={angryShake.close}>
              <Text style={styles.buttonText}>No thank you</Text>
            </Pressable>
            <Pressable style={styles.primaryActionButton} onPress={sendReport}>
              <Text style={styles.buttonText}>Send report</Text>
            </Pressable>
          </View>
        </View>
      </View>
    </Modal>
  );
}

export default function RootLayout() {
  return (
    <GestureHandlerRootView style={styles.gestureRoot}>
      <SafeAreaProvider>
        <AppProvider>
          <WakeWordController />
          <NightlyRitualCueController />
          <SettingsReportQueueDrainController />
          <AngryShakeReportController />
          <StatusBar barStyle="light-content" />
          <Stack
            screenOptions={{
              headerStyle: { backgroundColor: "#111827" },
              headerTintColor: "#E5E7EB",
              contentStyle: { backgroundColor: "#0B1020" }
            }}
          >
            <Stack.Screen name="index" options={{ title: "xMilo" }} />
            <Stack.Screen name="setup" options={{ title: "Setup" }} />
            <Stack.Screen name="settings" options={{ title: "Settings" }} />
            <Stack.Screen name="help-privacy" options={{ title: "Help & privacy", headerShown: false }} />
            <Stack.Screen name="archive" options={{ title: "Archive" }} />
            <Stack.Screen name="memory/index" options={{ title: "Memory", headerShown: false }} />
            <Stack.Screen name="memory/[memoryId]" options={{ title: "Memory detail", headerShown: false }} />
            <Stack.Screen
              name="lair"
              options={{
                title: "Wizard Lair",
                headerShown: false,
                contentStyle: { backgroundColor: "transparent" },
              }}
            />
          </Stack>
        </AppProvider>
      </SafeAreaProvider>
    </GestureHandlerRootView>
  );
}

const styles = StyleSheet.create({
  gestureRoot: { flex: 1 },
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
  modalButtonRow: { flexDirection: "row", gap: 12, justifyContent: "flex-end", marginTop: 18 },
  primaryActionButton: {
    minWidth: 112,
    backgroundColor: "#2563EB",
    borderRadius: 14,
    paddingHorizontal: 16,
    paddingVertical: 14,
    alignItems: "center"
  },
  secondaryActionButton: {
    minWidth: 104,
    backgroundColor: "#1F2937",
    borderRadius: 14,
    paddingHorizontal: 16,
    paddingVertical: 14,
    alignItems: "center"
  },
  buttonText: { color: "#FFFFFF", fontWeight: "700" }
});
