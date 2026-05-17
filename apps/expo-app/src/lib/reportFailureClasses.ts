const SETTINGS_REPORT_FAILURE_CLASSES = [
  "transport_start_failed",
  "sidecar_health_unreachable",
  "sidecar_auth_check_failed",
  "sidecar_report_submit_failed",
  "relay_rejected_or_unreachable",
  "proof_missing_accepted_or_report_id",
  "queue_write_failed",
  "unknown_report_send_failure",
  "relay_unreachable",
  "relay_http_error",
  "relay_bad_response",
  "relay_empty_jwt",
  "relay_config_missing",
  "relay_session_store_failed",
  "relay_session_unknown_failure"
] as const;

const SIDECAR_AUTH_SESSION_FAILURE_CLASSES = [
  "relay_unreachable",
  "relay_http_error",
  "relay_bad_response",
  "relay_empty_jwt",
  "relay_config_missing",
  "relay_session_store_failed",
  "relay_session_unknown_failure"
] as const;

export type SettingsReportFailureClass = (typeof SETTINGS_REPORT_FAILURE_CLASSES)[number];
export type SidecarAuthSessionFailureClass = (typeof SIDECAR_AUTH_SESSION_FAILURE_CLASSES)[number];

export function isSettingsReportFailureClass(value: unknown): value is SettingsReportFailureClass {
  return typeof value === "string" && SETTINGS_REPORT_FAILURE_CLASSES.includes(value as SettingsReportFailureClass);
}

export function isSidecarAuthSessionFailureClass(value: unknown): value is SidecarAuthSessionFailureClass {
  return typeof value === "string" && SIDECAR_AUTH_SESSION_FAILURE_CLASSES.includes(value as SidecarAuthSessionFailureClass);
}

export function sanitizeSettingsReportFailureClass(
  value: unknown,
  fallback: SettingsReportFailureClass = "unknown_report_send_failure"
): SettingsReportFailureClass {
  return isSettingsReportFailureClass(value) ? value : fallback;
}
