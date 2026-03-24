import { useEffect, useRef } from "react";
import { Vibration } from "react-native";
import * as Notifications from "expo-notifications";
import * as Speech from "expo-speech";
import { useApp } from "../state/AppContext";

const vibrationPatterns: Record<string, number[]> = {
  deferred: [0, 180, 140, 180],
  started: [0, 260],
  completed: [0, 120, 100, 120, 100, 120]
};

export function useNightlyRitualCues() {
  const { nightlyRitual, speakAloudEnabled } = useApp();
  const handledKeyRef = useRef<string | null>(null);

  useEffect(() => {
    if (!nightlyRitual) return;

    const key = `${nightlyRitual.status}:${nightlyRitual.event.timestamp}`;
    if (handledKeyRef.current === key) {
      return;
    }
    handledKeyRef.current = key;

    const vibrationPattern = vibrationPatterns[nightlyRitual.status];
    Vibration.vibrate(vibrationPattern);

    Notifications.getPermissionsAsync()
      .then((permissions) => {
        if (!permissions.granted) {
          return null;
        }

        return Notifications.scheduleNotificationAsync({
          content: {
            title: nightlyRitual.title,
            body: nightlyRitual.description,
            sound: "default"
          },
          trigger: null
        });
      })
      .catch(() => null);

    if (speakAloudEnabled) {
      Speech.stop();
      Speech.speak(nightlyRitual.vocalCue);
    }
  }, [nightlyRitual, speakAloudEnabled]);
}
