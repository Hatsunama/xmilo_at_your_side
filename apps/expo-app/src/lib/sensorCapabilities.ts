import { Accelerometer, type AccelerometerMeasurement } from "expo-sensors";

export type SensorCapabilityStatus = {
  capability_id: "accelerometer";
  declared: boolean;
  dependency_installed: boolean;
  permission: "not_required" | "unknown" | "unavailable";
  hardware_available: boolean | null;
  runtime_tested: boolean;
  runtime_ok: boolean;
  availability_scope: "foreground_only" | "unsupported" | "unknown";
  checked_at: string;
  last_tested_at?: string;
  failure_stage?: "dependency" | "hardware" | "runtime" | "timeout" | "unknown";
  error_code?: string;
  note?: string;
};

type ProbeOptions = {
  timeoutMs?: number;
  updateIntervalMs?: number;
};

const DEFAULT_TIMEOUT_MS = 1200;
const DEFAULT_UPDATE_INTERVAL_MS = 50;

export async function probeAccelerometerCapability(options: ProbeOptions = {}): Promise<SensorCapabilityStatus> {
  const checkedAt = new Date().toISOString();
  const timeoutMs = normalizePositiveInteger(options.timeoutMs, DEFAULT_TIMEOUT_MS);
  const updateIntervalMs = normalizePositiveInteger(options.updateIntervalMs, DEFAULT_UPDATE_INTERVAL_MS);

  const base = (): SensorCapabilityStatus => ({
    capability_id: "accelerometer",
    declared: true,
    dependency_installed: Boolean(Accelerometer),
    permission: "not_required",
    hardware_available: null,
    runtime_tested: false,
    runtime_ok: false,
    availability_scope: "unknown",
    checked_at: checkedAt,
  });

  if (!Accelerometer) {
    return {
      ...base(),
      dependency_installed: false,
      permission: "unavailable",
      availability_scope: "unsupported",
      failure_stage: "dependency",
      error_code: "accelerometer_module_unavailable",
      note: "expo-sensors Accelerometer module is not available in this build.",
    };
  }

  let hardwareAvailable: boolean | null = null;
  try {
    hardwareAvailable = typeof Accelerometer.isAvailableAsync === "function"
      ? await Accelerometer.isAvailableAsync()
      : null;
  } catch (error) {
    return {
      ...base(),
      hardware_available: null,
      availability_scope: "unknown",
      failure_stage: "hardware",
      error_code: sanitizeErrorCode(error),
      note: "Accelerometer hardware availability check failed.",
    };
  }

  if (hardwareAvailable === false) {
    return {
      ...base(),
      hardware_available: false,
      availability_scope: "unsupported",
      failure_stage: "hardware",
      note: "Accelerometer hardware is not available on this device.",
    };
  }

  try {
    Accelerometer.setUpdateInterval(updateIntervalMs);
  } catch (error) {
    return {
      ...base(),
      hardware_available: hardwareAvailable,
      runtime_tested: true,
      runtime_ok: false,
      availability_scope: "unknown",
      last_tested_at: new Date().toISOString(),
      failure_stage: "runtime",
      error_code: sanitizeErrorCode(error),
      note: "Accelerometer update interval could not be set.",
    };
  }

  return new Promise<SensorCapabilityStatus>((resolve) => {
    let settled = false;
    let subscription: { remove: () => void } | null = null;
    let timeout: ReturnType<typeof setTimeout> | null = null;

    const cleanup = () => {
      if (timeout) {
        clearTimeout(timeout);
        timeout = null;
      }
      if (subscription) {
        subscription.remove();
        subscription = null;
      }
    };

    const finish = (status: SensorCapabilityStatus) => {
      if (settled) return;
      settled = true;
      cleanup();
      resolve(status);
    };

    timeout = setTimeout(() => {
      finish({
        ...base(),
        hardware_available: hardwareAvailable,
        runtime_tested: true,
        runtime_ok: false,
        availability_scope: "unknown",
        last_tested_at: new Date().toISOString(),
        failure_stage: "timeout",
        error_code: "accelerometer_sample_timeout",
        note: "No accelerometer sample was received before the probe timed out.",
      });
    }, timeoutMs);

    try {
      subscription = Accelerometer.addListener((measurement) => {
        if (!isFiniteAccelerometerMeasurement(measurement)) return;
        finish({
          ...base(),
          hardware_available: true,
          runtime_tested: true,
          runtime_ok: true,
          availability_scope: "foreground_only",
          last_tested_at: new Date().toISOString(),
          note: "Accelerometer delivered a foreground runtime sample.",
        });
      });
    } catch (error) {
      finish({
        ...base(),
        hardware_available: hardwareAvailable,
        runtime_tested: true,
        runtime_ok: false,
        availability_scope: "unknown",
        last_tested_at: new Date().toISOString(),
        failure_stage: "runtime",
        error_code: sanitizeErrorCode(error),
        note: "Accelerometer foreground subscription failed.",
      });
    }
  });
}

function isFiniteAccelerometerMeasurement(measurement: AccelerometerMeasurement) {
  return Number.isFinite(measurement.x) && Number.isFinite(measurement.y) && Number.isFinite(measurement.z);
}

function normalizePositiveInteger(value: number | undefined, fallback: number) {
  if (typeof value !== "number" || !Number.isFinite(value) || value <= 0) return fallback;
  return Math.round(value);
}

function sanitizeErrorCode(error: unknown) {
  const raw = error instanceof Error ? error.message : String(error ?? "unknown_error");
  const safe = raw
    .replace(/eyJ[A-Za-z0-9_-]{10,}\.[A-Za-z0-9_-]{10,}\.[A-Za-z0-9_-]{10,}/g, "redacted_jwt")
    .replace(/authorization\s*[:=]\s*bearer\s+[^\s,"'}]+/gi, "authorization_bearer_redacted")
    .replace(/bearer\s+[A-Za-z0-9._-]{16,}/gi, "bearer_redacted")
    .replace(/(api[_-]?key|provider[_-]?key|secret|token|password)\s*[:=]\s*[^\s,"'}]+/gi, "$1_redacted")
    .replace(/[^A-Za-z0-9_.:-]/g, "_")
    .replace(/_+/g, "_")
    .slice(0, 96);
  return safe || "unknown_error";
}
