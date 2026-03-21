/**
 * useWakeWord — listens continuously for the phrase "xMilo" (and common
 * mishearings) and has Milo speak a random wake message via TTS.
 *
 * Lifecycle:
 *   - Starts speech recognition when wakeWordEnabled becomes true.
 *   - Stops and cleans up when wakeWordEnabled becomes false.
 *   - Auto-restarts recognition after each recognition session ends
 *     (Android and iOS stop after silence; we immediately restart).
 *   - A 3-second cooldown prevents rapid re-triggering.
 *
 * Mount point: WakeWordController in app/_layout.tsx (inside AppProvider).
 */

import { useEffect, useRef } from "react";
import * as Speech from "expo-speech";
import {
  ExpoSpeechRecognitionModule,
  useSpeechRecognitionEvent,
} from "expo-speech-recognition";
import { useApp } from "../state/AppContext";

// ─── Wake messages ─────────────────────────────────────────────────────────────
// Several varied responses so Milo doesn't feel like a broken record.

const WAKE_MESSAGES = [
  "I'm here. What do you need?",
  "You called?",
  "Ah, you summoned me.",
  "At your service.",
  "Ready when you are.",
  "Yes? I'm listening.",
  "Awake and ready.",
  "What can I do for you?",
];

// ─── Wake word patterns ────────────────────────────────────────────────────────
// Covers common speech-to-text variants of "xMilo".

const WAKE_PATTERNS = [
  "xmilo",
  "x milo",
  "hey milo",
  "hey xmilo",
  "hey x milo",
  "zmilo",
  "z milo",
  "exile o",
  "ex milo",
];

function containsWakeWord(text: string): boolean {
  const lower = text.toLowerCase().trim();
  return WAKE_PATTERNS.some((p) => lower.includes(p));
}

function randomWakeMessage(): string {
  return WAKE_MESSAGES[Math.floor(Math.random() * WAKE_MESSAGES.length)];
}

// ─── Hook ──────────────────────────────────────────────────────────────────────

export function useWakeWord() {
  const { wakeWordEnabled } = useApp();

  // Refs used inside async callbacks and event handlers to avoid stale closures.
  const activeRef = useRef(false);     // true while we should be listening
  const cooldownRef = useRef(false);   // true during the post-wake cooldown
  const restartTimerRef = useRef<ReturnType<typeof setTimeout> | null>(null);

  // ── Start recognition ────────────────────────────────────────────────────────
  async function startListening() {
    if (!activeRef.current) return;

    const { granted } = await ExpoSpeechRecognitionModule.requestPermissionsAsync();
    if (!granted || !activeRef.current) return;

    ExpoSpeechRecognitionModule.start({
      lang: "en-US",
      interimResults: true,
      continuous: false,         // we manually restart on "end" for reliability
      requiresOnDeviceRecognition: false,
    });
  }

  // ── Enable / disable ─────────────────────────────────────────────────────────
  useEffect(() => {
    if (wakeWordEnabled) {
      activeRef.current = true;
      startListening();
    } else {
      activeRef.current = false;
      if (restartTimerRef.current !== null) {
        clearTimeout(restartTimerRef.current);
        restartTimerRef.current = null;
      }
      ExpoSpeechRecognitionModule.stop();
    }

    return () => {
      activeRef.current = false;
      if (restartTimerRef.current !== null) {
        clearTimeout(restartTimerRef.current);
        restartTimerRef.current = null;
      }
      ExpoSpeechRecognitionModule.stop();
    };
  // startListening is defined inside the hook — stable identity, no dep needed.
  // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [wakeWordEnabled]);

  // ── Speech result handler ─────────────────────────────────────────────────────
  useSpeechRecognitionEvent("result", (event) => {
    if (!activeRef.current || cooldownRef.current) return;

    // Check every final result in this event batch.
    const transcripts = event.results
      .filter((r) => r.transcript)
      .map((r) => r.transcript);

    const triggered = transcripts.some(containsWakeWord);

    if (triggered) {
      cooldownRef.current = true;
      Speech.speak(randomWakeMessage(), {
        onDone: () => {
          cooldownRef.current = false;
        },
        onError: () => {
          cooldownRef.current = false;
        },
      });
    }
  });

  // ── Auto-restart on session end ───────────────────────────────────────────────
  // Android and iOS both terminate a recognition session after a period of
  // silence. We restart automatically so the wake word stays always-on.
  useSpeechRecognitionEvent("end", () => {
    if (!activeRef.current) return;
    // Small delay avoids hammering the recognition API on rapid error/end cycles.
    restartTimerRef.current = setTimeout(() => {
      restartTimerRef.current = null;
      startListening();
    }, 600);
  });
}
