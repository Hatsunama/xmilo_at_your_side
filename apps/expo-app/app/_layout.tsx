import { Stack } from "expo-router";
import { SafeAreaProvider } from "react-native-safe-area-context";
import { StatusBar } from "react-native";
import * as Notifications from "expo-notifications";
import { AppProvider } from "../src/state/AppContext";
import { useWakeWord } from "../src/hooks/useWakeWord";
import { useNightlyRitualCues } from "../src/hooks/useNightlyRitualCues";
import { PersistentCastleSurfaceProvider } from "../src/components/PersistentCastleSurface";

Notifications.setNotificationHandler({
  handleNotification: async () => ({
    shouldPlaySound: true,
    shouldSetBadge: false,
    shouldShowBanner: true,
    shouldShowList: true
  })
});

// Mounts inside AppProvider so it can read wakeWordEnabled from context.
function WakeWordController() {
  useWakeWord();
  return null;
}

function NightlyRitualCueController() {
  useNightlyRitualCues();
  return null;
}

export default function RootLayout() {
  return (
    <SafeAreaProvider>
      <AppProvider>
        <PersistentCastleSurfaceProvider>
          <WakeWordController />
          <NightlyRitualCueController />
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
            <Stack.Screen name="archive" options={{ title: "Archive" }} />
            <Stack.Screen
              name="lair"
              options={{
                title: "Wizard Lair",
                headerShown: false,
                contentStyle: { backgroundColor: "transparent" },
              }}
            />
          </Stack>
        </PersistentCastleSurfaceProvider>
      </AppProvider>
    </SafeAreaProvider>
  );
}
