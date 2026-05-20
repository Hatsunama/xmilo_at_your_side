package httpx

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"xmilo/relay-go/internal/config"
	"xmilo/relay-go/internal/db"
	"xmilo/relay-go/internal/email"
	"xmilo/relay-go/internal/entitlements"
	xaiclient "xmilo/relay-go/internal/openai"
	"xmilo/relay-go/internal/sessions"
	"xmilo/relay-go/internal/turns"
	"xmilo/relay-go/shared/contracts"
)

type App struct {
	cfg          config.Config
	store        *db.Store
	sessions     *sessions.Service
	turns        *turns.Service
	entitlements *entitlements.Service
	settingsRepo settingsReportRepository
	emailer      *email.Sender
	started      time.Time
}

func NewApp(ctx context.Context, cfg config.Config) (*App, error) {
	store, err := db.Open(ctx, cfg.PostgresDSN)
	if err != nil {
		return nil, err
	}

	entSvc := &entitlements.Service{
		Store:      store,
		TrialHours: cfg.TrialHours,
		InviteDays: cfg.InviteBetaDays,
	}

	sessionSvc := &sessions.Service{
		Store:                store,
		JWTSecret:            []byte(cfg.JWTSecret),
		DevEntitled:          cfg.DevEntitled,
		Entitlements:         entSvc,
		AccessMode:           cfg.AccessMode,
		InviteDays:           cfg.InviteBetaDays,
		HandoffSecret:        []byte(cfg.WebsiteHandoffSecret),
		HandoffAudience:      cfg.WebsiteHandoffAudience,
		HandoffMaxAgeSeconds: cfg.WebsiteHandoffMaxAgeSeconds,
	}

	turnSvc := &turns.Service{
		Store: store,
		LLM:   xaiclient.New(cfg.XAIAPIKey, cfg.XAIModel, cfg.XAIBaseURL),
	}

	emailer := &email.Sender{
		From:     cfg.SMTPFrom,
		Host:     cfg.SMTPHost,
		Port:     cfg.SMTPPort,
		Username: cfg.SMTPUsername,
		Password: cfg.SMTPPassword,
		UseTLS:   cfg.SMTPUseTLS,
	}

	return &App{
		cfg:          cfg,
		store:        store,
		sessions:     sessionSvc,
		turns:        turnSvc,
		entitlements: entSvc,
		settingsRepo: &postgresSettingsReportStore{store: store},
		emailer:      emailer,
		started:      time.Now().UTC(),
	}, nil
}

func (a *App) Start(ctx context.Context) error {
	mux := http.NewServeMux()

	// Core
	mux.HandleFunc("/health", a.handleHealth)

	// Session bootstrap (no auth — first call from fresh install)
	mux.HandleFunc("/session/start", a.handleSessionStart)

	// Auth flows (no auth — device_user_id in body is sufficient identity)
	mux.HandleFunc("/auth/register", a.handleAuthRegister)
	mux.HandleFunc("/auth/verify", a.handleAuthVerify)
	mux.HandleFunc("/auth/invite", a.handleAuthInvite)

	// Auth refresh (requires valid JWT — re-computes entitled from DB)
	mux.HandleFunc("/auth/refresh", a.authRequired(a.handleAuthRefresh))
	mux.HandleFunc("/auth/2fa/status", a.authRequired(a.handleTwoFactorStatus))
	mux.HandleFunc("/auth/2fa/setup/begin", a.authRequired(a.handleTwoFactorBegin))
	mux.HandleFunc("/auth/2fa/setup/confirm", a.authRequired(a.handleTwoFactorConfirm))
	mux.HandleFunc("/auth/2fa/verify", a.authRequired(a.handleTwoFactorVerify))
	mux.HandleFunc("/auth/2fa/recovery/regenerate", a.authRequired(a.handleTwoFactorRecoveryRegenerate))
	mux.HandleFunc("/auth/2fa/disable", a.authRequired(a.handleTwoFactorDisable))
	mux.HandleFunc("/auth/website/handoff/create", a.authRequired(a.handleWebsiteHandoffCreate))
	mux.HandleFunc("/auth/account/delete", a.authRequired(a.handleAccountDelete))

	// Main LLM endpoint (requires entitled JWT)
	mux.HandleFunc("/llm/turn", a.authRequired(a.handleTurn))

	// Error reporting (auth optional — best-effort)
	mux.HandleFunc("/error/report", a.handleErrorReport)
	mux.HandleFunc("/report/ai", a.authRequired(a.handleAIContentReport))
	mux.HandleFunc("/report/settings", a.authRequired(a.handleSettingsReport))

	// RevenueCat billing webhook
	mux.HandleFunc("/billing/revenuecat", a.handleRevenueCat)

	// Admin JSON APIs require X-Admin-Password; /admin does not serve a console in the public relay.
	mux.HandleFunc("/admin", a.handleAdminPage)
	mux.HandleFunc("/admin/stats", a.adminRequired(a.handleAdminStats))
	mux.HandleFunc("/admin/invite/create", a.adminRequired(a.handleAdminCreateInvites))
	mux.HandleFunc("/admin/invites", a.adminRequired(a.handleAdminListInvites))
	mux.HandleFunc("/admin/errors", a.adminRequired(a.handleAdminErrors))
	mux.HandleFunc("/admin/ai-reports", a.adminRequired(a.handleAdminAIReports))
	mux.HandleFunc("/admin/ai-reports/status", a.adminRequired(a.handleAdminAIReportStatus))
	mux.HandleFunc("/admin/settings-reports", a.adminRequired(a.handleAdminSettingsReports))
	mux.HandleFunc("/admin/settings-reports/detail", a.adminRequired(a.handleAdminSettingsReportDetail))
	mux.HandleFunc("/admin/settings-reports/status", a.adminRequired(a.handleAdminSettingsReportStatus))

	server := &http.Server{Addr: a.cfg.HTTPAddr, Handler: mux}
	go func() {
		<-ctx.Done()
		_ = server.Shutdown(context.Background())
		a.store.Close()
	}()
	return server.ListenAndServe()
}

// ─── Health ───────────────────────────────────────────────────────────────────

func (a *App) handleHealth(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"ok":         true,
		"service":    "milo-relay",
		"uptime_sec": int64(time.Since(a.started).Seconds()),
	})
}

// ─── Session bootstrap ────────────────────────────────────────────────────────

func (a *App) handleSessionStart(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "method not allowed"})
		return
	}
	var req contracts.SessionStartRequest
	if err := decodeJSON(r, &req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": err.Error()})
		return
	}
	if req.DeviceName == "" {
		req.DeviceName = "unknown-android-device"
	}
	resp, err := a.sessions.Start(r.Context(), req)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, resp)
}

// ─── Auth: register / verify / invite ─────────────────────────────────────────

// POST /auth/register  {"email":"...", "device_user_id":"du_..."}
// Sends a verification email. No-op if the same email+device already has a pending token.
func (a *App) handleAuthRegister(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "method not allowed"})
		return
	}
	var req struct {
		Email        string `json:"email"`
		DeviceUserID string `json:"device_user_id"`
	}
	if err := decodeJSON(r, &req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": err.Error()})
		return
	}
	if req.Email == "" || req.DeviceUserID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "email and device_user_id required"})
		return
	}
	// Normalise
	req.Email = strings.ToLower(strings.TrimSpace(req.Email))

	token := uuid.NewString()
	expiresAt := time.Now().UTC().Add(24 * time.Hour)

	_, err := a.store.Pool.Exec(r.Context(), `
		INSERT INTO email_verifications (token, email, device_user_id, expires_at)
		VALUES ($1, $2, $3, $4)
	`, token, req.Email, req.DeviceUserID, expiresAt)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": "could not create verification"})
		return
	}

	verifyURL := a.cfg.PublicBaseURL + "/auth/verify?token=" + token
	if err := a.emailer.SendVerification(req.Email, verifyURL); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": "could not send email"})
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "message": "verification email sent"})
}

// GET /auth/verify?token=...
// Verifies email, grants trial. Returns HTML (user taps link from email client).
func (a *App) handleAuthVerify(w http.ResponseWriter, r *http.Request) {
	token := r.URL.Query().Get("token")
	if token == "" {
		http.Error(w, "missing token", http.StatusBadRequest)
		return
	}

	var ev struct {
		Email        string
		DeviceUserID string
		ExpiresAt    time.Time
		VerifiedAt   *time.Time
	}
	row := a.store.Pool.QueryRow(r.Context(), `
		SELECT email, device_user_id, expires_at, verified_at
		FROM email_verifications WHERE token = $1
	`, token)
	if err := row.Scan(&ev.Email, &ev.DeviceUserID, &ev.ExpiresAt, &ev.VerifiedAt); err != nil {
		serveVerifyHTML(w, false, "Link not found or already used.")
		return
	}
	if ev.VerifiedAt != nil {
		serveVerifyHTML(w, true, "Already verified. Return to xMilo.")
		return
	}
	if time.Now().UTC().After(ev.ExpiresAt) {
		serveVerifyHTML(w, false, "This link has expired. Request a new one from the app.")
		return
	}

	now := time.Now().UTC()
	_, err := a.store.Pool.Exec(r.Context(),
		`UPDATE email_verifications SET verified_at = $1 WHERE token = $2`, now, token)
	if err != nil {
		serveVerifyHTML(w, false, "Verification failed. Please try again.")
		return
	}

	if a.cfg.TrialAllowed() {
		// Grant trial (pending — clock starts on first /llm/turn call)
		_ = a.entitlements.GrantTrial(r.Context(), ev.DeviceUserID)
		serveVerifyHTML(w, true, "Email verified! Return to the xMilo app to start your free trial.")
		return
	}

	serveVerifyHTML(w, true, "Email verified! Return to the xMilo app and redeem your access code to begin.")
}

// POST /auth/invite  {"code":"XXXXXXXX", "device_user_id":"du_..."}
// Redeems a single-use access code and grants the configured access period.
func (a *App) handleAuthInvite(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "method not allowed"})
		return
	}
	var req struct {
		Code         string `json:"code"`
		DeviceUserID string `json:"device_user_id"`
	}
	if err := decodeJSON(r, &req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": err.Error()})
		return
	}
	req.Code = strings.ToUpper(strings.TrimSpace(req.Code))
	if req.Code == "" || req.DeviceUserID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "code and device_user_id required"})
		return
	}
	identity, err := a.sessions.IdentityState(r.Context(), req.DeviceUserID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}
	if !identity.EmailVerified {
		writeJSON(w, http.StatusForbidden, map[string]any{"error": "verify your email before redeeming an access code"})
		return
	}
	if err := a.entitlements.RedeemInvite(r.Context(), req.Code, req.DeviceUserID); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"ok":                     true,
		"message":                "access code accepted",
		"access_code_days":       a.cfg.InviteBetaDays,
		"access_code_grant_days": a.cfg.InviteBetaDays,
		"access_mode":            a.cfg.AccessMode,
		"access_code_only":       a.cfg.AccessCodeOnly(),
		"trial_allowed":          a.cfg.TrialAllowed(),
		"subscription_allowed":   a.cfg.SubscriptionAllowed(),
	})
}

// ─── Auth refresh ─────────────────────────────────────────────────────────────

func (a *App) handleAuthRefresh(w http.ResponseWriter, r *http.Request, claims map[string]any) {
	deviceUserID, _ := claims["sub"].(string)
	sessionID, _ := claims["session_id"].(string)
	resp, err := a.sessions.Refresh(r.Context(), deviceUserID, sessionID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, resp)
}

func (a *App) handleTwoFactorStatus(w http.ResponseWriter, r *http.Request, claims map[string]any) {
	deviceUserID, _ := claims["sub"].(string)
	identity, err := a.sessions.IdentityState(r.Context(), deviceUserID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"ok":                    true,
		"verified_email":        identity.VerifiedEmail,
		"email_verified":        identity.EmailVerified,
		"two_factor_enabled":    identity.TwoFactorEnabled,
		"two_factor_ok":         identity.TwoFactorOK,
		"website_handoff_ready": identity.EmailVerified && identity.TwoFactorOK,
	})
}

func (a *App) handleTwoFactorBegin(w http.ResponseWriter, r *http.Request, claims map[string]any) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "method not allowed"})
		return
	}
	deviceUserID, _ := claims["sub"].(string)
	secret, otpURL, err := a.sessions.BeginTwoFactorSetup(r.Context(), deviceUserID)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"ok":      true,
		"secret":  secret,
		"otp_url": otpURL,
	})
}

func (a *App) handleTwoFactorConfirm(w http.ResponseWriter, r *http.Request, claims map[string]any) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "method not allowed"})
		return
	}
	var req struct {
		Code string `json:"code"`
	}
	if err := decodeJSON(r, &req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": err.Error()})
		return
	}
	deviceUserID, _ := claims["sub"].(string)
	sessionID, _ := claims["session_id"].(string)
	recoveryCodes, err := a.sessions.ConfirmTwoFactorSetup(r.Context(), deviceUserID, req.Code)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": err.Error()})
		return
	}
	resp, refreshErr := a.sessions.Refresh(r.Context(), deviceUserID, sessionID)
	if refreshErr != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": refreshErr.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"ok":             true,
		"recovery_codes": recoveryCodes,
		"session":        resp,
	})
}

func (a *App) handleTwoFactorVerify(w http.ResponseWriter, r *http.Request, claims map[string]any) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "method not allowed"})
		return
	}
	var req struct {
		Code         string `json:"code"`
		RecoveryCode string `json:"recovery_code"`
	}
	if err := decodeJSON(r, &req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": err.Error()})
		return
	}
	deviceUserID, _ := claims["sub"].(string)
	sessionID, _ := claims["session_id"].(string)
	if err := a.sessions.VerifyTwoFactor(r.Context(), deviceUserID, req.Code, req.RecoveryCode); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": err.Error()})
		return
	}
	resp, refreshErr := a.sessions.Refresh(r.Context(), deviceUserID, sessionID)
	if refreshErr != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": refreshErr.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "session": resp})
}

func (a *App) handleTwoFactorRecoveryRegenerate(w http.ResponseWriter, r *http.Request, claims map[string]any) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "method not allowed"})
		return
	}
	deviceUserID, _ := claims["sub"].(string)
	recoveryCodes, err := a.sessions.RegenerateRecoveryCodes(r.Context(), deviceUserID)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "recovery_codes": recoveryCodes})
}

func (a *App) handleTwoFactorDisable(w http.ResponseWriter, r *http.Request, claims map[string]any) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "method not allowed"})
		return
	}
	var req struct {
		Code         string `json:"code"`
		RecoveryCode string `json:"recovery_code"`
	}
	if err := decodeJSON(r, &req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": err.Error()})
		return
	}
	deviceUserID, _ := claims["sub"].(string)
	sessionID, _ := claims["session_id"].(string)
	if err := a.sessions.DisableTwoFactor(r.Context(), deviceUserID, req.Code, req.RecoveryCode); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": err.Error()})
		return
	}
	resp, refreshErr := a.sessions.Refresh(r.Context(), deviceUserID, sessionID)
	if refreshErr != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": refreshErr.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "session": resp})
}

func (a *App) handleWebsiteHandoffCreate(w http.ResponseWriter, r *http.Request, claims map[string]any) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "method not allowed"})
		return
	}
	token, expiresAt, err := a.sessions.CreateWebsiteHandoff(r.Context(), claims)
	if err != nil {
		writeJSON(w, http.StatusForbidden, map[string]any{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"ok":            true,
		"handoff_token": token,
		"handoff_url":   "https://sol.xmiloatyourside.com/api/session/handoff?token=" + token,
		"expires_at":    expiresAt.Format(time.RFC3339),
	})
}

func (a *App) handleAccountDelete(w http.ResponseWriter, r *http.Request, claims map[string]any) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "method not allowed"})
		return
	}
	deviceUserID, _ := claims["sub"].(string)
	if deviceUserID == "" {
		writeJSON(w, http.StatusUnauthorized, map[string]any{"error": "missing device identity"})
		return
	}
	if err := a.sessions.DeleteAccount(r.Context(), deviceUserID); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// ─── LLM turn ─────────────────────────────────────────────────────────────────

func (a *App) handleTurn(w http.ResponseWriter, r *http.Request, claims map[string]any) {
	entitled, _ := claims["entitled"].(bool)
	if !entitled {
		writeJSON(w, http.StatusForbidden, map[string]any{"error": "entitlement_lost"})
		return
	}

	var req contracts.RelayTurnRequest
	if err := decodeJSON(r, &req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": err.Error()})
		return
	}

	// Start trial clock on first real task (no-op for invite/subscription users)
	deviceUserID, _ := claims["sub"].(string)
	_ = a.entitlements.StartTrialIfNeeded(r.Context(), deviceUserID)

	resp, err := a.turns.Execute(r.Context(), req, deviceUserID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, resp)
}

// ─── Error report ─────────────────────────────────────────────────────────────

// POST /error/report  {"device_user_id":"...", "error_type":"...", "message":"...", "context":{}}
// Best-effort: auth not required so errors can be reported even before/after session.
func (a *App) handleErrorReport(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "method not allowed"})
		return
	}
	var req struct {
		DeviceUserID string         `json:"device_user_id"`
		ErrorType    string         `json:"error_type"`
		Message      string         `json:"message"`
		Context      map[string]any `json:"context"`
	}
	// Don't DisallowUnknownFields for error reports — context payload varies
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": err.Error()})
		return
	}
	defer r.Body.Close()

	ctxData, _ := json.Marshal(req.Context)
	_, _ = a.store.Pool.Exec(r.Context(), `
		INSERT INTO error_reports (device_user_id, error_type, message, context_data, created_at)
		VALUES ($1, $2, $3, $4, $5)
	`, req.DeviceUserID, req.ErrorType, req.Message, ctxData, time.Now().UTC())

	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (a *App) handleAIContentReport(w http.ResponseWriter, r *http.Request, claims map[string]any) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "method not allowed"})
		return
	}
	var req struct {
		TaskID       string `json:"task_id"`
		EventType    string `json:"event_type"`
		ReportReason string `json:"report_reason"`
		ReportNote   string `json:"report_note"`
		OutputText   string `json:"output_text"`
		PromptText   string `json:"prompt_text"`
		RuntimeID    string `json:"runtime_id"`
		ModelName    string `json:"model_name"`
	}
	if err := decodeJSON(r, &req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": err.Error()})
		return
	}
	deviceUserID, _ := claims["sub"].(string)
	if deviceUserID == "" {
		writeJSON(w, http.StatusUnauthorized, map[string]any{"error": "missing device identity"})
		return
	}
	req.EventType = strings.TrimSpace(req.EventType)
	req.ReportReason = strings.TrimSpace(req.ReportReason)
	req.OutputText = strings.TrimSpace(req.OutputText)
	if req.EventType == "" || req.ReportReason == "" || req.OutputText == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "event_type, report_reason, and output_text are required"})
		return
	}
	_, err := a.store.Pool.Exec(r.Context(), `
		INSERT INTO ai_content_reports
			(device_user_id, task_id, event_type, report_reason, report_note, output_text, prompt_text, runtime_id, model_name, status, created_at)
		VALUES ($1, NULLIF($2, ''), $3, $4, $5, $6, $7, $8, $9, 'new', $10)
	`, deviceUserID, req.TaskID, req.EventType, req.ReportReason, req.ReportNote, req.OutputText, req.PromptText, req.RuntimeID, req.ModelName, time.Now().UTC())
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": "could not store ai content report"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

const maxSettingsReportBodyBytes int64 = 1 << 20
const maxSettingsReportStatusBodyBytes int64 = 8 << 10
const maxSettingsReportReviewNoteBytes = 2000
const maxSettingsReportReviewedByBytes = 120
const maxAdminInviteCreateBodyBytes int64 = 4 << 10
const maxAdminInviteCreateCount = 500
const maxAdminInviteCreateDays = 365

type settingsReportRequest struct {
	ClientReportID      string          `json:"client_report_id"`
	CreatedAt           string          `json:"created_at"`
	BundleSchemaVersion int             `json:"bundle_schema_version"`
	BundleType          string          `json:"bundle_type"`
	BundleHash          string          `json:"bundle_hash"`
	AppVersion          string          `json:"app_version"`
	RuntimeID           string          `json:"runtime_id"`
	SidecarVersion      string          `json:"sidecar_version"`
	ReportReason        string          `json:"report_reason"`
	UserNote            string          `json:"user_note"`
	Bundle              json.RawMessage `json:"bundle"`
}

type settingsReportProof struct {
	Accepted       bool   `json:"accepted"`
	ReportID       string `json:"report_id"`
	Status         string `json:"status"`
	ReceivedAt     string `json:"received_at"`
	ClientReportID string `json:"client_report_id"`
	BundleHash     string `json:"bundle_hash"`
	Duplicate      bool   `json:"duplicate"`
}

var canonicalSettingsReportStatuses = []string{
	"new",
	"triaged",
	"resolved",
	"dismissed",
	"needs_followup",
}

var errSettingsReportNotFound = errors.New("settings report not found")

type settingsReportRepository interface {
	StoreSettingsReport(ctx context.Context, deviceUserID string, req settingsReportRequest, bundleJSON []byte) (settingsReportProof, error)
	ListSettingsReports(ctx context.Context, statusFilter string) (settingsReportListResult, error)
	GetSettingsReport(ctx context.Context, reportID string) (settingsReportRecord, error)
	UpdateSettingsReportStatus(ctx context.Context, reportID, status, reviewNote, reviewedBy string, now time.Time) (settingsReportRecord, error)
}

type postgresSettingsReportStore struct {
	store *db.Store
}

type settingsReportListResult struct {
	Records []settingsReportRecord
	Counts  map[string]int
}

type settingsReportRecord struct {
	ReportID            string
	DeviceUserID        string
	ClientReportID      string
	BundleSchemaVersion int
	BundleType          string
	BundleHash          string
	BundleSizeBytes     int
	AppVersion          string
	RuntimeID           string
	SidecarVersion      string
	ReportReason        string
	UserNote            string
	BundleJSON          json.RawMessage
	Status              string
	ReviewNote          string
	ReviewedBy          string
	ReceivedAt          time.Time
	UpdatedAt           time.Time
	ReviewedAt          *time.Time
}

type redactionTracker struct {
	Redacted         bool
	SuppressedFields int
}

func (a *App) settingsReportRepository() settingsReportRepository {
	if a.settingsRepo != nil {
		return a.settingsRepo
	}
	return &postgresSettingsReportStore{store: a.store}
}

func (a *App) handleSettingsReport(w http.ResponseWriter, r *http.Request, claims map[string]any) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "method not allowed"})
		return
	}

	deviceUserID, _ := claims["sub"].(string)
	if deviceUserID == "" {
		writeJSON(w, http.StatusUnauthorized, map[string]any{"error": "missing device identity"})
		return
	}

	var req settingsReportRequest
	r.Body = http.MaxBytesReader(w, r.Body, maxSettingsReportBodyBytes)
	defer r.Body.Close()
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&req); err != nil {
		var maxErr *http.MaxBytesError
		if errors.As(err, &maxErr) {
			writeJSON(w, http.StatusRequestEntityTooLarge, map[string]any{"error": "settings report exceeds 1 MiB limit"})
			return
		}
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": err.Error()})
		return
	}

	req.ClientReportID = strings.TrimSpace(req.ClientReportID)
	req.BundleType = strings.TrimSpace(req.BundleType)
	if req.BundleType == "" {
		req.BundleType = "settings_report"
	}
	req.BundleHash = strings.TrimSpace(req.BundleHash)
	req.AppVersion = strings.TrimSpace(req.AppVersion)
	req.RuntimeID = strings.TrimSpace(req.RuntimeID)
	req.SidecarVersion = strings.TrimSpace(req.SidecarVersion)
	req.ReportReason = strings.TrimSpace(req.ReportReason)
	req.UserNote = strings.TrimSpace(req.UserNote)

	if req.ClientReportID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "client_report_id is required"})
		return
	}
	if req.BundleSchemaVersion != 1 {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "bundle_schema_version must be 1"})
		return
	}
	if req.BundleType != "settings_report" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "unsupported bundle_type"})
		return
	}
	if req.CreatedAt != "" {
		if _, err := time.Parse(time.RFC3339, req.CreatedAt); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]any{"error": "created_at must be RFC3339"})
			return
		}
	}

	bundleJSON := bytes.TrimSpace(req.Bundle)
	if len(bundleJSON) == 0 || !bytes.HasPrefix(bundleJSON, []byte("{")) {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "bundle must be a JSON object"})
		return
	}
	var bundle map[string]any
	if err := json.Unmarshal(bundleJSON, &bundle); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "bundle must be valid JSON"})
		return
	}
	if hasBlockedSettingsReportField(bundle) {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "bundle contains blocked sensitive field"})
		return
	}

	proof, err := a.settingsReportRepository().StoreSettingsReport(r.Context(), deviceUserID, req, bundleJSON)
	if err != nil {
		if err.Error() == "client_report_id_hash_mismatch" {
			writeJSON(w, http.StatusConflict, map[string]any{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": "could not store settings report"})
		return
	}
	writeJSON(w, http.StatusOK, proof)
}

func (s *postgresSettingsReportStore) StoreSettingsReport(ctx context.Context, deviceUserID string, req settingsReportRequest, bundleJSON []byte) (settingsReportProof, error) {
	now := time.Now().UTC()
	reportID := "sr_" + uuid.NewString()
	var proof settingsReportProof

	err := s.store.Pool.QueryRow(ctx, `
		INSERT INTO settings_report_bundles
			(report_id, device_user_id, client_report_id, bundle_schema_version, bundle_type, bundle_hash,
			 bundle_size_bytes, app_version, runtime_id, sidecar_version, report_reason, user_note,
			 bundle_json, status, received_at, updated_at)
		VALUES
			($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13::jsonb, 'new', $14, $14)
		ON CONFLICT (device_user_id, client_report_id) DO NOTHING
		RETURNING report_id, status, received_at, client_report_id, bundle_hash
	`, reportID, deviceUserID, req.ClientReportID, req.BundleSchemaVersion, req.BundleType, req.BundleHash,
		len(bundleJSON), req.AppVersion, req.RuntimeID, req.SidecarVersion, req.ReportReason, req.UserNote,
		string(bundleJSON), now).Scan(&proof.ReportID, &proof.Status, &now, &proof.ClientReportID, &proof.BundleHash)
	if err == nil {
		proof.Accepted = true
		proof.ReceivedAt = now.Format(time.RFC3339)
		proof.Duplicate = false
		return proof, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return settingsReportProof{}, err
	}

	var existingReceivedAt time.Time
	err = s.store.Pool.QueryRow(ctx, `
		SELECT report_id, status, received_at, client_report_id, bundle_hash
		FROM settings_report_bundles
		WHERE device_user_id = $1 AND client_report_id = $2
	`, deviceUserID, req.ClientReportID).Scan(&proof.ReportID, &proof.Status, &existingReceivedAt, &proof.ClientReportID, &proof.BundleHash)
	if err != nil {
		return settingsReportProof{}, err
	}
	if proof.BundleHash != req.BundleHash {
		return settingsReportProof{}, errors.New("client_report_id_hash_mismatch")
	}
	proof.Accepted = true
	proof.ReceivedAt = existingReceivedAt.Format(time.RFC3339)
	proof.Duplicate = true
	return proof, nil
}

func (s *postgresSettingsReportStore) ListSettingsReports(ctx context.Context, statusFilter string) (settingsReportListResult, error) {
	counts, err := s.countSettingsReportsByStatus(ctx)
	if err != nil {
		return settingsReportListResult{}, err
	}

	baseQuery := `
		SELECT report_id, device_user_id, client_report_id, bundle_schema_version, bundle_type, bundle_hash,
		       bundle_size_bytes, app_version, runtime_id, sidecar_version, report_reason, user_note,
		       bundle_json::text, status, review_note, reviewed_by, received_at, updated_at, reviewed_at
		FROM settings_report_bundles`
	var rows pgx.Rows
	if statusFilter == "" || statusFilter == "all" {
		rows, err = s.store.Pool.Query(ctx, baseQuery+`
		ORDER BY received_at DESC
		LIMIT 100`)
	} else {
		rows, err = s.store.Pool.Query(ctx, baseQuery+`
		WHERE status = $1
		ORDER BY received_at DESC
		LIMIT 100`, statusFilter)
	}
	if err != nil {
		return settingsReportListResult{}, err
	}
	defer rows.Close()

	var records []settingsReportRecord
	for rows.Next() {
		record, err := scanSettingsReportRecord(rows)
		if err != nil {
			return settingsReportListResult{}, err
		}
		records = append(records, record)
	}
	if err := rows.Err(); err != nil {
		return settingsReportListResult{}, err
	}
	return settingsReportListResult{Records: records, Counts: counts}, nil
}

func (s *postgresSettingsReportStore) GetSettingsReport(ctx context.Context, reportID string) (settingsReportRecord, error) {
	row := s.store.Pool.QueryRow(ctx, `
		SELECT report_id, device_user_id, client_report_id, bundle_schema_version, bundle_type, bundle_hash,
		       bundle_size_bytes, app_version, runtime_id, sidecar_version, report_reason, user_note,
		       bundle_json::text, status, review_note, reviewed_by, received_at, updated_at, reviewed_at
		FROM settings_report_bundles
		WHERE report_id = $1`, reportID)
	record, err := scanSettingsReportRecord(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return settingsReportRecord{}, errSettingsReportNotFound
	}
	return record, err
}

func (s *postgresSettingsReportStore) UpdateSettingsReportStatus(ctx context.Context, reportID, status, reviewNote, reviewedBy string, now time.Time) (settingsReportRecord, error) {
	row := s.store.Pool.QueryRow(ctx, `
		UPDATE settings_report_bundles
		SET status = $2, review_note = $3, reviewed_by = $4, reviewed_at = $5, updated_at = $5
		WHERE report_id = $1
		RETURNING report_id, device_user_id, client_report_id, bundle_schema_version, bundle_type, bundle_hash,
		          bundle_size_bytes, app_version, runtime_id, sidecar_version, report_reason, user_note,
		          bundle_json::text, status, review_note, reviewed_by, received_at, updated_at, reviewed_at
	`, reportID, status, reviewNote, reviewedBy, now)
	record, err := scanSettingsReportRecord(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return settingsReportRecord{}, errSettingsReportNotFound
	}
	return record, err
}

func (s *postgresSettingsReportStore) countSettingsReportsByStatus(ctx context.Context) (map[string]int, error) {
	counts := canonicalSettingsReportStatusCounts()
	rows, err := s.store.Pool.Query(ctx, `
		SELECT status, COUNT(*)
		FROM settings_report_bundles
		WHERE status IN ('new', 'triaged', 'resolved', 'dismissed', 'needs_followup')
		GROUP BY status`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		var status string
		var count int
		if err := rows.Scan(&status, &count); err != nil {
			return nil, err
		}
		counts[status] = count
	}
	return counts, rows.Err()
}

type settingsReportScanner interface {
	Scan(dest ...any) error
}

func scanSettingsReportRecord(scanner settingsReportScanner) (settingsReportRecord, error) {
	var record settingsReportRecord
	var bundleJSON string
	err := scanner.Scan(
		&record.ReportID,
		&record.DeviceUserID,
		&record.ClientReportID,
		&record.BundleSchemaVersion,
		&record.BundleType,
		&record.BundleHash,
		&record.BundleSizeBytes,
		&record.AppVersion,
		&record.RuntimeID,
		&record.SidecarVersion,
		&record.ReportReason,
		&record.UserNote,
		&bundleJSON,
		&record.Status,
		&record.ReviewNote,
		&record.ReviewedBy,
		&record.ReceivedAt,
		&record.UpdatedAt,
		&record.ReviewedAt,
	)
	if err != nil {
		return settingsReportRecord{}, err
	}
	record.BundleJSON = json.RawMessage(bundleJSON)
	return record, nil
}

func hasBlockedSettingsReportField(value any) bool {
	switch typed := value.(type) {
	case map[string]any:
		for key, child := range typed {
			if isBlockedSettingsReportKey(key) || hasBlockedSettingsReportField(child) {
				return true
			}
		}
	case []any:
		for _, child := range typed {
			if hasBlockedSettingsReportField(child) {
				return true
			}
		}
	}
	return false
}

func isBlockedSettingsReportKey(key string) bool {
	normalized := strings.ToLower(strings.TrimSpace(key))
	compact := strings.NewReplacer("_", "", "-", "").Replace(normalized)
	switch normalized {
	case "authorization", "cookie", "session_jwt", "jwt", "api_key", "secret", "password", "token", "auth_header", "provider_config", "prompt_text", "hidden_prompt", "conversation_text", "private_tool_payload":
		return true
	default:
		return compact == "apikey" ||
			compact == "authheader" ||
			compact == "providerconfig" ||
			compact == "hiddenprompt" ||
			compact == "privatetoolpayload" ||
			strings.HasSuffix(normalized, "_api_key") ||
			strings.HasSuffix(compact, "apikey") ||
			strings.HasSuffix(normalized, "_token") ||
			strings.HasSuffix(normalized, "_secret") ||
			strings.HasSuffix(normalized, "_password")
	}
}

// ─── RevenueCat webhook ───────────────────────────────────────────────────────

// POST /billing/revenuecat
// RevenueCat sends events here. Auth via Authorization header matching REVENUECAT_WEBHOOK_AUTH.
func (a *App) handleRevenueCat(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "method not allowed"})
		return
	}
	// Verify shared secret if configured
	if a.cfg.RevenueCatWebhookAuth != "" {
		got := r.Header.Get("Authorization")
		if got != a.cfg.RevenueCatWebhookAuth {
			writeJSON(w, http.StatusUnauthorized, map[string]any{"error": "invalid webhook auth"})
			return
		}
	}

	var payload map[string]any
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": err.Error()})
		return
	}
	defer r.Body.Close()

	// RevenueCat wraps events in an "event" key
	eventData, _ := payload["event"].(map[string]any)
	if eventData == nil {
		writeJSON(w, http.StatusOK, map[string]any{"ok": true, "note": "no event key"})
		return
	}
	eventType, _ := eventData["type"].(string)
	// app_user_id should be set to device_user_id at purchase time
	appUserID, _ := eventData["app_user_id"].(string)

	_ = a.entitlements.ProcessRevenueCatEvent(r.Context(), eventType, appUserID)
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// ─── Admin ────────────────────────────────────────────────────────────────────

// GET /admin does not serve a private admin console from the public relay.
func (a *App) handleAdminPage(w http.ResponseWriter, r *http.Request) {
	writeAdminDisabled(w)
}

func (a *App) handleAdminStats(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "method not allowed"})
		return
	}
	stats, err := a.entitlements.Stats(r.Context())
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": "could not load admin stats"})
		return
	}
	writeJSON(w, http.StatusOK, projectAdminStats(stats, int64(time.Since(a.started).Seconds())))
}

func (a *App) handleAdminCreateInvites(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "method not allowed"})
		return
	}
	var req struct {
		Count int `json:"count"`
		Days  int `json:"days"`
	}
	r.Body = http.MaxBytesReader(w, r.Body, maxAdminInviteCreateBodyBytes)
	if err := decodeJSON(r, &req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": err.Error()})
		return
	}
	if req.Count <= 0 || req.Count > maxAdminInviteCreateCount {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "count must be between 1 and 500"})
		return
	}
	if req.Days <= 0 || req.Days > maxAdminInviteCreateDays {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "days must be between 1 and 365"})
		return
	}
	codes, err := a.entitlements.CreateInviteCodes(r.Context(), req.Count, req.Days, "admin")
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": "could not create invite codes"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "codes": codes, "count": len(codes)})
}

func (a *App) handleAdminListInvites(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "method not allowed"})
		return
	}
	list, err := a.entitlements.ListInviteCodes(r.Context())
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": "could not list invite codes"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "invite_codes": projectAdminInviteCodes(list)})
}

func (a *App) handleAdminErrors(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "method not allowed"})
		return
	}
	list, err := a.entitlements.ListErrorReports(r.Context())
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": "could not list error reports"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "error_reports": projectAdminErrorReports(list)})
}

func (a *App) handleAdminAIReports(w http.ResponseWriter, r *http.Request) {
	writeAdminDisabled(w)
}

func (a *App) handleAdminAIReportStatus(w http.ResponseWriter, r *http.Request) {
	writeAdminDisabled(w)
}

func writeAdminDisabled(w http.ResponseWriter) {
	writeJSON(w, http.StatusNotFound, map[string]any{"error": "not found"})
}

func projectAdminStats(stats map[string]any, uptimeSeconds int64) map[string]any {
	return map[string]any{
		"ok":                   true,
		"total_device_users":   safeNumber(stats["total_device_users"]),
		"verified_emails":      safeNumber(stats["verified_emails"]),
		"active_trials":        safeNumber(stats["active_trials"]),
		"pending_trials":       safeNumber(stats["pending_trials"]),
		"active_subscriptions": safeNumber(stats["active_subscriptions"]),
		"active_invites":       safeNumber(stats["active_invites"]),
		"unused_invite_codes":  safeNumber(stats["unused_invite_codes"]),
		"total_llm_turns":      safeNumber(stats["total_llm_turns"]),
		"relay_uptime_sec":     uptimeSeconds,
	}
}

func projectAdminInviteCodes(list []map[string]any) []map[string]any {
	projected := make([]map[string]any, 0, len(list))
	for _, item := range list {
		projected = append(projected, projectAdminInviteCode(item))
	}
	return projected
}

func projectAdminInviteCode(item map[string]any) map[string]any {
	projected := map[string]any{
		"code_masked":  maskInviteCode(safeString(item["code"], "")),
		"granted_days": safeNumber(item["granted_days"]),
		"created_at":   redactAdminText(safeString(item["created_at"], ""), nil),
		"created_by":   redactAdminText(safeString(item["created_by"], ""), nil),
		"used":         safeBool(item["used"]),
	}
	if usedAt := redactAdminText(safeString(item["used_at"], ""), nil); usedAt != "" {
		projected["used_at"] = usedAt
	}
	if safeString(item["used_by"], "") != "" {
		projected["used_by_ref"] = "[suppressed]"
	}
	return projected
}

func projectAdminErrorReports(list []map[string]any) []map[string]any {
	projected := make([]map[string]any, 0, len(list))
	for _, item := range list {
		projected = append(projected, map[string]any{
			"id":          safeNumber(item["id"]),
			"error_type":  redactAdminText(safeString(item["error_type"], ""), nil),
			"message":     redactAdminText(safeString(item["message"], ""), nil),
			"created_at":  redactAdminText(safeString(item["created_at"], ""), nil),
			"device_ref":  suppressedRef(item["device_user_id"]),
			"redacted":    true,
			"safe_record": true,
		})
	}
	return projected
}

func maskInviteCode(code string) string {
	code = strings.TrimSpace(code)
	if code == "" {
		return ""
	}
	if len(code) <= 4 {
		return "[masked]"
	}
	return code[:2] + "****" + code[len(code)-2:]
}

func suppressedRef(value any) string {
	if safeString(value, "") == "" {
		return ""
	}
	return "[suppressed]"
}

func safeBool(value any) bool {
	typed, _ := value.(bool)
	return typed
}

func (a *App) handleAdminSettingsReports(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "method not allowed"})
		return
	}
	statusFilter := strings.TrimSpace(strings.ToLower(r.URL.Query().Get("status")))
	if statusFilter != "" && statusFilter != "all" && !isAllowedSettingsReportStatus(statusFilter) {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid settings report status filter"})
		return
	}
	result, err := a.settingsReportRepository().ListSettingsReports(r.Context(), statusFilter)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": "could not list settings reports"})
		return
	}
	var list []map[string]any
	for _, record := range result.Records {
		list = append(list, projectSettingsReportListItem(record))
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"ok":               true,
		"filter_status":    normalizedSettingsReportStatusFilter(statusFilter),
		"canonical_status": canonicalSettingsReportStatuses,
		"counts":           ensureCanonicalSettingsReportCounts(result.Counts),
		"settings_reports": list,
		"redaction_notice": settingsReportRedactionNotice,
	})
}

func (a *App) handleAdminSettingsReportDetail(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "method not allowed"})
		return
	}
	reportID := strings.TrimSpace(r.URL.Query().Get("report_id"))
	if !isSafeSettingsReportID(reportID) {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "valid report_id is required"})
		return
	}
	record, err := a.settingsReportRepository().GetSettingsReport(r.Context(), reportID)
	if errors.Is(err, errSettingsReportNotFound) {
		writeJSON(w, http.StatusNotFound, map[string]any{"error": "settings report not found"})
		return
	}
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": "could not load settings report"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "settings_report": projectSettingsReportDetail(record)})
}

func (a *App) handleAdminSettingsReportStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "method not allowed"})
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, maxSettingsReportStatusBodyBytes)
	var req struct {
		ReportID   string `json:"report_id"`
		Status     string `json:"status"`
		ReviewNote string `json:"review_note"`
		ReviewedBy string `json:"reviewed_by"`
	}
	if err := decodeJSON(r, &req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": err.Error()})
		return
	}
	req.ReportID = strings.TrimSpace(req.ReportID)
	req.Status = strings.TrimSpace(strings.ToLower(req.Status))
	req.ReviewNote = strings.TrimSpace(req.ReviewNote)
	req.ReviewedBy = strings.TrimSpace(req.ReviewedBy)
	if !isSafeSettingsReportID(req.ReportID) {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "report_id is required"})
		return
	}
	if !isAllowedSettingsReportStatus(req.Status) {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid settings report status"})
		return
	}
	if len(req.ReviewNote) > maxSettingsReportReviewNoteBytes {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "review_note is too long"})
		return
	}
	if len(req.ReviewedBy) > maxSettingsReportReviewedByBytes {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "reviewed_by is too long"})
		return
	}
	req.ReviewNote = redactAdminText(req.ReviewNote, nil)
	req.ReviewedBy = redactAdminText(req.ReviewedBy, nil)
	now := time.Now().UTC()
	record, err := a.settingsReportRepository().UpdateSettingsReportStatus(r.Context(), req.ReportID, req.Status, req.ReviewNote, req.ReviewedBy, now)
	if errors.Is(err, errSettingsReportNotFound) {
		writeJSON(w, http.StatusNotFound, map[string]any{"error": "settings report not found"})
		return
	}
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": "could not update settings report"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "settings_report": projectSettingsReportDetail(record)})
}

func isAllowedSettingsReportStatus(status string) bool {
	for _, allowed := range canonicalSettingsReportStatuses {
		if status == allowed {
			return true
		}
	}
	return false
}

func canonicalSettingsReportStatusCounts() map[string]int {
	counts := make(map[string]int, len(canonicalSettingsReportStatuses))
	for _, status := range canonicalSettingsReportStatuses {
		counts[status] = 0
	}
	return counts
}

func ensureCanonicalSettingsReportCounts(input map[string]int) map[string]int {
	counts := canonicalSettingsReportStatusCounts()
	for _, status := range canonicalSettingsReportStatuses {
		counts[status] = input[status]
	}
	return counts
}

func normalizedSettingsReportStatusFilter(status string) string {
	if status == "" {
		return "all"
	}
	return status
}

func isSafeSettingsReportID(reportID string) bool {
	if reportID == "" || len(reportID) > 128 {
		return false
	}
	for _, ch := range reportID {
		if (ch >= 'a' && ch <= 'z') || (ch >= 'A' && ch <= 'Z') || (ch >= '0' && ch <= '9') || ch == '_' || ch == '-' {
			continue
		}
		return false
	}
	return true
}

const settingsReportUnavailable = "Unavailable in this report."
const settingsReportRedactionNotice = "Sensitive fields are redacted or suppressed before display. Free-text secrets may not be perfectly detectable; treat report text carefully."

var adminTextRedactionPatterns = []*regexp.Regexp{
	regexp.MustCompile(`(?i)authorization\s*[:=]\s*bearer\s+[^\s,"'}]+`),
	regexp.MustCompile(`(?i)\bbearer\s+[A-Za-z0-9._-]{12,}`),
	regexp.MustCompile(`\beyJ[A-Za-z0-9_-]{10,}\.[A-Za-z0-9_-]{10,}\.[A-Za-z0-9_-]{10,}\b`),
	regexp.MustCompile(`\bsk-[A-Za-z0-9_-]{12,}\b`),
	regexp.MustCompile(`(?i)\bxai-[A-Za-z0-9_-]{12,}\b`),
	regexp.MustCompile(`(?i)\bsk-ant-[A-Za-z0-9_-]{12,}\b`),
	regexp.MustCompile(`(?i)\banthropic[_-]?[A-Za-z0-9_-]{16,}\b`),
	regexp.MustCompile(`(?i)\b(password|secret|token|api[_-]?key|provider[_-]?key)\s*[:=]\s*["']?[^"'\s,;]{4,}`),
	regexp.MustCompile(`\b[A-Za-z0-9_-]{64,}\b`),
}

func projectSettingsReportListItem(record settingsReportRecord) map[string]any {
	bundle := decodeSettingsReportBundle(record.BundleJSON)
	tracker := &redactionTracker{}
	userNote := redactAdminText(record.UserNote, tracker)
	sourceAvailability := safeMapSection(bundle["source_availability"], tracker)
	redactionSummary := safeMapSection(bundle["redaction_summary"], tracker)
	reportSource := safeString(bundle["report_source"], "settings_report")

	item := map[string]any{
		"report_id":               record.ReportID,
		"client_report_id":        record.ClientReportID,
		"bundle_schema_version":   record.BundleSchemaVersion,
		"bundle_type":             record.BundleType,
		"bundle_hash":             record.BundleHash,
		"bundle_size_bytes":       record.BundleSizeBytes,
		"app_version":             redactAdminText(record.AppVersion, tracker),
		"runtime_id":              redactAdminText(record.RuntimeID, tracker),
		"sidecar_version":         redactAdminText(record.SidecarVersion, tracker),
		"report_reason":           redactAdminText(record.ReportReason, tracker),
		"report_source":           redactAdminText(reportSource, tracker),
		"user_note_preview":       limitString(userNote, 180),
		"status":                  record.Status,
		"review_note_preview":     limitString(redactAdminText(record.ReviewNote, tracker), 180),
		"reviewed_by":             redactAdminText(record.ReviewedBy, tracker),
		"received_at":             record.ReceivedAt.Format(time.RFC3339),
		"updated_at":              record.UpdatedAt.Format(time.RFC3339),
		"reviewed_at":             formatOptionalTime(record.ReviewedAt),
		"source_availability":     sourceAvailability,
		"redaction_summary":       redactionSummary,
		"redaction_notice":        settingsReportRedactionNotice,
		"redaction_applied":       tracker.Redacted || tracker.SuppressedFields > 0,
		"suppressed_field_count":  tracker.SuppressedFields,
		"unavailable_field_notes": unavailableSettingsReportFields(),
	}
	return item
}

func projectSettingsReportDetail(record settingsReportRecord) map[string]any {
	bundle := decodeSettingsReportBundle(record.BundleJSON)
	tracker := &redactionTracker{}
	reportSource := safeString(bundle["report_source"], "settings_report")

	detail := map[string]any{
		"report_summary": map[string]any{
			"report_id":             record.ReportID,
			"client_report_id":      record.ClientReportID,
			"bundle_schema_version": record.BundleSchemaVersion,
			"bundle_type":           record.BundleType,
			"report_reason":         redactAdminText(record.ReportReason, tracker),
			"report_source":         redactAdminText(reportSource, tracker),
			"status":                record.Status,
			"received_at":           record.ReceivedAt.Format(time.RFC3339),
			"updated_at":            record.UpdatedAt.Format(time.RFC3339),
			"reviewed_at":           formatOptionalTime(record.ReviewedAt),
		},
		"user_note":              redactAdminText(record.UserNote, tracker),
		"conversation_excerpt":   projectConversationExcerpt(bundle["today_conversation"], tracker),
		"runtime_event_summary":  projectRuntimeEvents(bundle["recent_runtime_events"], tracker),
		"runtime_state_summary":  safeMapSection(bundle["runtime_state_summary"], tracker),
		"source_availability":    safeMapSection(bundle["source_availability"], tracker),
		"redaction_summary":      safeMapSection(bundle["redaction_summary"], tracker),
		"app_runtime_metadata":   safeMapSection(bundle["app_runtime_metadata"], tracker),
		"hash_metadata":          map[string]any{"bundle_hash": record.BundleHash, "bundle_size_bytes": record.BundleSizeBytes},
		"app_version":            redactAdminText(record.AppVersion, tracker),
		"runtime_id":             redactAdminText(record.RuntimeID, tracker),
		"sidecar_version":        redactAdminText(record.SidecarVersion, tracker),
		"review_decision":        projectReviewDecision(record, tracker),
		"redaction_notice":       settingsReportRedactionNotice,
		"unavailable_field_note": settingsReportUnavailable,
		"unavailable_fields":     unavailableSettingsReportFields(),
		"raw_bundle_returned":    false,
		"suppressed_field_count": tracker.SuppressedFields,
	}
	detail["redaction_applied"] = tracker.Redacted || tracker.SuppressedFields > 0
	return detail
}

func projectReviewDecision(record settingsReportRecord, tracker *redactionTracker) map[string]any {
	return map[string]any{
		"status":      record.Status,
		"review_note": redactAdminText(record.ReviewNote, tracker),
		"reviewed_by": redactAdminText(record.ReviewedBy, tracker),
		"reviewed_at": formatOptionalTime(record.ReviewedAt),
		"updated_at":  record.UpdatedAt.Format(time.RFC3339),
	}
}

func decodeSettingsReportBundle(raw json.RawMessage) map[string]any {
	var bundle map[string]any
	if len(bytes.TrimSpace(raw)) == 0 {
		return map[string]any{}
	}
	if err := json.Unmarshal(raw, &bundle); err != nil {
		return map[string]any{}
	}
	return bundle
}

func projectConversationExcerpt(value any, tracker *redactionTracker) map[string]any {
	section, ok := value.(map[string]any)
	if !ok {
		return unavailableSection()
	}
	out := map[string]any{
		"source":        redactAdminText(safeString(section["source"], ""), tracker),
		"availability":  redactAdminText(safeString(section["availability"], "unknown"), tracker),
		"chat_date":     redactAdminText(safeString(section["chat_date"], ""), tracker),
		"message_count": safeNumber(section["message_count"]),
	}
	messages, _ := section["messages"].([]any)
	if len(messages) == 0 {
		out["messages"] = []any{}
		return out
	}
	start := len(messages) - 20
	if start < 0 {
		start = 0
	}
	projected := make([]map[string]any, 0, len(messages)-start)
	for _, item := range messages[start:] {
		message, ok := item.(map[string]any)
		if !ok {
			continue
		}
		projected = append(projected, map[string]any{
			"role":              redactAdminText(safeString(message["role"], "unknown"), tracker),
			"text":              redactAdminText(safeString(message["text"], ""), tracker),
			"task_id":           nullableAdminText(message["task_id"], tracker),
			"attempt_id":        nullableAdminText(message["attempt_id"], tracker),
			"source_event_type": nullableAdminText(message["source_event_type"], tracker),
			"created_at":        redactAdminText(safeString(message["created_at"], ""), tracker),
		})
	}
	out["messages"] = projected
	return out
}

func projectRuntimeEvents(value any, tracker *redactionTracker) map[string]any {
	section, ok := value.(map[string]any)
	if !ok {
		return unavailableSection()
	}
	out := map[string]any{
		"source":       redactAdminText(safeString(section["source"], ""), tracker),
		"availability": redactAdminText(safeString(section["availability"], "unknown"), tracker),
		"event_count":  safeNumber(section["event_count"]),
	}
	events, _ := section["events"].([]any)
	if len(events) == 0 {
		out["events"] = []any{}
		return out
	}
	start := len(events) - 20
	if start < 0 {
		start = 0
	}
	projected := make([]map[string]any, 0, len(events)-start)
	for _, item := range events[start:] {
		event, ok := item.(map[string]any)
		if !ok {
			continue
		}
		projected = append(projected, map[string]any{
			"type":      redactAdminText(safeString(event["type"], "unknown"), tracker),
			"timestamp": redactAdminText(safeString(event["timestamp"], ""), tracker),
			"source":    redactAdminText(safeString(event["source"], "unknown"), tracker),
			"summary":   redactAdminText(safeString(event["summary"], ""), tracker),
		})
	}
	out["events"] = projected
	return out
}

func safeMapSection(value any, tracker *redactionTracker) map[string]any {
	section, ok := value.(map[string]any)
	if !ok {
		return unavailableSection()
	}
	safe := map[string]any{}
	for key, child := range section {
		if isBlockedSettingsReportKey(key) {
			tracker.SuppressedFields++
			continue
		}
		safe[redactAdminText(key, tracker)] = safeProjectionValue(child, tracker)
	}
	if len(safe) == 0 {
		return unavailableSection()
	}
	return safe
}

func safeProjectionValue(value any, tracker *redactionTracker) any {
	switch typed := value.(type) {
	case string:
		return redactAdminText(typed, tracker)
	case float64, bool, nil:
		return typed
	case []any:
		out := make([]any, 0, len(typed))
		for _, item := range typed {
			out = append(out, safeProjectionValue(item, tracker))
		}
		return out
	case map[string]any:
		out := map[string]any{}
		for key, child := range typed {
			if isBlockedSettingsReportKey(key) {
				tracker.SuppressedFields++
				continue
			}
			out[redactAdminText(key, tracker)] = safeProjectionValue(child, tracker)
		}
		return out
	default:
		return redactAdminText(safeString(typed, ""), tracker)
	}
}

func redactAdminText(text string, tracker *redactionTracker) string {
	if text == "" {
		return ""
	}
	redacted := text
	for _, pattern := range adminTextRedactionPatterns {
		next := pattern.ReplaceAllString(redacted, "[redacted]")
		if next != redacted && tracker != nil {
			tracker.Redacted = true
		}
		redacted = next
	}
	return redacted
}

func nullableAdminText(value any, tracker *redactionTracker) any {
	if value == nil {
		return nil
	}
	text := safeString(value, "")
	if text == "" {
		return nil
	}
	return redactAdminText(text, tracker)
}

func safeString(value any, fallback string) string {
	switch typed := value.(type) {
	case string:
		if strings.TrimSpace(typed) == "" {
			return fallback
		}
		return strings.TrimSpace(typed)
	case float64:
		return strings.TrimSpace(strings.TrimRight(strings.TrimRight(strconvFormatFloat(typed), "0"), "."))
	case bool:
		if typed {
			return "true"
		}
		return "false"
	default:
		return fallback
	}
}

func strconvFormatFloat(value float64) string {
	return strings.TrimRight(strings.TrimRight(fmt.Sprintf("%.6f", value), "0"), ".")
}

func safeNumber(value any) any {
	switch typed := value.(type) {
	case float64:
		return typed
	case int:
		return typed
	case int64:
		return typed
	default:
		return 0
	}
}

func formatOptionalTime(value *time.Time) any {
	if value == nil {
		return nil
	}
	return value.Format(time.RFC3339)
}

func unavailableSection() map[string]any {
	return map[string]any{"availability": "unavailable", "note": settingsReportUnavailable}
}

func unavailableSettingsReportFields() []string {
	return []string{
		"build identifier",
		"structured settings snapshot",
		"structured permissions/setup snapshot",
		"current route/screen beyond report_source",
		"explicit error class unless present in safe runtime events",
	}
}

func limitString(value string, max int) string {
	if max <= 0 || len(value) <= max {
		return value
	}
	return value[:max]
}

// ─── Middleware ───────────────────────────────────────────────────────────────

func (a *App) authRequired(next func(http.ResponseWriter, *http.Request, map[string]any)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		auth := r.Header.Get("Authorization")
		if !strings.HasPrefix(auth, "Bearer ") {
			writeJSON(w, http.StatusUnauthorized, map[string]any{"error": "missing bearer token"})
			return
		}
		raw := strings.TrimPrefix(auth, "Bearer ")
		claims, err := a.sessions.ParseJWT(raw)
		if err != nil {
			writeJSON(w, http.StatusUnauthorized, map[string]any{"error": "invalid token"})
			return
		}
		next(w, r, claims)
	}
}

func (a *App) adminRequired(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if a.cfg.AdminPassword == "" {
			writeJSON(w, http.StatusForbidden, map[string]any{"error": "admin access denied"})
			return
		}
		if r.Header.Get("X-Admin-Password") != a.cfg.AdminPassword {
			writeJSON(w, http.StatusUnauthorized, map[string]any{"error": "admin access denied"})
			return
		}
		next(w, r)
	}
}

// ─── Helpers ──────────────────────────────────────────────────────────────────

func decodeJSON(r *http.Request, out interface{}) error {
	defer r.Body.Close()
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	return decoder.Decode(out)
}

func writeJSON(w http.ResponseWriter, status int, payload interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

func serveVerifyHTML(w http.ResponseWriter, ok bool, message string) {
	icon := "✓"
	color := "#22c55e"
	if !ok {
		icon = "✗"
		color = "#ef4444"
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(`<!DOCTYPE html><html><head><meta charset="utf-8">
<meta name="viewport" content="width=device-width,initial-scale=1">
<title>xMilo Verification</title>
<style>body{font-family:system-ui,sans-serif;background:#0b1020;color:#e2e8f0;display:flex;align-items:center;justify-content:center;min-height:100vh;margin:0}
.card{background:#111827;border-radius:18px;padding:32px;max-width:380px;text-align:center;border:1px solid #1f2937}
.icon{font-size:48px;margin-bottom:16px;color:` + color + `}
.msg{line-height:1.6;color:#cbd5e1}
.brand{margin-top:24px;color:#93c5fd;font-size:13px;font-weight:700;letter-spacing:.05em}
</style></head><body>
<div class="card"><div class="icon">` + icon + `</div><p class="msg">` + message + `</p>
<p class="brand">xMilo</p></div></body></html>`))
}
