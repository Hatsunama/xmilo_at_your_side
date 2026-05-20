package db

import (
	"strings"
	"testing"
)

func TestSettingsReportStatusMigrationConstants(t *testing.T) {
	got := map[string]bool{}
	for _, status := range settingsReportCanonicalStatuses {
		got[status] = true
	}
	for _, status := range []string{"new", "triaged", "resolved", "dismissed", "needs_followup"} {
		if !got[status] {
			t.Fatalf("canonical status missing: %s", status)
		}
	}
	for _, stale := range []string{"confirmed", "rule_update_needed"} {
		if got[stale] {
			t.Fatalf("stale status must not be canonical: %s", stale)
		}
	}

	if settingsReportStaleStatusMap["confirmed"] != "resolved" {
		t.Fatalf("confirmed must map to resolved")
	}
	if settingsReportStaleStatusMap["rule_update_needed"] != "needs_followup" {
		t.Fatalf("rule_update_needed must map to needs_followup")
	}
}

func TestFormatUnexpectedSettingsReportStatuses(t *testing.T) {
	msg := formatUnexpectedSettingsReportStatuses(map[string]int64{
		"zzz": 2,
		"aaa": 1,
	})
	if msg != "aaa=1, zzz=2" {
		t.Fatalf("unexpected stable status message: %s", msg)
	}
	if strings.Contains(msg, "confirmed") || strings.Contains(msg, "rule_update_needed") {
		t.Fatalf("unexpected status message should not coerce stale statuses: %s", msg)
	}
}
