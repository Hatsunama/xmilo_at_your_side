import { useEffect, useRef } from "react";
import { AppState } from "react-native";
import { countQueuedSettingsReports, drainQueuedSettingsReports } from "../lib/reportQueue";
import { ensureSettingsReportTransportReady } from "../lib/reportTransport";
import type { SettingsReportProof } from "../lib/bridge";

const INITIAL_DRAIN_DELAY_MS = 2500;
const SUCCESS_COOLDOWN_MS = 15000;
const FAILURE_COOLDOWN_MS = 60000;
const FOREGROUND_RETRY_INTERVAL_MS = 30000;

export function useSettingsReportQueueDrain(options?: {
  onSent?: (proofs: SettingsReportProof[]) => void;
}) {
  const drainingRef = useRef(false);
  const nextAllowedAtRef = useRef(0);
  const mountedRef = useRef(false);
  const onSentRef = useRef(options?.onSent);

  useEffect(() => {
    onSentRef.current = options?.onSent;
  }, [options?.onSent]);

  useEffect(() => {
    mountedRef.current = true;

    async function runDrain() {
      const now = Date.now();
      if (drainingRef.current || now < nextAllowedAtRef.current) return;
      drainingRef.current = true;
      let started = false;
      let remainingCountForLog = 0;
      try {
        const queuedCount = await countQueuedSettingsReports();
        remainingCountForLog = queuedCount;
        if (!mountedRef.current || queuedCount <= 0) return;
        started = true;
        logSettingsReportQueueDrainBreadcrumb("settings_report_queue_drain_started");
        const readiness = await ensureSettingsReportTransportReady();
        if (!mountedRef.current) return;
        if (!readiness.ok) {
          nextAllowedAtRef.current = Date.now() + FAILURE_COOLDOWN_MS;
          logSettingsReportQueueDrainBreadcrumb("settings_report_queue_drain_completed", {
            sent_count: 0,
            remaining_count: queuedCount
          });
          return;
        }
        const result = await drainQueuedSettingsReports();
        if (!mountedRef.current) return;
        const remainingCount = await countQueuedSettingsReports();
        remainingCountForLog = remainingCount;
        logSettingsReportQueueDrainBreadcrumb("settings_report_queue_drain_completed", {
          sent_count: result.sent,
          remaining_count: remainingCount
        });
        if (result.sent > 0 && result.proofs.length > 0) {
          onSentRef.current?.(result.proofs);
        }
        nextAllowedAtRef.current = Date.now() + (result.failed > 0 ? FAILURE_COOLDOWN_MS : SUCCESS_COOLDOWN_MS);
      } catch {
        nextAllowedAtRef.current = Date.now() + FAILURE_COOLDOWN_MS;
        if (started) {
          logSettingsReportQueueDrainBreadcrumb("settings_report_queue_drain_completed", {
            sent_count: 0,
            remaining_count: remainingCountForLog
          });
        }
      } finally {
        drainingRef.current = false;
      }
    }

    const initialTimer = setTimeout(() => {
      void runDrain();
    }, INITIAL_DRAIN_DELAY_MS);

    const interval = setInterval(() => {
      if (AppState.currentState === "active") {
        void runDrain();
      }
    }, FOREGROUND_RETRY_INTERVAL_MS);

    const subscription = AppState.addEventListener("change", (nextState) => {
      if (nextState === "active") {
        void runDrain();
      }
    });

    return () => {
      mountedRef.current = false;
      clearTimeout(initialTimer);
      clearInterval(interval);
      subscription.remove();
    };
  }, []);
}

function logSettingsReportQueueDrainBreadcrumb(event: string, fields?: Record<string, string | number | boolean | undefined>) {
  const parts = [`xMilo ${event}`];
  for (const [key, value] of Object.entries(fields ?? {})) {
    if (typeof value === "undefined") continue;
    parts.push(`${key}=${String(value)}`);
  }
  console.info(parts.join(" "));
}
