package httpx

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"

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

	// RevenueCat billing webhook
	mux.HandleFunc("/billing/revenuecat", a.handleRevenueCat)

	// Hidden admin page — password required via X-Admin-Password header
	mux.HandleFunc("/admin", a.handleAdminPage)
	mux.HandleFunc("/admin/stats", a.adminRequired(a.handleAdminStats))
	mux.HandleFunc("/admin/invite/create", a.adminRequired(a.handleAdminCreateInvites))
	mux.HandleFunc("/admin/invites", a.adminRequired(a.handleAdminListInvites))
	mux.HandleFunc("/admin/errors", a.adminRequired(a.handleAdminErrors))
	mux.HandleFunc("/admin/ai-reports", a.adminRequired(a.handleAdminAIReports))
	mux.HandleFunc("/admin/ai-reports/status", a.adminRequired(a.handleAdminAIReportStatus))

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

// GET /admin — serves the admin HTML page (always accessible; API endpoints require password)
func (a *App) handleAdminPage(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(adminHTML))
}

func (a *App) handleAdminStats(w http.ResponseWriter, r *http.Request) {
	stats, err := a.entitlements.Stats(r.Context())
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}
	stats["relay_uptime_sec"] = int64(time.Since(a.started).Seconds())
	stats["access_mode"] = a.cfg.AccessMode
	stats["access_code_only"] = a.cfg.AccessCodeOnly()
	stats["trial_allowed"] = a.cfg.TrialAllowed()
	stats["subscription_allowed"] = a.cfg.SubscriptionAllowed()
	stats["access_code_days"] = a.cfg.InviteBetaDays
	writeJSON(w, http.StatusOK, stats)
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
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": err.Error()})
		return
	}
	defer r.Body.Close()
	if req.Count <= 0 || req.Count > 500 {
		req.Count = 10
	}
	if req.Days <= 0 {
		req.Days = a.cfg.InviteBetaDays
	}
	codes, err := a.entitlements.CreateInviteCodes(r.Context(), req.Count, req.Days, "admin")
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "codes": codes, "count": len(codes)})
}

func (a *App) handleAdminListInvites(w http.ResponseWriter, r *http.Request) {
	list, err := a.entitlements.ListInviteCodes(r.Context())
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "invite_codes": list})
}

func (a *App) handleAdminErrors(w http.ResponseWriter, r *http.Request) {
	list, err := a.entitlements.ListErrorReports(r.Context())
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "error_reports": list})
}

func (a *App) handleAdminAIReports(w http.ResponseWriter, r *http.Request) {
	list, err := a.entitlements.ListAIContentReports(r.Context())
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "ai_content_reports": list})
}

func (a *App) handleAdminAIReportStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "method not allowed"})
		return
	}
	var req struct {
		ID         int64  `json:"id"`
		Status     string `json:"status"`
		ReviewNote string `json:"review_note"`
	}
	if err := decodeJSON(r, &req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": err.Error()})
		return
	}
	if req.ID <= 0 {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "id must be positive"})
		return
	}
	if err := a.entitlements.UpdateAIContentReportStatus(r.Context(), req.ID, req.Status, req.ReviewNote); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
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
			writeJSON(w, http.StatusForbidden, map[string]any{"error": "admin not configured (set RELAY_ADMIN_PASSWORD)"})
			return
		}
		if r.Header.Get("X-Admin-Password") != a.cfg.AdminPassword {
			writeJSON(w, http.StatusUnauthorized, map[string]any{"error": "invalid admin password"})
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

// adminHTML is the self-contained admin page served at GET /admin.
// It uses fetch() to call the admin API endpoints with the password in X-Admin-Password.
const adminHTML = `<!DOCTYPE html>
<html><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1">
<title>xMilo Admin</title>
<style>
*{box-sizing:border-box}body{font-family:monospace;background:#0b1020;color:#e2e8f0;padding:20px;max-width:900px;margin:0 auto}
h1{color:#93c5fd;margin-bottom:4px}h2{color:#7dd3fc;margin:24px 0 8px}
.row{display:flex;gap:8px;align-items:center;flex-wrap:wrap;margin:8px 0}
input,button,select{padding:8px 12px;border-radius:6px;border:1px solid #334155;background:#1e293b;color:#e2e8f0;font:inherit}
button{cursor:pointer;background:#2563eb}button:hover{background:#1d4ed8}
.danger{background:#dc2626}pre{background:#111827;padding:16px;border-radius:8px;overflow:auto;white-space:pre-wrap;font-size:11px;max-height:400px;border:1px solid #1f2937}
.pill{display:inline-block;padding:2px 8px;border-radius:999px;font-size:11px;background:#1e293b;border:1px solid #334155}
</style></head><body>
<h1>xMilo Admin</h1>
<div class="row"><input type="password" id="pw" placeholder="Admin password" style="width:240px"><span class="pill" id="auth-state">not set</span></div>
<script>
const pw=()=>document.getElementById('pw').value;
const h=()=>({'Content-Type':'application/json','X-Admin-Password':pw()});
document.getElementById('pw').oninput=()=>{document.getElementById('auth-state').textContent='changed — re-load data'};

async function api(path,method='GET',body){
  const opts={method,headers:h()};
  if(body)opts.body=JSON.stringify(body);
  const r=await fetch(path,opts);
  return r.json();
}
async function show(id,fn){
  document.getElementById(id).textContent='loading…';
  try{document.getElementById(id).textContent=JSON.stringify(await fn(),null,2)}
  catch(e){document.getElementById(id).textContent='Error: '+e}
}
</script>

<h2>Stats</h2>
<button onclick="show('out-stats',()=>api('/admin/stats'))">Refresh</button>
<pre id="out-stats">Press Refresh</pre>

<h2>Access Codes — Create</h2>
<div class="row">
  <input type="number" id="cnt" value="10" min="1" max="500" style="width:70px"> codes &nbsp;
  <input type="number" id="days" value="5" min="1" style="width:60px"> days &nbsp;
  <button onclick="show('out-inv',()=>api('/admin/invite/create','POST',{count:+cnt.value,days:+days.value}))">Create</button>
</div>
<pre id="out-inv">—</pre>

<h2>Access Codes — List</h2>
<button onclick="show('out-inv-list',()=>api('/admin/invites'))">Load</button>
<pre id="out-inv-list">—</pre>

<h2>Error Reports</h2>
<button onclick="show('out-errors',()=>api('/admin/errors'))">Load</button>
<pre id="out-errors">—</pre>

<h2>AI Content Reports</h2>
<button onclick="show('out-ai-reports',()=>api('/admin/ai-reports'))">Load</button>
<div class="row">
  <input type="number" id="air-id" placeholder="report id" style="width:110px">
  <select id="air-status">
    <option value="new">new</option>
    <option value="triaged">triaged</option>
    <option value="confirmed">confirmed</option>
    <option value="dismissed">dismissed</option>
    <option value="rule_update_needed">rule_update_needed</option>
  </select>
  <input type="text" id="air-note" placeholder="review note" style="width:240px">
  <button onclick="show('out-ai-status',()=>api('/admin/ai-reports/status','POST',{id:+document.getElementById('air-id').value,status:document.getElementById('air-status').value,review_note:document.getElementById('air-note').value}))">Update status</button>
</div>
<pre id="out-ai-status">—</pre>
<pre id="out-ai-reports">—</pre>
</body></html>`
