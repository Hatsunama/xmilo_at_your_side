package httpx

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"

	"xmilo/relay-go/internal/config"
	"xmilo/relay-go/internal/sessions"
)

type fakeSettingsReportRepo struct {
	records  map[string]settingsReportRecord
	byClient map[string]string
	now      time.Time
}

func newFakeSettingsReportRepo() *fakeSettingsReportRepo {
	return &fakeSettingsReportRepo{
		records:  map[string]settingsReportRecord{},
		byClient: map[string]string{},
		now:      time.Date(2026, 5, 20, 12, 0, 0, 0, time.UTC),
	}
}

func (f *fakeSettingsReportRepo) StoreSettingsReport(_ context.Context, deviceUserID string, req settingsReportRequest, bundleJSON []byte) (settingsReportProof, error) {
	key := deviceUserID + "|" + req.ClientReportID
	if reportID, ok := f.byClient[key]; ok {
		record := f.records[reportID]
		if record.BundleHash != req.BundleHash {
			return settingsReportProof{}, errors.New("client_report_id_hash_mismatch")
		}
		return settingsReportProof{
			Accepted:       true,
			ReportID:       record.ReportID,
			Status:         record.Status,
			ReceivedAt:     record.ReceivedAt.Format(time.RFC3339),
			ClientReportID: record.ClientReportID,
			BundleHash:     record.BundleHash,
			Duplicate:      true,
		}, nil
	}

	reportID := "sr_" + strings.ReplaceAll(req.ClientReportID, "-", "_")
	record := settingsReportRecord{
		ReportID:            reportID,
		DeviceUserID:        deviceUserID,
		ClientReportID:      req.ClientReportID,
		BundleSchemaVersion: req.BundleSchemaVersion,
		BundleType:          req.BundleType,
		BundleHash:          req.BundleHash,
		BundleSizeBytes:     len(bundleJSON),
		AppVersion:          req.AppVersion,
		RuntimeID:           req.RuntimeID,
		SidecarVersion:      req.SidecarVersion,
		ReportReason:        req.ReportReason,
		UserNote:            req.UserNote,
		BundleJSON:          append([]byte(nil), bundleJSON...),
		Status:              "new",
		ReceivedAt:          f.now,
		UpdatedAt:           f.now,
	}
	f.records[reportID] = record
	f.byClient[key] = reportID
	return settingsReportProof{
		Accepted:       true,
		ReportID:       reportID,
		Status:         "new",
		ReceivedAt:     f.now.Format(time.RFC3339),
		ClientReportID: req.ClientReportID,
		BundleHash:     req.BundleHash,
		Duplicate:      false,
	}, nil
}

func (f *fakeSettingsReportRepo) ListSettingsReports(_ context.Context, statusFilter string) (settingsReportListResult, error) {
	counts := canonicalSettingsReportStatusCounts()
	var records []settingsReportRecord
	for _, record := range f.records {
		if _, ok := counts[record.Status]; ok {
			counts[record.Status]++
		}
		if statusFilter == "" || statusFilter == "all" || record.Status == statusFilter {
			records = append(records, record)
		}
	}
	return settingsReportListResult{Records: records, Counts: counts}, nil
}

func (f *fakeSettingsReportRepo) GetSettingsReport(_ context.Context, reportID string) (settingsReportRecord, error) {
	record, ok := f.records[reportID]
	if !ok {
		return settingsReportRecord{}, errSettingsReportNotFound
	}
	return record, nil
}

func (f *fakeSettingsReportRepo) UpdateSettingsReportStatus(_ context.Context, reportID, status, reviewNote, reviewedBy string, now time.Time) (settingsReportRecord, error) {
	record, ok := f.records[reportID]
	if !ok {
		return settingsReportRecord{}, errSettingsReportNotFound
	}
	record.Status = status
	record.ReviewNote = reviewNote
	record.ReviewedBy = reviewedBy
	record.ReviewedAt = &now
	record.UpdatedAt = now
	f.records[reportID] = record
	return record, nil
}

func newTestApp(repo *fakeSettingsReportRepo) *App {
	return &App{
		cfg:          config.Config{AdminPassword: "admin-password"},
		sessions:     &sessions.Service{JWTSecret: []byte("test-secret")},
		settingsRepo: repo,
		started:      time.Now().UTC(),
	}
}

func testBearer(t *testing.T) string {
	t.Helper()
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"sub": "du_test",
		"exp": time.Now().UTC().Add(time.Hour).Unix(),
	})
	raw, err := token.SignedString([]byte("test-secret"))
	if err != nil {
		t.Fatal(err)
	}
	return "Bearer " + raw
}

func validSettingsReportPayload(clientID, hash string) string {
	payload := map[string]any{
		"client_report_id":      clientID,
		"created_at":            "2026-05-20T12:00:00Z",
		"bundle_schema_version": 1,
		"bundle_type":           "settings_report",
		"bundle_hash":           hash,
		"app_version":           "1.2.3",
		"runtime_id":            "runtime-1",
		"sidecar_version":       "sidecar-1",
		"report_reason":         "settings_issue",
		"user_note":             "please review",
		"bundle": map[string]any{
			"schema_version": 1,
			"report_source":  "settings_button",
			"source_availability": map[string]any{
				"today_conversation":    "included",
				"recent_runtime_events": "included",
				"runtime_state_summary": "included",
				"app_runtime_logs":      "not_collected_in_this_build",
				"task_diagnostics":      "included",
			},
			"redaction_summary": map[string]any{
				"version": "phase15_app_local_redactor_v1",
			},
			"today_conversation": map[string]any{
				"availability":  "included",
				"message_count": 1,
				"messages": []any{
					map[string]any{"role": "user", "text": "hello", "created_at": "2026-05-20T12:00:00Z"},
				},
			},
			"recent_runtime_events": map[string]any{
				"availability": "included",
				"event_count":  1,
				"events": []any{
					map[string]any{"type": "settings_report", "summary": "report opened", "timestamp": "2026-05-20T12:00:00Z", "source": "app"},
				},
			},
			"runtime_state_summary": map[string]any{
				"availability": "included",
				"runtime_id":   "runtime-1",
			},
			"app_runtime_metadata": map[string]any{
				"app_version": "1.2.3",
				"platform":    "android",
			},
		},
	}
	raw, _ := json.Marshal(payload)
	return string(raw)
}

func authedSubmit(t *testing.T, app *App, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/report/settings", strings.NewReader(body))
	req.Header.Set("Authorization", testBearer(t))
	rr := httptest.NewRecorder()
	app.authRequired(app.handleSettingsReport)(rr, req)
	return rr
}

func authedAdmin(t *testing.T, app *App, handler http.HandlerFunc, method, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	req.Header.Set("X-Admin-Password", "admin-password")
	rr := httptest.NewRecorder()
	app.adminRequired(handler)(rr, req)
	return rr
}

func TestSettingsReportSubmitValidationAndDuplicates(t *testing.T) {
	app := newTestApp(newFakeSettingsReportRepo())

	req := httptest.NewRequest(http.MethodPost, "/report/settings", strings.NewReader(validSettingsReportPayload("client-1", "hash-1")))
	rr := httptest.NewRecorder()
	app.authRequired(app.handleSettingsReport)(rr, req)
	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("submit without auth = %d, want 401", rr.Code)
	}

	rr = authedSubmit(t, app, validSettingsReportPayload("client-1", "hash-1"))
	if rr.Code != http.StatusOK {
		t.Fatalf("valid submit = %d: %s", rr.Code, rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), `"status":"new"`) || !strings.Contains(rr.Body.String(), `"accepted":true`) {
		t.Fatalf("valid submit proof missing status new/accepted: %s", rr.Body.String())
	}

	rr = authedSubmit(t, app, validSettingsReportPayload("client-1", "hash-1"))
	if rr.Code != http.StatusOK || !strings.Contains(rr.Body.String(), `"duplicate":true`) {
		t.Fatalf("duplicate submit mismatch = %d: %s", rr.Code, rr.Body.String())
	}

	rr = authedSubmit(t, app, validSettingsReportPayload("client-1", "hash-2"))
	if rr.Code != http.StatusConflict {
		t.Fatalf("changed duplicate hash = %d, want 409: %s", rr.Code, rr.Body.String())
	}
}

func TestSettingsReportSubmitRejectsMalformedUnsafeAndInvalidPayloads(t *testing.T) {
	app := newTestApp(newFakeSettingsReportRepo())
	cases := []struct {
		name string
		body string
		code int
	}{
		{name: "malformed JSON", body: `{"client_report_id":`, code: http.StatusBadRequest},
		{name: "unknown top field", body: `{"client_report_id":"c","bundle_schema_version":1,"bundle":{"summary":{}},"extra":true}`, code: http.StatusBadRequest},
		{name: "missing client id", body: `{"bundle_schema_version":1,"bundle":{"summary":{}}}`, code: http.StatusBadRequest},
		{name: "invalid version", body: `{"client_report_id":"c","bundle_schema_version":2,"bundle":{"summary":{}}}`, code: http.StatusBadRequest},
		{name: "invalid type", body: `{"client_report_id":"c","bundle_schema_version":1,"bundle_type":"other","bundle":{"summary":{}}}`, code: http.StatusBadRequest},
		{name: "blocked sensitive key", body: `{"client_report_id":"c","bundle_schema_version":1,"bundle":{"api_key":"xai-secret-value"}}`, code: http.StatusBadRequest},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rr := authedSubmit(t, app, tc.body)
			if rr.Code != tc.code {
				t.Fatalf("%s = %d, want %d: %s", tc.name, rr.Code, tc.code, rr.Body.String())
			}
		})
	}

	oversized := `{"client_report_id":"` + strings.Repeat("x", int(maxSettingsReportBodyBytes)+1) + `"}`
	rr := authedSubmit(t, app, oversized)
	if rr.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("oversized = %d, want 413: %s", rr.Code, rr.Body.String())
	}
}

func TestSettingsReportStatusAllowlist(t *testing.T) {
	for _, status := range []string{"new", "triaged", "resolved", "dismissed", "needs_followup"} {
		if !isAllowedSettingsReportStatus(status) {
			t.Fatalf("canonical status rejected: %s", status)
		}
	}
	for _, status := range []string{"confirmed", "rule_update_needed", "closed", ""} {
		if isAllowedSettingsReportStatus(status) {
			t.Fatalf("non-canonical status accepted: %s", status)
		}
	}
}

func TestAdminEndpointsRequireAdminPassword(t *testing.T) {
	app := newTestApp(newFakeSettingsReportRepo())
	cases := []struct {
		name    string
		handler http.HandlerFunc
		method  string
		path    string
		body    string
	}{
		{"stats", app.handleAdminStats, http.MethodGet, "/admin/stats", ""},
		{"invite_create", app.handleAdminCreateInvites, http.MethodPost, "/admin/invite/create", `{"count":1,"days":30}`},
		{"invites", app.handleAdminListInvites, http.MethodGet, "/admin/invites", ""},
		{"errors", app.handleAdminErrors, http.MethodGet, "/admin/errors", ""},
		{"ai_reports", app.handleAdminAIReports, http.MethodGet, "/admin/ai-reports", ""},
		{"ai_report_status", app.handleAdminAIReportStatus, http.MethodPost, "/admin/ai-reports/status", `{"id":1,"status":"confirmed"}`},
		{"settings_list", app.handleAdminSettingsReports, http.MethodGet, "/admin/settings-reports", ""},
		{"settings_detail", app.handleAdminSettingsReportDetail, http.MethodGet, "/admin/settings-reports/detail?report_id=sr_test", ""},
		{"settings_status", app.handleAdminSettingsReportStatus, http.MethodPost, "/admin/settings-reports/status", `{}`},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			req := httptest.NewRequest(c.method, c.path, strings.NewReader(c.body))
			rr := httptest.NewRecorder()
			app.adminRequired(c.handler)(rr, req)
			if rr.Code != http.StatusUnauthorized {
				t.Fatalf("%s without admin password = %d, want 401", c.name, rr.Code)
			}
			if strings.Contains(rr.Body.String(), "password") || !strings.Contains(rr.Body.String(), "admin access denied") {
				t.Fatalf("%s auth response is not generic: %s", c.name, rr.Body.String())
			}
		})
	}

	app.cfg.AdminPassword = ""
	req := httptest.NewRequest(http.MethodGet, "/admin/settings-reports", nil)
	rr := httptest.NewRecorder()
	app.adminRequired(app.handleAdminSettingsReports)(rr, req)
	if rr.Code != http.StatusForbidden {
		t.Fatalf("admin API with missing configured password = %d, want 403", rr.Code)
	}
	if strings.Contains(rr.Body.String(), "RELAY_ADMIN_PASSWORD") || strings.Contains(rr.Body.String(), "configured") || !strings.Contains(rr.Body.String(), "admin access denied") {
		t.Fatalf("missing-admin-config response leaks configuration state: %s", rr.Body.String())
	}
}

func TestAdminEndpointMethodRestrictionsAndDisabledLegacyAIReports(t *testing.T) {
	app := newTestApp(newFakeSettingsReportRepo())
	methodCases := []struct {
		name    string
		handler http.HandlerFunc
		method  string
		path    string
	}{
		{"stats_post", app.handleAdminStats, http.MethodPost, "/admin/stats"},
		{"invites_post", app.handleAdminListInvites, http.MethodPost, "/admin/invites"},
		{"errors_post", app.handleAdminErrors, http.MethodPost, "/admin/errors"},
		{"invite_create_get", app.handleAdminCreateInvites, http.MethodGet, "/admin/invite/create"},
		{"settings_list_post", app.handleAdminSettingsReports, http.MethodPost, "/admin/settings-reports"},
		{"settings_detail_post", app.handleAdminSettingsReportDetail, http.MethodPost, "/admin/settings-reports/detail"},
		{"settings_status_get", app.handleAdminSettingsReportStatus, http.MethodGet, "/admin/settings-reports/status"},
	}
	for _, tc := range methodCases {
		t.Run(tc.name, func(t *testing.T) {
			rr := authedAdmin(t, app, tc.handler, tc.method, tc.path, "")
			if rr.Code != http.StatusMethodNotAllowed {
				t.Fatalf("%s = %d, want 405: %s", tc.name, rr.Code, rr.Body.String())
			}
		})
	}

	for _, tc := range []struct {
		name    string
		handler http.HandlerFunc
		method  string
		path    string
		body    string
	}{
		{"ai_reports_get", app.handleAdminAIReports, http.MethodGet, "/admin/ai-reports", ""},
		{"ai_reports_post", app.handleAdminAIReports, http.MethodPost, "/admin/ai-reports", `{"prompt_text":"raw prompt","output_text":"raw output"}`},
		{"ai_status_post", app.handleAdminAIReportStatus, http.MethodPost, "/admin/ai-reports/status", `{"id":1,"status":"confirmed","review_note":"raw output_text"}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rr := authedAdmin(t, app, tc.handler, tc.method, tc.path, tc.body)
			if rr.Code != http.StatusNotFound {
				t.Fatalf("%s = %d, want 404: %s", tc.name, rr.Code, rr.Body.String())
			}
			for _, forbidden := range []string{"prompt_text", "output_text", "confirmed", "raw prompt", "raw output"} {
				if strings.Contains(rr.Body.String(), forbidden) {
					t.Fatalf("%s disabled response leaked %q: %s", tc.name, forbidden, rr.Body.String())
				}
			}
		})
	}
}

func TestAdminStatsProjectionAllowsAggregatesOnly(t *testing.T) {
	projected := projectAdminStats(map[string]any{
		"total_device_users":    int64(12),
		"verified_emails":       int64(4),
		"active_trials":         int64(3),
		"pending_trials":        int64(2),
		"active_subscriptions":  int64(1),
		"active_invites":        int64(5),
		"unused_invite_codes":   int64(6),
		"total_llm_turns":       int64(7),
		"access_mode":           "private",
		"access_code_only":      true,
		"trial_allowed":         true,
		"subscription_allowed":  true,
		"access_code_days":      30,
		"postgres_dsn":          "postgres://secret",
		"RELAY_ADMIN_PASSWORD":  "secret",
		"provider_config_token": "secret",
	}, 9)
	body, err := json.Marshal(projected)
	if err != nil {
		t.Fatal(err)
	}
	for _, expected := range []string{"total_device_users", "verified_emails", "active_invites", "relay_uptime_sec"} {
		if !strings.Contains(string(body), expected) {
			t.Fatalf("stats projection missing %s: %s", expected, string(body))
		}
	}
	for _, forbidden := range []string{"access_mode", "access_code_only", "trial_allowed", "subscription_allowed", "access_code_days", "postgres_dsn", "RELAY_ADMIN_PASSWORD", "provider_config_token"} {
		if strings.Contains(string(body), forbidden) {
			t.Fatalf("stats projection leaked %s: %s", forbidden, string(body))
		}
	}
}

func TestAdminInviteCreateValidation(t *testing.T) {
	app := newTestApp(newFakeSettingsReportRepo())
	cases := []struct {
		name string
		body string
	}{
		{"unknown_field", `{"count":1,"days":30,"code":"SHOULDNOT"}`},
		{"count_zero", `{"count":0,"days":30}`},
		{"count_too_high", `{"count":501,"days":30}`},
		{"days_zero", `{"count":1,"days":0}`},
		{"days_too_high", `{"count":1,"days":366}`},
		{"oversized", `{"count":1,"days":30,"extra":"` + strings.Repeat("x", int(maxAdminInviteCreateBodyBytes)+1) + `"}`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rr := authedAdmin(t, app, app.handleAdminCreateInvites, http.MethodPost, "/admin/invite/create", tc.body)
			if rr.Code != http.StatusBadRequest {
				t.Fatalf("%s = %d, want 400: %s", tc.name, rr.Code, rr.Body.String())
			}
			if strings.Contains(rr.Body.String(), "SHOULDNOT") || strings.Contains(rr.Body.String(), "admin-password") {
				t.Fatalf("%s response leaked unsafe input/config: %s", tc.name, rr.Body.String())
			}
		})
	}
}

func TestAdminInviteAndErrorProjectionsSuppressRawSensitiveFields(t *testing.T) {
	inviteBody, err := json.Marshal(projectAdminInviteCodes([]map[string]any{{
		"code":         "ABCDEFGH",
		"granted_days": 30,
		"created_at":   "2026-05-20T12:00:00Z",
		"created_by":   "admin",
		"used":         true,
		"used_by":      "du_raw_device_user_id",
		"used_at":      "2026-05-20T13:00:00Z",
	}}))
	if err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{"ABCDEFGH", "du_raw_device_user_id", `"code"`, `"used_by"`} {
		if bytes.Contains(inviteBody, []byte(forbidden)) {
			t.Fatalf("invite projection leaked %q: %s", forbidden, string(inviteBody))
		}
	}
	for _, expected := range []string{"code_masked", "AB****GH", "used_by_ref"} {
		if !bytes.Contains(inviteBody, []byte(expected)) {
			t.Fatalf("invite projection missing %q: %s", expected, string(inviteBody))
		}
	}

	errorBody, err := json.Marshal(projectAdminErrorReports([]map[string]any{{
		"id":             int64(3),
		"device_user_id": "du_raw_error_device",
		"error_type":     "network",
		"message":        "authorization: bearer abcdefghijklmnop token=supersecret",
		"context_data":   map[string]any{"api_key": "xai-should-not-display"},
		"created_at":     "2026-05-20T12:00:00Z",
	}}))
	if err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{"du_raw_error_device", "abcdefghijklmnop", "supersecret", "context_data", "api_key", "xai-should-not-display"} {
		if bytes.Contains(errorBody, []byte(forbidden)) {
			t.Fatalf("error projection leaked %q: %s", forbidden, string(errorBody))
		}
	}
	for _, expected := range []string{"device_ref", "[suppressed]", "[redacted]"} {
		if !bytes.Contains(errorBody, []byte(expected)) {
			t.Fatalf("error projection missing %q: %s", expected, string(errorBody))
		}
	}
}

func TestAdminSettingsReportSafeProjectionAndReviewWorkflow(t *testing.T) {
	repo := newFakeSettingsReportRepo()
	app := newTestApp(repo)
	rr := authedSubmit(t, app, validSettingsReportPayload("client-2", "hash-safe"))
	if rr.Code != http.StatusOK {
		t.Fatalf("seed submit = %d: %s", rr.Code, rr.Body.String())
	}

	record := repo.records["sr_client_2"]
	record.UserNote = "contains sk-12345678901234567890 and token=supersecret"
	record.BundleJSON = json.RawMessage(`{
		"report_source":"settings_button",
		"source_availability":{"today_conversation":"included"},
		"redaction_summary":{"version":"phase15","api_key":"xai-should-not-display"},
		"today_conversation":{"availability":"included","message_count":1,"messages":[{"role":"user","text":"bearer abcdefghijklmnop","created_at":"2026-05-20T12:00:00Z"}]},
		"recent_runtime_events":{"availability":"included","event_count":1,"events":[{"type":"report","summary":"authorization: bearer abcdefghijklmnop","timestamp":"2026-05-20T12:00:00Z","source":"app"}]},
		"runtime_state_summary":{"runtime_id":"runtime-1"},
		"app_runtime_metadata":{"platform":"android"}
	}`)
	repo.records[record.ReportID] = record

	rr = authedAdmin(t, app, app.handleAdminSettingsReports, http.MethodGet, "/admin/settings-reports?status=new", "")
	if rr.Code != http.StatusOK {
		t.Fatalf("admin list = %d: %s", rr.Code, rr.Body.String())
	}
	assertSafeAdminBody(t, rr.Body.Bytes())
	if !strings.Contains(rr.Body.String(), `"counts"`) || !strings.Contains(rr.Body.String(), `"source_availability"`) {
		t.Fatalf("admin list missing counts/source availability: %s", rr.Body.String())
	}

	rr = authedAdmin(t, app, app.handleAdminSettingsReportDetail, http.MethodGet, "/admin/settings-reports/detail?report_id=sr_client_2", "")
	if rr.Code != http.StatusOK {
		t.Fatalf("admin detail = %d: %s", rr.Code, rr.Body.String())
	}
	assertSafeAdminBody(t, rr.Body.Bytes())
	for _, expected := range []string{"conversation_excerpt", "runtime_event_summary", "redaction_summary", "Unavailable in this report."} {
		if !strings.Contains(rr.Body.String(), expected) {
			t.Fatalf("admin detail missing %s: %s", expected, rr.Body.String())
		}
	}

	rr = authedAdmin(t, app, app.handleAdminSettingsReportStatus, http.MethodPost, "/admin/settings-reports/status", `{"report_id":"sr_client_2","status":"confirmed","review_note":"old","reviewed_by":"lane"}`)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("stale status update = %d, want 400: %s", rr.Code, rr.Body.String())
	}

	rr = authedAdmin(t, app, app.handleAdminSettingsReportStatus, http.MethodPost, "/admin/settings-reports/status", `{"report_id":"sr_client_2","status":"needs_followup","review_note":"follow up without token=supersecret","reviewed_by":"lane5"}`)
	if rr.Code != http.StatusOK {
		t.Fatalf("status update = %d: %s", rr.Code, rr.Body.String())
	}
	assertSafeAdminBody(t, rr.Body.Bytes())
	if !strings.Contains(rr.Body.String(), `"status":"needs_followup"`) || !strings.Contains(rr.Body.String(), `"reviewed_by":"lane5"`) {
		t.Fatalf("status update response missing review metadata: %s", rr.Body.String())
	}

	rr = authedAdmin(t, app, app.handleAdminSettingsReportStatus, http.MethodPost, "/admin/settings-reports/status", `{"report_id":"sr_client_2","status":"triaged","review_note":"`+strings.Repeat("x", maxSettingsReportReviewNoteBytes+1)+`","reviewed_by":"lane5"}`)
	if rr.Code != http.StatusBadRequest || !strings.Contains(rr.Body.String(), "review_note is too long") {
		t.Fatalf("overlong review_note = %d, want 400: %s", rr.Code, rr.Body.String())
	}

	rr = authedAdmin(t, app, app.handleAdminSettingsReportStatus, http.MethodPost, "/admin/settings-reports/status", `{"report_id":"sr_client_2","status":"triaged","review_note":"ok","reviewed_by":"`+strings.Repeat("x", maxSettingsReportReviewedByBytes+1)+`"}`)
	if rr.Code != http.StatusBadRequest || !strings.Contains(rr.Body.String(), "reviewed_by is too long") {
		t.Fatalf("overlong reviewed_by = %d, want 400: %s", rr.Code, rr.Body.String())
	}

	rr = authedAdmin(t, app, app.handleAdminSettingsReportStatus, http.MethodPost, "/admin/settings-reports/status", `{"report_id":"sr_client_2","status":"triaged","review_note":"`+strings.Repeat("x", int(maxSettingsReportStatusBodyBytes)+1)+`","reviewed_by":"lane5"}`)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("oversized status body = %d, want 400: %s", rr.Code, rr.Body.String())
	}
}

func assertSafeAdminBody(t *testing.T, body []byte) {
	t.Helper()
	lower := strings.ToLower(string(body))
	for _, forbidden := range []string{
		"bundle_json",
		"xai-should-not-display",
		"sk-12345678901234567890",
		"abcdefghijklmnop",
		"supersecret",
		"api_key",
	} {
		if bytes.Contains([]byte(lower), []byte(strings.ToLower(forbidden))) {
			t.Fatalf("admin body leaked %q: %s", forbidden, string(body))
		}
	}
}

func TestAdminPageDoesNotServePrivateConsoleUI(t *testing.T) {
	app := newTestApp(newFakeSettingsReportRepo())
	req := httptest.NewRequest(http.MethodGet, "/admin", nil)
	rr := httptest.NewRecorder()
	app.handleAdminPage(rr, req)
	if rr.Code != http.StatusNotFound {
		t.Fatalf("/admin = %d, want 404", rr.Code)
	}
	if contentType := rr.Header().Get("Content-Type"); !strings.Contains(contentType, "application/json") {
		t.Fatalf("/admin content type = %q, want JSON", contentType)
	}
	body := rr.Body.String()
	for _, forbidden := range []string{
		"xMilo Admin",
		"Protected relay operations console",
		"Settings Reports",
		"Status filter",
		"Status summary",
		"Review decision",
		"review_note",
		"reviewed_by",
		"Copy report id",
		"Source availability",
		"Redaction summary",
		"System Health / Stats",
		"Access Codes",
		"Error Reports",
		"AI Content Reports",
		"bundle_json",
		"X-Admin-Password",
		"admin password",
		"<html",
		"<script",
	} {
		if strings.Contains(body, forbidden) {
			t.Fatalf("/admin response contains private console content %q: %s", forbidden, body)
		}
	}
}
