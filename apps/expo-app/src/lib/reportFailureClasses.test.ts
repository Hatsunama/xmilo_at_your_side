import {
  isSettingsReportFailureClass,
  isSidecarAuthSessionFailureClass,
  sanitizeSettingsReportFailureClass
} from "./reportFailureClasses";

describe("report failure classes", () => {
  it("accepts bounded app and sidecar failure classes", () => {
    expect(isSettingsReportFailureClass("sidecar_health_unreachable")).toBe(true);
    expect(isSettingsReportFailureClass("relay_session_store_failed")).toBe(true);
    expect(isSidecarAuthSessionFailureClass("relay_session_store_failed")).toBe(true);
  });

  it("rejects unknown or unsafe failure classes", () => {
    expect(isSettingsReportFailureClass("relay_totally_new_failure")).toBe(false);
    expect(isSettingsReportFailureClass("unsafe_unbounded_value")).toBe(false);
    expect(isSidecarAuthSessionFailureClass("sidecar_health_unreachable")).toBe(false);
  });

  it("sanitizes arbitrary values to a bounded fallback", () => {
    expect(sanitizeSettingsReportFailureClass("relay_http_error")).toBe("relay_http_error");
    expect(sanitizeSettingsReportFailureClass("unexpected")).toBe("unknown_report_send_failure");
    expect(sanitizeSettingsReportFailureClass(null, "sidecar_report_submit_failed")).toBe("sidecar_report_submit_failed");
  });
});
