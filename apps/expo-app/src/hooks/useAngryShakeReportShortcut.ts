import { useCallback, useEffect, useRef, useState } from "react";
import { Accelerometer, type AccelerometerMeasurement } from "expo-sensors";
import { probeAccelerometerCapability } from "../lib/sensorCapabilities";

type ShakeSample = {
  at: number;
  delta: number;
  dominant: "x" | "y" | "z";
  value: number;
};

const UPDATE_INTERVAL_MS = 50;
const WINDOW_MS = 900;
const PEAK_THRESHOLD_G = 1.25;
const REQUIRED_PEAKS = 3;
const REQUIRED_REVERSALS = 2;
const COOLDOWN_MS = 8000;

export function useAngryShakeReportShortcut() {
  const [visible, setVisible] = useState(false);
  const visibleRef = useRef(false);
  const samplesRef = useRef<ShakeSample[]>([]);
  const lastTriggerAtRef = useRef(0);

  const close = useCallback(() => {
    setVisible(false);
  }, []);

  useEffect(() => {
    visibleRef.current = visible;
  }, [visible]);

  useEffect(() => {
    let disposed = false;
    let subscription: { remove: () => void } | null = null;

    async function start() {
      try {
        const capability = await probeAccelerometerCapability({
          timeoutMs: 1200,
          updateIntervalMs: UPDATE_INTERVAL_MS,
        });
        if (disposed || !capability.runtime_ok || capability.availability_scope !== "foreground_only") {
          return;
        }
        Accelerometer.setUpdateInterval(UPDATE_INTERVAL_MS);
        subscription = Accelerometer.addListener((measurement) => {
          if (disposed) return;
          handleMeasurement(measurement);
        });
      } catch {
        return;
      }
    }

    function handleMeasurement(measurement: AccelerometerMeasurement) {
      if (!isFiniteMeasurement(measurement)) return;
      const now = Date.now();
      samplesRef.current = [...samplesRef.current, toShakeSample(measurement, now)].filter((sample) => now - sample.at <= WINDOW_MS);
      if (visibleRef.current) return;
      if (now - lastTriggerAtRef.current < COOLDOWN_MS) return;
      if (!isDeliberateShake(samplesRef.current)) return;
      lastTriggerAtRef.current = now;
      samplesRef.current = [];
      setVisible(true);
    }

    void start();

    return () => {
      disposed = true;
      subscription?.remove();
      subscription = null;
    };
  }, []);

  return {
    visible,
    close,
    calibration: {
      updateIntervalMs: UPDATE_INTERVAL_MS,
      windowMs: WINDOW_MS,
      peakThresholdG: PEAK_THRESHOLD_G,
      requiredPeaks: REQUIRED_PEAKS,
      requiredReversals: REQUIRED_REVERSALS,
      cooldownMs: COOLDOWN_MS,
    },
  };
}

function isDeliberateShake(samples: ShakeSample[]) {
  const peaks = samples.filter((sample) => sample.delta >= PEAK_THRESHOLD_G);
  if (peaks.length < REQUIRED_PEAKS) return false;
  return countDominantAxisReversals(peaks) >= REQUIRED_REVERSALS;
}

function countDominantAxisReversals(samples: ShakeSample[]) {
  let reversals = 0;
  let lastSign = 0;
  let lastAxis: ShakeSample["dominant"] | null = null;
  for (const sample of samples) {
    const sign = sample.value > 0 ? 1 : sample.value < 0 ? -1 : 0;
    if (sign === 0) continue;
    if (lastAxis === sample.dominant && lastSign !== 0 && sign !== lastSign) {
      reversals += 1;
    }
    lastAxis = sample.dominant;
    lastSign = sign;
  }
  return reversals;
}

function toShakeSample(measurement: AccelerometerMeasurement, at: number): ShakeSample {
  const magnitude = Math.sqrt((measurement.x * measurement.x) + (measurement.y * measurement.y) + (measurement.z * measurement.z));
  const dominant = dominantAxis(measurement);
  return {
    at,
    delta: Math.abs(magnitude - 1),
    dominant,
    value: measurement[dominant],
  };
}

function dominantAxis(measurement: AccelerometerMeasurement): ShakeSample["dominant"] {
  const absX = Math.abs(measurement.x);
  const absY = Math.abs(measurement.y);
  const absZ = Math.abs(measurement.z);
  if (absX >= absY && absX >= absZ) return "x";
  if (absY >= absX && absY >= absZ) return "y";
  return "z";
}

function isFiniteMeasurement(measurement: AccelerometerMeasurement) {
  return Number.isFinite(measurement.x) && Number.isFinite(measurement.y) && Number.isFinite(measurement.z);
}
