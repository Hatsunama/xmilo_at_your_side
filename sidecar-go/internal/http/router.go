package httpx

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"xmilo/sidecar-go/internal/buildinfo"
	"xmilo/sidecar-go/internal/config"
	"xmilo/sidecar-go/internal/contextpolicy"
	"xmilo/sidecar-go/internal/db"
	"xmilo/sidecar-go/internal/legacy"
	"xmilo/sidecar-go/internal/llm"
	"xmilo/sidecar-go/internal/maintenance"
	"xmilo/sidecar-go/internal/mind"
	"xmilo/sidecar-go/internal/movement"
	"xmilo/sidecar-go/internal/netutil"
	"xmilo/sidecar-go/internal/providerpolicy"
	"xmilo/sidecar-go/internal/relay"
	"xmilo/sidecar-go/internal/runtime"
	"xmilo/sidecar-go/internal/tasks"
	"xmilo/sidecar-go/internal/ws"
)

type App struct {
	cfg            config.Config
	store          *db.Store
	hub            *ws.Hub
	engine         *tasks.Engine
	planner        *movement.Planner
	relayClient    *relay.Client // kept on App for background refresh goroutine
	startedAt      time.Time
	mindLoaded     bool
	wakeLockActive bool
	wakeLockStatus string
}

func NewApp(cfg config.Config) (*App, error) {
	store, err := db.Open(cfg.DBPath)
	if err != nil {
		return nil, err
	}

	if err := legacy.RunOnce(store, cfg.MindRoot); err != nil {
		return nil, err
	}

	prompt, err := mind.New(cfg.MindRoot).SystemPrompt()
	if err != nil {
		return nil, err
	}

	getJWT := func() (string, error) { return store.GetRuntimeConfig("relay_session_jwt") }
	relayClient := relay.New(cfg.RelayBaseURL, getJWT)

	hub := ws.NewHub()
	var turnClient tasks.TurnClient = relayClient
	if cfg.LocalBYOKActive() {
		localClient, err := llm.NewLocalProvider(cfg)
		if err != nil {
			return nil, err
		}
		turnClient = localClient
	}
	engine := tasks.New(store, turnClient, hub, prompt)
	if err := engine.ReconcileStaleActiveTask(); err != nil {
		return nil, err
	}
	planner := movement.NewPlanner()

	return &App{
		cfg:            cfg,
		store:          store,
		hub:            hub,
		engine:         engine,
		planner:        planner,
		relayClient:    relayClient,
		startedAt:      time.Now().UTC(),
		mindLoaded:     true,
		wakeLockStatus: "unsupported_by_sidecar_runtime_host",
	}, nil
}

func (a *App) Start(ctx context.Context) error {
	a.markWakeLockUnsupported()

	if err := a.store.SetRuntimeConfig("last_meaningful_user_action_at", time.Now().UTC().Format(time.RFC3339)); err != nil {
		return err
	}

	maintenance.Start(ctx, a.store, a.hub)

	mux := http.NewServeMux()
	mux.Handle("/health", RequireBearer(a.cfg.BearerToken, http.HandlerFunc(a.handleHealth)))
	mux.Handle("/ready", RequireBearer(a.cfg.BearerToken, http.HandlerFunc(a.handleReady)))
	mux.Handle("/bootstrap", RequireBearer(a.cfg.BearerToken, http.HandlerFunc(a.handleBootstrap)))

	// Auth passthrough: sidecar is the sole relay caller; app never speaks to relay directly.
	mux.Handle("/auth/register", RequireBearer(a.cfg.BearerToken, http.HandlerFunc(a.handleAuthRegister)))
	mux.Handle("/auth/invite", RequireBearer(a.cfg.BearerToken, http.HandlerFunc(a.handleAuthInvite)))
	mux.Handle("/auth/check", RequireBearer(a.cfg.BearerToken, http.HandlerFunc(a.handleAuthCheck)))
	// /auth/refresh receives a JWT pushed down from the app (e.g. after RevenueCat purchase).
	mux.Handle("/auth/refresh", RequireBearer(a.cfg.BearerToken, http.HandlerFunc(a.handleAuthRefresh)))
	mux.Handle("/auth/logout", RequireBearer(a.cfg.BearerToken, http.HandlerFunc(a.handleAuthLogout)))
	mux.Handle("/auth/account/delete", RequireBearer(a.cfg.BearerToken, http.HandlerFunc(a.handleAuthDeleteAccount)))
	mux.Handle("/auth/2fa/status", RequireBearer(a.cfg.BearerToken, http.HandlerFunc(a.handleTwoFactorStatus)))
	mux.Handle("/auth/2fa/setup/begin", RequireBearer(a.cfg.BearerToken, http.HandlerFunc(a.handleTwoFactorBegin)))
	mux.Handle("/auth/2fa/setup/confirm", RequireBearer(a.cfg.BearerToken, http.HandlerFunc(a.handleTwoFactorConfirm)))
	mux.Handle("/auth/2fa/verify", RequireBearer(a.cfg.BearerToken, http.HandlerFunc(a.handleTwoFactorVerify)))
	mux.Handle("/auth/2fa/disable", RequireBearer(a.cfg.BearerToken, http.HandlerFunc(a.handleTwoFactorDisable)))
	mux.Handle("/auth/2fa/recovery/regenerate", RequireBearer(a.cfg.BearerToken, http.HandlerFunc(a.handleTwoFactorRecoveryRegenerate)))
	mux.Handle("/auth/website/handoff/create", RequireBearer(a.cfg.BearerToken, http.HandlerFunc(a.handleWebsiteHandoffCreate)))

	mux.Handle("/task/start", RequireBearer(a.cfg.BearerToken, http.HandlerFunc(a.handleTaskStart)))
	mux.Handle("/command/submit", RequireBearer(a.cfg.BearerToken, http.HandlerFunc(a.handleCommandSubmit)))
	mux.Handle("/task/current", RequireBearer(a.cfg.BearerToken, http.HandlerFunc(a.handleTaskCurrent)))
	mux.Handle("/task/interrupt", RequireBearer(a.cfg.BearerToken, http.HandlerFunc(a.handleTaskInterrupt)))
	mux.Handle("/local-provider/options", RequireBearer(a.cfg.BearerToken, http.HandlerFunc(a.handleLocalProviderOptions)))
	mux.Handle("/local-provider/resolve", RequireBearer(a.cfg.BearerToken, http.HandlerFunc(a.handleLocalProviderResolve)))
	mux.Handle("/runtime/evidence/app-bridge", RequireBearer(a.cfg.BearerToken, http.HandlerFunc(a.handleAppBridgeEvidence)))
	mux.Handle("/runtime/capability-state", RequireBearer(a.cfg.BearerToken, http.HandlerFunc(a.handleCapabilityState)))
	mux.Handle("/context/set", RequireBearer(a.cfg.BearerToken, http.HandlerFunc(a.handleContextSet)))
	mux.Handle("/context/clear", RequireBearer(a.cfg.BearerToken, http.HandlerFunc(a.handleContextClear)))
	mux.Handle("/task/cancel", RequireBearer(a.cfg.BearerToken, http.HandlerFunc(a.handleTaskCancel)))
	mux.Handle("/task/choice", RequireBearer(a.cfg.BearerToken, http.HandlerFunc(a.handleTaskChoice)))
	mux.Handle("/task/resume_queue", RequireBearer(a.cfg.BearerToken, http.HandlerFunc(a.handleTaskResumeQueue)))
	mux.Handle("/thought/request", RequireBearer(a.cfg.BearerToken, http.HandlerFunc(a.handleThoughtRequest)))
	mux.Handle("/state", RequireBearer(a.cfg.BearerToken, http.HandlerFunc(a.handleState)))
	mux.Handle("/storage/stats", RequireBearer(a.cfg.BearerToken, http.HandlerFunc(a.handleStorageStats)))
	mux.Handle("/report/ai", RequireBearer(a.cfg.BearerToken, http.HandlerFunc(a.handleAIReport)))
	mux.Handle("/report/settings", RequireBearer(a.cfg.BearerToken, http.HandlerFunc(a.handleSettingsReport)))
	mux.Handle("/reset", RequireBearer(a.cfg.BearerToken, http.HandlerFunc(a.handleReset)))
	mux.Handle("/trophy/conjure", RequireBearer(a.cfg.BearerToken, http.HandlerFunc(a.stub("trophy conjure not yet implemented"))))
	mux.Handle("/inspector/open", RequireBearer(a.cfg.BearerToken, http.HandlerFunc(a.stub("inspector open not yet implemented"))))
	mux.Handle("/inspector/close", RequireBearer(a.cfg.BearerToken, http.HandlerFunc(a.stub("inspector close not yet implemented"))))
	mux.Handle("/ws", RequireWSBearer(a.cfg.BearerToken, http.HandlerFunc(a.hub.Handle)))

	server := &http.Server{
		Addr:    net.JoinHostPort(a.cfg.Host, strconv.Itoa(a.cfg.Port)),
		Handler: mux,
	}
	emitSidecarHTTPStartupProof("XMILO_SIDECAR_HTTP_LISTENER_BIND_ATTEMPT", "host", a.cfg.Host, "port", strconv.Itoa(a.cfg.Port), "address", server.Addr)
	listener, err := net.Listen("tcp", server.Addr)
	if err != nil {
		emitSidecarHTTPStartupProof("XMILO_SIDECAR_HTTP_LISTENER_BIND_FAILED", "error_class", classifySidecarHTTPStartupError(err))
		return err
	}
	emitSidecarHTTPStartupProof("XMILO_SIDECAR_HTTP_LISTENER_BOUND", "host", a.cfg.Host, "port", strconv.Itoa(a.cfg.Port), "address", server.Addr)

	a.hub.Broadcast("runtime.ready", map[string]any{
		"ready":            true,
		"wake_lock_active": a.wakeLockActive,
		"wake_lock_status": a.wakeLockStatus,
		"runtime_host":     "xmilo_owned",
	})

	go func() {
		<-ctx.Done()
		_ = server.Shutdown(context.Background())
		_ = a.store.Close()
	}()
	go a.runJWTRefresher(ctx)
	go func() { _ = a.ensureRelaySession(ctx) }()

	emitSidecarHTTPStartupProof("XMILO_SIDECAR_HTTP_SERVE_STARTED", "address", server.Addr)
	err = server.Serve(listener)
	emitSidecarHTTPStartupProof("XMILO_SIDECAR_HTTP_SERVE_EXITED", "error_class", classifySidecarHTTPStartupError(err))
	return err
}

func sidecarHTTPStartupProofLine(event string, fields ...string) string {
	var b strings.Builder
	b.WriteString(event)
	for i := 0; i+1 < len(fields); i += 2 {
		key := sanitizeSidecarHTTPStartupProofPart(fields[i])
		value := sanitizeSidecarHTTPStartupProofPart(fields[i+1])
		if key == "" || value == "" {
			continue
		}
		b.WriteByte(' ')
		b.WriteString(key)
		b.WriteByte('=')
		b.WriteString(value)
	}
	return b.String()
}

func emitSidecarHTTPStartupProof(event string, fields ...string) {
	fmt.Fprintln(os.Stdout, sidecarHTTPStartupProofLine(event, fields...))
}

func sanitizeSidecarHTTPStartupProofPart(value string) string {
	value = strings.TrimSpace(value)
	var b strings.Builder
	for _, r := range value {
		switch {
		case r >= 'a' && r <= 'z':
			b.WriteRune(r)
		case r >= 'A' && r <= 'Z':
			b.WriteRune(r)
		case r >= '0' && r <= '9':
			b.WriteRune(r)
		case r == '_' || r == '-' || r == '.' || r == ':' || r == '[' || r == ']':
			b.WriteRune(r)
		}
		if b.Len() >= 160 {
			break
		}
	}
	return b.String()
}

func classifySidecarHTTPStartupError(err error) string {
	if err == nil {
		return "none"
	}
	if errors.Is(err, http.ErrServerClosed) {
		return "server_closed"
	}
	text := strings.ToLower(err.Error())
	switch {
	case strings.Contains(text, "address already in use"):
		return "address_in_use"
	case strings.Contains(text, "permission denied"):
		return "permission_denied"
	case strings.Contains(text, "bind"):
		return "bind_failed"
	case strings.Contains(text, "listen"):
		return "listen_failed"
	case strings.Contains(text, "closed network connection"):
		return "network_closed"
	case strings.Contains(text, "timeout") || strings.Contains(text, "deadline exceeded"):
		return "timeout"
	default:
		return "other"
	}
}

// ─── Proactive JWT refresh ────────────────────────────────────────────────────

func (a *App) runJWTRefresher(ctx context.Context) {
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			a.maybeRefreshJWT(ctx)
		}
	}
}

func (a *App) maybeRefreshJWT(ctx context.Context) {
	raw, _ := a.store.GetRuntimeConfig("relay_expires_at")
	if raw == "" {
		return
	}
	expiresAt, err := time.Parse(time.RFC3339, raw)
	if err != nil {
		return
	}
	if time.Until(expiresAt) > 10*time.Minute {
		return // still fresh
	}
	newJWT, newExpiry, err := a.relayClient.Refresh(ctx)
	if err != nil {
		return // will retry next tick
	}
	_ = a.store.SetRuntimeConfig("relay_session_jwt", newJWT)
	_ = a.store.SetRuntimeConfig("relay_expires_at", newExpiry)
}

const (
	authSessionRelayUnreachable = "relay_unreachable"
	authSessionRelayHTTPError   = "relay_http_error"
	authSessionBadResponse      = "relay_bad_response"
	authSessionEmptyJWT         = "relay_empty_jwt"
	authSessionConfigMissing    = "relay_config_missing"
	authSessionStoreFailed      = "relay_session_store_failed"
	authSessionUnknownFailure   = "relay_session_unknown_failure"
)

type authSessionError struct {
	class  string
	status int
}

func (e authSessionError) Error() string {
	if e.class == "" {
		return authSessionUnknownFailure
	}
	return e.class
}

func authSessionErrorClass(err error) string {
	var sessionErr authSessionError
	if errors.As(err, &sessionErr) && sessionErr.class != "" {
		return sessionErr.class
	}
	return authSessionUnknownFailure
}

func authSessionErrorStatus(err error) string {
	var sessionErr authSessionError
	if errors.As(err, &sessionErr) && sessionErr.status > 0 {
		return strconv.Itoa(sessionErr.status)
	}
	return ""
}

func (a *App) ensureRelaySession(ctx context.Context) error {
	sessionJWT, err := a.store.GetRuntimeConfig("relay_session_jwt")
	if err != nil {
		return authSessionError{class: authSessionStoreFailed}
	}
	expiresRaw, err := a.store.GetRuntimeConfig("relay_expires_at")
	if err != nil {
		return authSessionError{class: authSessionStoreFailed}
	}
	if strings.TrimSpace(sessionJWT) != "" {
		if expiresRaw != "" {
			expiresAt, err := time.Parse(time.RFC3339, expiresRaw)
			if err == nil && time.Until(expiresAt) > time.Minute {
				return nil
			}
		}
		refreshCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
		defer cancel()
		newJWT, newExpiry, err := a.relayClient.Refresh(refreshCtx)
		if err == nil {
			if err := a.store.SetRuntimeConfig("relay_session_jwt", newJWT); err != nil {
				return authSessionError{class: authSessionStoreFailed}
			}
			if err := a.store.SetRuntimeConfig("relay_expires_at", newExpiry); err != nil {
				return authSessionError{class: authSessionStoreFailed}
			}
			return nil
		}
	}
	bootstrapCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	return bootstrapRelaySession(bootstrapCtx, a.cfg, a.store)
}

// ─── Auth passthrough handlers ────────────────────────────────────────────────

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
	if err := a.relayClient.Register(r.Context(), req.Email, req.DeviceUserID); err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]any{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

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
	if err := a.relayClient.RedeemInvite(r.Context(), req.Code, req.DeviceUserID); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": err.Error()})
		return
	}

	// Redeeming access updates entitlement in the relay DB, but the sidecar may still
	// hold a stale JWT until the background refresher wakes up. Refresh immediately so
	// the app can enter the main hall and submit a real task without a false 403 gap.
	newJWT, newExpiry, err := a.relayClient.Refresh(r.Context())
	if err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]any{"error": err.Error()})
		return
	}
	_ = a.store.SetRuntimeConfig("relay_session_jwt", newJWT)
	_ = a.store.SetRuntimeConfig("relay_expires_at", newExpiry)
	_ = a.store.SetRuntimeConfig("relay_device_user_id", jwtStringClaim(newJWT, "sub"))

	writeJSON(w, http.StatusOK, map[string]any{
		"ok":                     true,
		"device_user_id":         jwtStringClaim(newJWT, "sub"),
		"entitled":               jwtEntitledClaim(newJWT),
		"expires_at":             newExpiry,
		"access_mode":            jwtStringClaim(newJWT, "access_mode"),
		"access_code_only":       jwtBoolClaim(newJWT, "access_code_only"),
		"trial_allowed":          jwtBoolClaim(newJWT, "trial_allowed"),
		"subscription_allowed":   jwtBoolClaim(newJWT, "subscription_allowed"),
		"access_code_grant_days": jwtIntClaim(newJWT, "access_code_grant_days"),
		"verified_email":         jwtStringClaim(newJWT, "verified_email"),
		"email_verified":         jwtBoolClaim(newJWT, "email_verified"),
		"two_factor_enabled":     jwtBoolClaim(newJWT, "two_factor_enabled"),
		"two_factor_ok":          jwtBoolClaim(newJWT, "two_factor_ok"),
		"website_handoff_ready":  jwtBoolClaim(newJWT, "website_handoff_ready"),
	})
}

func (a *App) handleAuthCheck(w http.ResponseWriter, r *http.Request) {
	if a.cfg.LocalBYOKActive() {
		writeJSON(w, http.StatusOK, a.localAccessStatus())
		return
	}
	emitSidecarHTTPStartupProof("XMILO_SIDECAR_AUTH_CHECK_SESSION_ENSURE_STARTED")
	if err := a.ensureRelaySession(r.Context()); err != nil {
		class := authSessionErrorClass(err)
		emitSidecarHTTPStartupProof("XMILO_SIDECAR_AUTH_CHECK_SESSION_ENSURE_FAILED", "error_class", class)
		writeJSON(w, http.StatusBadGateway, map[string]any{"error": "auth_check_failed", "error_class": class, "entitled": false})
		return
	}
	sessionJWT, err := a.store.GetRuntimeConfig("relay_session_jwt")
	if err != nil {
		class := authSessionStoreFailed
		emitSidecarHTTPStartupProof("XMILO_SIDECAR_AUTH_CHECK_SESSION_ENSURE_FAILED", "error_class", class)
		writeJSON(w, http.StatusBadGateway, map[string]any{"error": "auth_check_failed", "error_class": class, "entitled": false})
		return
	}
	expiresAt, err := a.store.GetRuntimeConfig("relay_expires_at")
	if err != nil {
		class := authSessionStoreFailed
		emitSidecarHTTPStartupProof("XMILO_SIDECAR_AUTH_CHECK_SESSION_ENSURE_FAILED", "error_class", class)
		writeJSON(w, http.StatusBadGateway, map[string]any{"error": "auth_check_failed", "error_class": class, "entitled": false})
		return
	}
	if strings.TrimSpace(sessionJWT) == "" {
		class := authSessionEmptyJWT
		emitSidecarHTTPStartupProof("XMILO_SIDECAR_AUTH_CHECK_SESSION_ENSURE_FAILED", "error_class", class)
		writeJSON(w, http.StatusBadGateway, map[string]any{"error": "auth_check_failed", "error_class": class, "entitled": false})
		return
	}
	emitSidecarHTTPStartupProof("XMILO_SIDECAR_AUTH_CHECK_SESSION_ENSURE_READY")

	writeJSON(w, http.StatusOK, map[string]any{
		"ok":                     true,
		"device_user_id":         jwtStringClaim(sessionJWT, "sub"),
		"entitled":               jwtEntitledClaim(sessionJWT),
		"expires_at":             expiresAt,
		"access_mode":            jwtStringClaim(sessionJWT, "access_mode"),
		"access_code_only":       jwtBoolClaim(sessionJWT, "access_code_only"),
		"trial_allowed":          jwtBoolClaim(sessionJWT, "trial_allowed"),
		"subscription_allowed":   jwtBoolClaim(sessionJWT, "subscription_allowed"),
		"access_code_grant_days": jwtIntClaim(sessionJWT, "access_code_grant_days"),
		"verified_email":         jwtStringClaim(sessionJWT, "verified_email"),
		"email_verified":         jwtBoolClaim(sessionJWT, "email_verified"),
		"two_factor_enabled":     jwtBoolClaim(sessionJWT, "two_factor_enabled"),
		"two_factor_ok":          jwtBoolClaim(sessionJWT, "two_factor_ok"),
		"website_handoff_ready":  jwtBoolClaim(sessionJWT, "website_handoff_ready"),
	})
}

func (a *App) handleTwoFactorStatus(w http.ResponseWriter, r *http.Request) {
	result, err := a.relayClient.TwoFactorStatus(r.Context())
	if err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]any{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (a *App) handleTwoFactorBegin(w http.ResponseWriter, r *http.Request) {
	result, err := a.relayClient.TwoFactorBegin(r.Context())
	if err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]any{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (a *App) handleTwoFactorConfirm(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Code string `json:"code"`
	}
	if err := decodeJSON(r, &req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": err.Error()})
		return
	}
	result, err := a.relayClient.TwoFactorConfirm(r.Context(), req.Code)
	if err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]any{"error": err.Error()})
		return
	}
	if session, _ := result["session"].(map[string]any); session != nil {
		if sessionJWT, _ := session["session_jwt"].(string); sessionJWT != "" {
			_ = a.store.SetRuntimeConfig("relay_session_jwt", sessionJWT)
		}
		if expiresAt, _ := session["expires_at"].(string); expiresAt != "" {
			_ = a.store.SetRuntimeConfig("relay_expires_at", expiresAt)
		}
	}
	writeJSON(w, http.StatusOK, result)
}

func (a *App) handleTwoFactorVerify(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Code         string `json:"code"`
		RecoveryCode string `json:"recovery_code"`
	}
	if err := decodeJSON(r, &req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": err.Error()})
		return
	}
	result, err := a.relayClient.TwoFactorVerify(r.Context(), req.Code, req.RecoveryCode)
	if err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]any{"error": err.Error()})
		return
	}
	if session, _ := result["session"].(map[string]any); session != nil {
		if sessionJWT, _ := session["session_jwt"].(string); sessionJWT != "" {
			_ = a.store.SetRuntimeConfig("relay_session_jwt", sessionJWT)
		}
		if expiresAt, _ := session["expires_at"].(string); expiresAt != "" {
			_ = a.store.SetRuntimeConfig("relay_expires_at", expiresAt)
		}
	}
	writeJSON(w, http.StatusOK, result)
}

func (a *App) handleTwoFactorDisable(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Code         string `json:"code"`
		RecoveryCode string `json:"recovery_code"`
	}
	if err := decodeJSON(r, &req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": err.Error()})
		return
	}
	result, err := a.relayClient.TwoFactorDisable(r.Context(), req.Code, req.RecoveryCode)
	if err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]any{"error": err.Error()})
		return
	}
	if session, _ := result["session"].(map[string]any); session != nil {
		if sessionJWT, _ := session["session_jwt"].(string); sessionJWT != "" {
			_ = a.store.SetRuntimeConfig("relay_session_jwt", sessionJWT)
		}
		if expiresAt, _ := session["expires_at"].(string); expiresAt != "" {
			_ = a.store.SetRuntimeConfig("relay_expires_at", expiresAt)
		}
	}
	writeJSON(w, http.StatusOK, result)
}

func (a *App) handleTwoFactorRecoveryRegenerate(w http.ResponseWriter, r *http.Request) {
	result, err := a.relayClient.TwoFactorRegenerateRecoveryCodes(r.Context())
	if err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]any{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (a *App) handleWebsiteHandoffCreate(w http.ResponseWriter, r *http.Request) {
	result, err := a.relayClient.CreateWebsiteHandoff(r.Context())
	if err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]any{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, result)
}

// ─── Existing handlers (unchanged from v7) ────────────────────────────────────

func (a *App) handleHealth(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, runtime.HealthResponse{
		OK:        true,
		Service:   "xmilo-sidecar",
		Version:   buildinfo.Version,
		UptimeSec: int64(time.Since(a.startedAt).Seconds()),
	})
}

func (a *App) handleReady(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, runtime.ReadyResponse{
		OK:                    true,
		WakeLockActive:        a.wakeLockActive,
		WakeLockStatus:        a.wakeLockStatus,
		RuntimeHost:           "xmilo_owned",
		DBReady:               true,
		RelayConfigured:       a.cfg.RelayBaseURL != "",
		MindLoaded:            a.mindLoaded,
		RuntimeID:             a.cfg.RuntimeID,
		LLMMode:               a.cfg.LLMMode,
		BYOKProvider:          a.cfg.BYOKProvider,
		SubscriptionEntitled:  false,
		BringYourOwnKeyActive: a.localBYOKKeyConfigured(),
		Phase9APIKeyAccess:    a.localBYOKKeyConfigured(),
		FirstTaskEligible:     a.localBYOKKeyConfigured(),
		RelayLLMTurnAllowed:   !a.cfg.LocalBYOKActive(),
		LocalLLMTurnAllowed:   a.cfg.LocalBYOKActive() && a.localBYOKKeyConfigured(),
	})
}

func (a *App) handleBootstrap(w http.ResponseWriter, r *http.Request) {
	version, _ := a.store.SchemaVersion()
	writeJSON(w, http.StatusOK, map[string]any{
		"ok":             true,
		"runtime_id":     a.cfg.RuntimeID,
		"schema_version": version,
		"mind_root":      filepath.Clean(a.cfg.MindRoot),
	})
}

func (a *App) handleAuthRefresh(w http.ResponseWriter, r *http.Request) {
	var req struct {
		SessionJWT string `json:"session_jwt"`
		ExpiresAt  string `json:"expires_at"`
	}
	if err := decodeJSON(r, &req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": err.Error()})
		return
	}
	_ = a.store.SetRuntimeConfig("relay_session_jwt", req.SessionJWT)
	_ = a.store.SetRuntimeConfig("relay_expires_at", req.ExpiresAt)
	_ = a.store.SetRuntimeConfig("relay_device_user_id", jwtStringClaim(req.SessionJWT, "sub"))
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (a *App) handleAuthLogout(w http.ResponseWriter, r *http.Request) {
	_ = a.store.SetRuntimeConfig("relay_session_jwt", "")
	_ = a.store.SetRuntimeConfig("relay_expires_at", "")
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (a *App) handleAuthDeleteAccount(w http.ResponseWriter, r *http.Request) {
	if err := a.relayClient.DeleteAccount(r.Context()); err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]any{"error": err.Error()})
		return
	}
	_ = a.store.SetRuntimeConfig("relay_session_jwt", "")
	_ = a.store.SetRuntimeConfig("relay_expires_at", "")
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (a *App) handleTaskStart(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Prompt string `json:"prompt"`
	}
	if err := decodeJSON(r, &req); err != nil || req.Prompt == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "prompt required"})
		return
	}
	if a.cfg.LocalBYOKActive() {
		if !a.localBYOKKeyConfigured() {
			writeJSON(w, http.StatusForbidden, map[string]any{
				"error":      "missing_local_provider_key",
				"error_code": "missing_local_provider_key",
			})
			return
		}
	} else {
		if sessionJWT, _ := a.store.GetRuntimeConfig("relay_session_jwt"); strings.TrimSpace(sessionJWT) == "" {
			writeJSON(w, http.StatusForbidden, map[string]any{
				"error":      "entitlement_lost",
				"error_code": "entitlement_lost",
			})
			return
		}
	}
	snap, assessment, err := a.engine.StartTask(r.Context(), req.Prompt)
	if err != nil {
		writeJSON(w, http.StatusConflict, map[string]any{
			"error":       err.Error(),
			"error_code":  taskStartErrorCode(err, assessment),
			"intake_gate": assessment,
		})
		return
	}
	writeJSON(w, http.StatusAccepted, map[string]any{"task_id": snap.TaskID, "attempt_id": snap.AttemptID, "immediate_state": snap, "intake_gate": assessment})
}

func (a *App) handleCommandSubmit(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Prompt string `json:"prompt"`
	}
	if err := decodeJSON(r, &req); err != nil || req.Prompt == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "prompt required"})
		return
	}
	if a.cfg.LocalBYOKActive() {
		if !a.localBYOKKeyConfigured() {
			writeJSON(w, http.StatusForbidden, map[string]any{
				"error":      "missing_local_provider_key",
				"error_code": "missing_local_provider_key",
			})
			return
		}
	} else {
		if sessionJWT, _ := a.store.GetRuntimeConfig("relay_session_jwt"); strings.TrimSpace(sessionJWT) == "" {
			writeJSON(w, http.StatusForbidden, map[string]any{
				"error":      "entitlement_lost",
				"error_code": "entitlement_lost",
			})
			return
		}
	}

	current := a.engine.Snapshot()
	if plan, ok := a.planner.Plan(req.Prompt, current.CurrentRoomID, current.MiloState); ok {
		if err := a.engine.ExecuteMovementIntent(plan); err != nil {
			writeJSON(w, http.StatusConflict, map[string]any{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusAccepted, map[string]any{
			"handled": true,
			"kind":    "movement",
			"plan":    plan,
		})
		return
	}

	snap, assessment, err := a.engine.StartTask(r.Context(), req.Prompt)
	if err != nil {
		writeJSON(w, http.StatusConflict, map[string]any{
			"error":       err.Error(),
			"error_code":  taskStartErrorCode(err, assessment),
			"intake_gate": assessment,
		})
		return
	}
	writeJSON(w, http.StatusAccepted, map[string]any{
		"handled":         true,
		"kind":            "task",
		"task_id":         snap.TaskID,
		"attempt_id":      snap.AttemptID,
		"immediate_state": snap,
		"intake_gate":     assessment,
	})
}

func taskStartErrorCode(err error, assessment *runtime.IntakeAssessment) string {
	if err == nil {
		return ""
	}
	msg := strings.TrimSpace(err.Error())
	if strings.Contains(msg, "active task already running") {
		return "ACTIVE_TASK_RUNNING"
	}
	if assessment != nil && strings.TrimSpace(assessment.ChosenClosedAction) != "" {
		return "INTAKE_" + strings.TrimSpace(assessment.ChosenClosedAction)
	}
	return "TASK_START_FAILED"
}

func (a *App) handleTaskCurrent(w http.ResponseWriter, r *http.Request) {
	task, err := a.store.GetTask("active")
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}
	if task == nil {
		writeJSON(w, http.StatusOK, map[string]any{"task": nil})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"task": task})
}

func (a *App) handleTaskInterrupt(w http.ResponseWriter, r *http.Request) {
	if err := a.engine.InterruptTask("interrupt"); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (a *App) handleContextSet(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "method not allowed"})
		return
	}
	var req contextpolicy.SetRequest
	if err := decodeJSON(r, &req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": err.Error()})
		return
	}
	stored, err := contextpolicy.Normalize(req, time.Now().UTC())
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": err.Error()})
		return
	}
	if err := a.store.SetRuntimeConfig("active_context", stored.Content); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}
	if err := a.store.SetRuntimeConfig("active_context_meta", contextpolicy.MetadataJSON(stored.Meta)); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "context": stored.Meta})
}

func (a *App) handleContextClear(w http.ResponseWriter, r *http.Request) {
	if err := a.store.SetRuntimeConfig("active_context", ""); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}
	if err := a.store.SetRuntimeConfig("active_context_meta", ""); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (a *App) handleLocalProviderOptions(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "method not allowed"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"ok":        true,
		"providers": providerpolicy.Options(),
	})
}

func (a *App) handleLocalProviderResolve(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "method not allowed"})
		return
	}
	var req struct {
		Provider     string `json:"provider"`
		BaseURL      string `json:"base_url"`
		Model        string `json:"model"`
		KeyFileReady bool   `json:"key_file_ready"`
		HasAPIKey    bool   `json:"has_api_key"`
	}
	if err := decodeJSON(r, &req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": err.Error()})
		return
	}
	resolved, err := providerpolicy.Resolve(providerpolicy.ResolveInput{
		Provider:     req.Provider,
		BaseURL:      req.BaseURL,
		Model:        req.Model,
		KeyFileReady: req.KeyFileReady,
		HasAPIKey:    req.HasAPIKey,
	})
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": err.Error(), "resolved": resolved})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "resolved": resolved})
}

const maxAppBridgeEvidencePayloadBytes = 16 * 1024

func (a *App) handleAppBridgeEvidence(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "method not allowed"})
		return
	}
	if a.engine == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]any{"error": "runtime_engine_unavailable"})
		return
	}

	var evidence runtime.AppBridgeEvidence
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxAppBridgeEvidencePayloadBytes))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&evidence); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid_app_bridge_evidence:" + err.Error()})
		return
	}
	if err := validateAppBridgeEvidence(evidence, time.Now().UTC()); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": err.Error()})
		return
	}

	stored, err := a.engine.RecordAppBridgeEvidence(evidence)
	if err != nil {
		status := http.StatusConflict
		if err.Error() == "app_bridge_evidence_no_active_task" {
			status = http.StatusConflict
		}
		writeJSON(w, status, map[string]any{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "evidence": stored})
}

func validateAppBridgeEvidence(evidence runtime.AppBridgeEvidence, now time.Time) error {
	if evidence.ProofClass != "app_bridge_verified" {
		return fmt.Errorf("invalid_app_bridge_proof_class")
	}
	if evidence.Source != "android_bridge" {
		return fmt.Errorf("invalid_app_bridge_source")
	}
	if !tasks.IsAllowedAppBridgeEvidenceOperation(evidence.Operation) {
		return fmt.Errorf("invalid_app_bridge_operation")
	}
	checkedAt, err := time.Parse(time.RFC3339, strings.TrimSpace(evidence.CheckedAt))
	if err != nil {
		return fmt.Errorf("invalid_app_bridge_checked_at")
	}
	if checkedAt.After(now.Add(time.Minute)) || now.Sub(checkedAt) > tasks.AppBridgeEvidenceFreshness {
		return fmt.Errorf("stale_app_bridge_evidence")
	}
	if strings.TrimSpace(evidence.Summary) == "" || len(evidence.Summary) > 240 {
		return fmt.Errorf("invalid_app_bridge_summary")
	}
	if evidence.Verified {
		if strings.TrimSpace(evidence.BlockingReason) != "" {
			return fmt.Errorf("verified_app_bridge_evidence_must_not_have_blocking_reason")
		}
	} else if strings.TrimSpace(evidence.BlockingReason) == "" {
		return fmt.Errorf("failed_app_bridge_evidence_requires_blocking_reason")
	}
	if containsSensitiveText(evidence.Summary) ||
		containsSensitiveText(evidence.BlockingReason) ||
		containsSensitiveText(evidence.EvidenceID) ||
		containsSensitiveText(evidence.AttemptID) ||
		containsSensitiveText(evidence.TaskID) {
		return fmt.Errorf("app_bridge_evidence_contains_sensitive_text")
	}
	if err := validateAppBridgeEvidenceDetails(evidence.Details, 0); err != nil {
		return err
	}
	return nil
}

func validateAppBridgeEvidenceDetails(details map[string]any, depth int) error {
	if len(details) > 32 {
		return fmt.Errorf("app_bridge_evidence_details_too_large")
	}
	if depth > 2 {
		return fmt.Errorf("app_bridge_evidence_details_too_deep")
	}
	for key, value := range details {
		if isSensitiveFieldName(key) {
			return fmt.Errorf("app_bridge_evidence_sensitive_detail:%s", key)
		}
		if len(key) > 64 {
			return fmt.Errorf("app_bridge_evidence_detail_key_too_long")
		}
		switch typed := value.(type) {
		case string:
			if len(typed) > 240 || containsSensitiveText(typed) {
				return fmt.Errorf("app_bridge_evidence_sensitive_or_oversized_detail:%s", key)
			}
		case bool, float64, nil:
		case map[string]any:
			if err := validateAppBridgeEvidenceDetails(typed, depth+1); err != nil {
				return err
			}
		case []any:
			if len(typed) > 16 {
				return fmt.Errorf("app_bridge_evidence_detail_array_too_large:%s", key)
			}
			for _, item := range typed {
				if text, ok := item.(string); ok {
					if len(text) > 120 || containsSensitiveText(text) {
						return fmt.Errorf("app_bridge_evidence_sensitive_or_oversized_detail:%s", key)
					}
				}
			}
		default:
			return fmt.Errorf("app_bridge_evidence_unsupported_detail:%s", key)
		}
	}
	return nil
}

func isSensitiveFieldName(key string) bool {
	normalized := strings.NewReplacer("-", "_", " ", "_").Replace(strings.ToLower(strings.TrimSpace(key)))
	return strings.Contains(normalized, "api_key") ||
		strings.Contains(normalized, "apikey") ||
		strings.Contains(normalized, "bearer") ||
		strings.Contains(normalized, "token") ||
		strings.Contains(normalized, "secret") ||
		strings.Contains(normalized, "password") ||
		strings.Contains(normalized, "provider_key") ||
		strings.Contains(normalized, "raw_key") ||
		strings.Contains(normalized, "key_file_path") ||
		strings.HasSuffix(normalized, "_path")
}

func containsSensitiveText(value string) bool {
	lower := strings.ToLower(strings.TrimSpace(value))
	return strings.Contains(lower, "bearer ") ||
		strings.Contains(lower, "api_key") ||
		strings.Contains(lower, "authorization:") ||
		strings.Contains(lower, "xai-") ||
		strings.Contains(lower, "anthropic-")
}

func (a *App) handleTaskCancel(w http.ResponseWriter, r *http.Request) {
	if err := a.engine.InterruptTask("cancel"); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (a *App) handleTaskChoice(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "method not allowed"})
		return
	}
	var req struct {
		TaskID   string `json:"task_id"`
		Choice   string `json:"choice"`
		Decision string `json:"decision"`
	}
	if err := decodeJSON(r, &req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": err.Error()})
		return
	}
	state, assessment, err := a.engine.RecordChoice(req.TaskID, req.Choice, req.Decision)
	if err != nil {
		writeJSON(w, http.StatusConflict, map[string]any{"error": err.Error(), "intake_gate": assessment})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "approval_state": state, "intake_gate": assessment})
}

func (a *App) handleTaskResumeQueue(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "method not allowed"})
		return
	}
	task, assessment, err := a.engine.ResumePending(r.Context())
	if err != nil {
		writeJSON(w, http.StatusConflict, map[string]any{"error": err.Error(), "intake_gate": assessment})
		return
	}
	writeJSON(w, http.StatusAccepted, map[string]any{"ok": true, "task": task, "intake_gate": assessment})
}

func (a *App) handleThoughtRequest(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, a.engine.ThoughtRequest())
}

func (a *App) handleState(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, a.engine.Snapshot())
}

func (a *App) handleCapabilityState(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		var state map[string]any
		_ = a.store.GetRuntimeConfigJSON("capability_state_snapshot", &state)
		if len(state) == 0 {
			writeJSON(w, http.StatusOK, map[string]any{"ok": true, "capability_state": nil})
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"ok": true, "capability_state": state})
	case http.MethodPost:
		var state map[string]any
		if err := decodeJSON(r, &state); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]any{"error": err.Error()})
			return
		}
		safeState := sanitizeCapabilityState(state)
		if err := a.store.SetRuntimeConfigJSON("capability_state_snapshot", safeState); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
			return
		}
		a.hub.Broadcast("runtime.capability_state", map[string]any{
			"status":           "updated",
			"checked_at":       safeState["checked_at"],
			"capability_count": capabilityCount(safeState),
		})
		writeJSON(w, http.StatusOK, map[string]any{"ok": true, "capability_state": safeState})
	default:
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "method not allowed"})
	}
}

func (a *App) handleStorageStats(w http.ResponseWriter, r *http.Request) {
	stats, err := a.store.StorageStats(a.cfg.DBPath)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, stats)
}

func (a *App) handleAIReport(w http.ResponseWriter, r *http.Request) {
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
		ModelName    string `json:"model_name"`
	}
	if err := decodeJSON(r, &req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": err.Error()})
		return
	}
	if err := a.relayClient.SubmitAIReport(r.Context(), map[string]any{
		"task_id":       req.TaskID,
		"event_type":    req.EventType,
		"report_reason": req.ReportReason,
		"report_note":   req.ReportNote,
		"output_text":   req.OutputText,
		"prompt_text":   req.PromptText,
		"runtime_id":    a.cfg.RuntimeID,
		"model_name":    req.ModelName,
	}); err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]any{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

const maxSettingsReportBodyBytes int64 = 1 << 20

func (a *App) handleSettingsReport(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "method not allowed"})
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, maxSettingsReportBodyBytes)
	defer r.Body.Close()
	var payload map[string]any
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&payload); err != nil {
		var maxErr *http.MaxBytesError
		if errors.As(err, &maxErr) {
			writeJSON(w, http.StatusRequestEntityTooLarge, map[string]any{"error": "settings report exceeds 1 MiB limit"})
			return
		}
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": err.Error()})
		return
	}
	if payload == nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "settings report must be a JSON object"})
		return
	}
	payload["runtime_id"] = a.cfg.RuntimeID
	payload["sidecar_version"] = buildinfo.Version
	reportCtx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()
	if err := a.ensureRelaySession(reportCtx); err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]any{"error": err.Error()})
		return
	}
	proof, statusCode, err := a.relayClient.SubmitSettingsReport(reportCtx, payload)
	if err != nil {
		if statusCode == 0 {
			statusCode = http.StatusBadGateway
		}
		writeJSON(w, statusCode, map[string]any{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, proof)
}

func (a *App) handleReset(w http.ResponseWriter, r *http.Request) {
	task, err := a.store.GetTask("active")
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}
	if task != nil {
		writeJSON(w, http.StatusConflict, map[string]any{"ok": false, "reason": "task_active"})
		return
	}
	var req struct {
		Tier string `json:"tier"`
	}
	if err := decodeJSON(r, &req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": err.Error()})
		return
	}
	switch req.Tier {
	case "chat_cache_only":
		_, _ = a.store.DB.Exec(`DELETE FROM conversation_tail`)
	case "archive_only":
		_, _ = a.store.DB.Exec(`DELETE FROM task_history`)
	default:
		writeJSON(w, http.StatusNotImplemented, map[string]any{"ok": false, "error": "reset tier not implemented yet"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (a *App) stub(message string) func(http.ResponseWriter, *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusNotImplemented, map[string]any{"ok": false, "error": message})
	}
}

func (a *App) markWakeLockUnsupported() {
	a.wakeLockActive = false
	a.wakeLockStatus = "unsupported_by_sidecar_runtime_host"
}

// ─── Helpers ──────────────────────────────────────────────────────────────────

func (a *App) localBYOKKeyConfigured() bool {
	return llm.LocalProviderReady(a.cfg)
}

func (a *App) localAccessStatus() map[string]any {
	active := a.localBYOKKeyConfigured()
	return map[string]any{
		"ok":                        true,
		"entitled":                  false,
		"subscription_entitled":     false,
		"access_mode":               "local_byok",
		"byok_provider":             a.cfg.BYOKProvider,
		"llm_mode":                  a.cfg.LLMMode,
		"bring_your_own_key_active": active,
		"phase9_api_key_access":     active,
		"first_task_eligible":       active,
		"relay_llm_turn_allowed":    false,
		"local_llm_turn_allowed":    active,
	}
}

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

func sanitizeCapabilityState(input map[string]any) map[string]any {
	output := map[string]any{
		"schema_version": 1,
		"checked_at":     sanitizeShortString(fmt.Sprint(input["checked_at"])),
	}
	if output["checked_at"] == "" {
		output["checked_at"] = time.Now().UTC().Format(time.RFC3339)
	}

	runtimeHost, _ := input["runtime_host"].(map[string]any)
	output["runtime_host"] = map[string]any{
		"online":  boolValue(runtimeHost["online"]),
		"version": sanitizeShortString(fmt.Sprint(runtimeHost["version"])),
		"health":  sanitizeShortString(fmt.Sprint(runtimeHost["health"])),
	}

	capabilities := map[string]any{}
	if rawCapabilities, ok := input["capabilities"].(map[string]any); ok {
		for name, rawCapability := range rawCapabilities {
			capability, ok := rawCapability.(map[string]any)
			if !ok {
				continue
			}
			key := sanitizeCapabilityKey(name)
			if key == "" {
				continue
			}
			capabilities[key] = map[string]any{
				"declared":           optionalBool(capability["declared"]),
				"requested":          optionalBool(capability["requested"]),
				"granted":            optionalBool(capability["granted"]),
				"available":          optionalBool(capability["available"]),
				"tool_available":     optionalBool(capability["tool_available"]),
				"tested":             optionalBool(capability["tested"]),
				"last_verified_at":   sanitizeShortString(fmt.Sprint(capability["last_verified_at"])),
				"failure_stage":      sanitizeShortString(fmt.Sprint(capability["failure_stage"])),
				"grant_scope":        sanitizeShortString(fmt.Sprint(capability["grant_scope"])),
				"location_accuracy":  sanitizeShortString(fmt.Sprint(capability["location_accuracy"])),
				"media_access":       sanitizeShortString(fmt.Sprint(capability["media_access"])),
				"accepted_for_setup": optionalBool(capability["accepted_for_setup"]),
				"repair_hint":        sanitizeShortString(fmt.Sprint(capability["repair_hint"])),
				"note":               sanitizeShortString(fmt.Sprint(capability["note"])),
			}
		}
	}
	output["capabilities"] = capabilities
	return output
}

func capabilityCount(state map[string]any) int {
	capabilities, _ := state["capabilities"].(map[string]any)
	return len(capabilities)
}

func sanitizeCapabilityKey(value string) string {
	var b strings.Builder
	for _, r := range value {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9', r == '_', r == '-', r == ':':
			b.WriteRune(r)
		case r >= 'A' && r <= 'Z':
			b.WriteRune(r + ('a' - 'A'))
		default:
			b.WriteRune('_')
		}
		if b.Len() >= 64 {
			break
		}
	}
	return strings.Trim(b.String(), "_")
}

func sanitizeShortString(value string) string {
	value = strings.TrimSpace(value)
	if value == "<nil>" {
		return ""
	}
	replacer := strings.NewReplacer("\r", " ", "\n", " ", "\t", " ")
	clean := replacer.Replace(value)
	if len(clean) > 240 {
		return clean[:240]
	}
	return clean
}

func optionalBool(value any) any {
	if v, ok := value.(bool); ok {
		return v
	}
	return nil
}

func boolValue(value any) bool {
	v, _ := value.(bool)
	return v
}

// jwtEntitledClaim extracts the entitled bool from a JWT without signature verification.
// We trust the relay issued it correctly; this is only used for the /auth/check response.
func jwtEntitledClaim(token string) bool {
	claims := jwtClaims(token)
	entitled, _ := claims["entitled"].(bool)
	return entitled
}

func jwtStringClaim(token, key string) string {
	claims := jwtClaims(token)
	value, _ := claims[key].(string)
	return value
}

func jwtBoolClaim(token, key string) bool {
	claims := jwtClaims(token)
	value, _ := claims[key].(bool)
	return value
}

func jwtIntClaim(token, key string) int {
	claims := jwtClaims(token)
	switch value := claims[key].(type) {
	case float64:
		return int(value)
	case int:
		return value
	default:
		return 0
	}
}

func jwtClaims(token string) map[string]any {
	parts := strings.SplitN(token, ".", 3)
	if len(parts) != 3 {
		return map[string]any{}
	}
	payload := parts[1]
	// Pad to multiple of 4 for standard base64
	for len(payload)%4 != 0 {
		payload += "="
	}
	data, err := base64.URLEncoding.DecodeString(payload)
	if err != nil {
		return map[string]any{}
	}
	var claims map[string]any
	if err := json.Unmarshal(data, &claims); err != nil {
		return map[string]any{}
	}
	return claims
}

func bootstrapRelaySession(ctx context.Context, cfg config.Config, store *db.Store) error {
	if strings.TrimSpace(cfg.RelayBaseURL) == "" {
		err := authSessionError{class: authSessionConfigMissing}
		emitSidecarHTTPStartupProof("XMILO_SIDECAR_RELAY_SESSION_BOOTSTRAP_FAILED", "error_class", err.class)
		return err
	}
	emitSidecarHTTPStartupProof("XMILO_SIDECAR_RELAY_SESSION_BOOTSTRAP_STARTED")
	reqBody, _ := json.Marshal(map[string]string{"device_name": "xmilo-sidecar"})
	httpClient := netutil.NewResilientHTTPClient(15 * time.Second)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, cfg.RelayBaseURL+"/session/start", bytes.NewReader(reqBody))
	if err != nil {
		sessionErr := authSessionError{class: authSessionConfigMissing}
		emitSidecarHTTPStartupProof("XMILO_SIDECAR_RELAY_SESSION_BOOTSTRAP_FAILED", "error_class", sessionErr.class)
		return sessionErr
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := httpClient.Do(req)
	if err != nil {
		sessionErr := authSessionError{class: authSessionRelayUnreachable}
		emitSidecarHTTPStartupProof("XMILO_SIDECAR_RELAY_SESSION_BOOTSTRAP_FAILED", "error_class", sessionErr.class)
		return sessionErr
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		sessionErr := authSessionError{class: authSessionBadResponse, status: resp.StatusCode}
		emitSidecarHTTPStartupProof("XMILO_SIDECAR_RELAY_SESSION_BOOTSTRAP_FAILED", "error_class", sessionErr.class, "status", authSessionErrorStatus(sessionErr))
		return sessionErr
	}
	if resp.StatusCode >= 400 {
		sessionErr := authSessionError{class: authSessionRelayHTTPError, status: resp.StatusCode}
		emitSidecarHTTPStartupProof("XMILO_SIDECAR_RELAY_SESSION_BOOTSTRAP_FAILED", "error_class", sessionErr.class, "status", authSessionErrorStatus(sessionErr))
		return sessionErr
	}
	var out struct {
		DeviceUserID string `json:"device_user_id"`
		SessionJWT   string `json:"session_jwt"`
		ExpiresAt    string `json:"expires_at"`
	}
	if err := json.Unmarshal(raw, &out); err != nil {
		sessionErr := authSessionError{class: authSessionBadResponse, status: resp.StatusCode}
		emitSidecarHTTPStartupProof("XMILO_SIDECAR_RELAY_SESSION_BOOTSTRAP_FAILED", "error_class", sessionErr.class, "status", authSessionErrorStatus(sessionErr))
		return sessionErr
	}
	if out.SessionJWT == "" {
		sessionErr := authSessionError{class: authSessionEmptyJWT, status: resp.StatusCode}
		emitSidecarHTTPStartupProof("XMILO_SIDECAR_RELAY_SESSION_BOOTSTRAP_FAILED", "error_class", sessionErr.class, "status", authSessionErrorStatus(sessionErr))
		return sessionErr
	}
	if err := store.SetRuntimeConfig("relay_session_jwt", out.SessionJWT); err != nil {
		sessionErr := authSessionError{class: authSessionStoreFailed, status: resp.StatusCode}
		emitSidecarHTTPStartupProof("XMILO_SIDECAR_RELAY_SESSION_BOOTSTRAP_FAILED", "error_class", sessionErr.class, "status", authSessionErrorStatus(sessionErr))
		return sessionErr
	}
	if err := store.SetRuntimeConfig("relay_expires_at", out.ExpiresAt); err != nil {
		sessionErr := authSessionError{class: authSessionStoreFailed, status: resp.StatusCode}
		emitSidecarHTTPStartupProof("XMILO_SIDECAR_RELAY_SESSION_BOOTSTRAP_FAILED", "error_class", sessionErr.class, "status", authSessionErrorStatus(sessionErr))
		return sessionErr
	}
	if out.DeviceUserID != "" {
		if err := store.SetRuntimeConfig("relay_device_user_id", out.DeviceUserID); err != nil {
			sessionErr := authSessionError{class: authSessionStoreFailed, status: resp.StatusCode}
			emitSidecarHTTPStartupProof("XMILO_SIDECAR_RELAY_SESSION_BOOTSTRAP_FAILED", "error_class", sessionErr.class, "status", authSessionErrorStatus(sessionErr))
			return sessionErr
		}
	}
	emitSidecarHTTPStartupProof("XMILO_SIDECAR_RELAY_SESSION_BOOTSTRAP_READY")
	return nil
}
