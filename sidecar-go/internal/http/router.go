package httpx

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"xmilo/sidecar-go/internal/config"
	"xmilo/sidecar-go/internal/db"
	"xmilo/sidecar-go/internal/legacy"
	"xmilo/sidecar-go/internal/maintenance"
	"xmilo/sidecar-go/internal/mind"
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
	relayClient    *relay.Client // kept on App for background refresh goroutine
	startedAt      time.Time
	mindLoaded     bool
	wakeLockActive bool
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

	// Bootstrap JWT on first launch.
	// relay_session_jwt is empty on a fresh install → relay returns 401 on every /llm/turn
	// → tasks immediately go stuck. This pre-fills the JWT so the first task can proceed.
	if existing, _ := store.GetRuntimeConfig("relay_session_jwt"); existing == "" {
		if err := bootstrapRelaySession(cfg, store); err != nil {
			_ = err // Non-fatal: sidecar starts; tasks go stuck until relay reachable.
		}
	}

	hub := ws.NewHub()
	engine := tasks.New(store, relayClient, hub, prompt)

	return &App{
		cfg:         cfg,
		store:       store,
		hub:         hub,
		engine:      engine,
		relayClient: relayClient,
		startedAt:   time.Now().UTC(),
		mindLoaded:  true,
	}, nil
}

func (a *App) Start(ctx context.Context) error {
	if err := a.acquireWakeLock(); err != nil {
		// keep going on non-Termux hosts
	}

	if err := a.store.SetRuntimeConfig("last_meaningful_user_action_at", time.Now().UTC().Format(time.RFC3339)); err != nil {
		return err
	}

	maintenance.Start(ctx)

	// Proactive JWT refresh — wakes every minute, refreshes when expiry < 10 min away.
	// This means email verification shows up in the entitled claim within ~1 minute
	// without requiring any app-side action (user just waits or taps /auth/check).
	go a.runJWTRefresher(ctx)

	mux := http.NewServeMux()
	mux.Handle("/health", RequireBearer(a.cfg.BearerToken, http.HandlerFunc(a.handleHealth)))
	mux.Handle("/ready", RequireBearer(a.cfg.BearerToken, http.HandlerFunc(a.handleReady)))
	mux.Handle("/bootstrap", RequireBearer(a.cfg.BearerToken, http.HandlerFunc(a.handleBootstrap)))

	// Auth passthrough: sidecar is the sole relay caller; app never speaks to relay directly.
	mux.Handle("/auth/register", RequireBearer(a.cfg.BearerToken, http.HandlerFunc(a.handleAuthRegister)))
	mux.Handle("/auth/invite", RequireBearer(a.cfg.BearerToken, http.HandlerFunc(a.handleAuthInvite)))
	// /auth/check forces an immediate relay refresh and returns the current entitled status.
	// The app calls this right after the user taps "I verified my email".
	mux.Handle("/auth/check", RequireBearer(a.cfg.BearerToken, http.HandlerFunc(a.handleAuthCheck)))
	// /auth/refresh receives a JWT pushed down from the app (e.g. after RevenueCat purchase).
	mux.Handle("/auth/refresh", RequireBearer(a.cfg.BearerToken, http.HandlerFunc(a.handleAuthRefresh)))

	mux.Handle("/task/start", RequireBearer(a.cfg.BearerToken, http.HandlerFunc(a.handleTaskStart)))
	mux.Handle("/task/current", RequireBearer(a.cfg.BearerToken, http.HandlerFunc(a.handleTaskCurrent)))
	mux.Handle("/task/interrupt", RequireBearer(a.cfg.BearerToken, http.HandlerFunc(a.handleTaskInterrupt)))
	mux.Handle("/context/set", RequireBearer(a.cfg.BearerToken, http.HandlerFunc(a.handleContextSet)))
	mux.Handle("/context/clear", RequireBearer(a.cfg.BearerToken, http.HandlerFunc(a.handleContextClear)))
	mux.Handle("/task/cancel", RequireBearer(a.cfg.BearerToken, http.HandlerFunc(a.handleTaskCancel)))
	mux.Handle("/task/choice", RequireBearer(a.cfg.BearerToken, http.HandlerFunc(a.handleTaskChoice)))
	mux.Handle("/task/resume_queue", RequireBearer(a.cfg.BearerToken, http.HandlerFunc(a.handleTaskResumeQueue)))
	mux.Handle("/thought/request", RequireBearer(a.cfg.BearerToken, http.HandlerFunc(a.handleThoughtRequest)))
	mux.Handle("/state", RequireBearer(a.cfg.BearerToken, http.HandlerFunc(a.handleState)))
	mux.Handle("/storage/stats", RequireBearer(a.cfg.BearerToken, http.HandlerFunc(a.handleStorageStats)))
	mux.Handle("/reset", RequireBearer(a.cfg.BearerToken, http.HandlerFunc(a.handleReset)))
	mux.Handle("/trophy/conjure", RequireBearer(a.cfg.BearerToken, http.HandlerFunc(a.stub("trophy conjure not yet implemented"))))
	mux.Handle("/inspector/open", RequireBearer(a.cfg.BearerToken, http.HandlerFunc(a.stub("inspector open not yet implemented"))))
	mux.Handle("/inspector/close", RequireBearer(a.cfg.BearerToken, http.HandlerFunc(a.stub("inspector close not yet implemented"))))
	mux.Handle("/ws", RequireWSBearer(a.cfg.BearerToken, http.HandlerFunc(a.hub.Handle)))

	server := &http.Server{
		Addr:    net.JoinHostPort(a.cfg.Host, strconv.Itoa(a.cfg.Port)),
		Handler: mux,
	}

	a.hub.Broadcast("runtime.ready", map[string]any{
		"ready":            true,
		"wake_lock_active": a.wakeLockActive,
	})

	go func() {
		<-ctx.Done()
		_ = server.Shutdown(context.Background())
		if a.wakeLockActive {
			_, _ = exec.Command("termux-wake-unlock").CombinedOutput()
		}
		_ = a.store.Close()
	}()

	return server.ListenAndServe()
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
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// handleAuthCheck forces an immediate relay refresh and returns the new entitled status.
// The app calls this after user verifies email — gets entitled=true within seconds.
func (a *App) handleAuthCheck(w http.ResponseWriter, r *http.Request) {
	newJWT, newExpiry, err := a.relayClient.Refresh(r.Context())
	if err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]any{"error": err.Error(), "entitled": false})
		return
	}
	_ = a.store.SetRuntimeConfig("relay_session_jwt", newJWT)
	_ = a.store.SetRuntimeConfig("relay_expires_at", newExpiry)

	// Quick decode of the new JWT to extract entitled claim
	entitled := jwtEntitledClaim(newJWT)
	writeJSON(w, http.StatusOK, map[string]any{
		"ok":         true,
		"entitled":   entitled,
		"expires_at": newExpiry,
	})
}

// ─── Existing handlers (unchanged from v7) ────────────────────────────────────

func (a *App) handleHealth(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, runtime.HealthResponse{
		OK:        true,
		Service:   "picoclaw-sidecar",
		Version:   "0.1.0",
		UptimeSec: int64(time.Since(a.startedAt).Seconds()),
	})
}

func (a *App) handleReady(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, runtime.ReadyResponse{
		OK:              true,
		WakeLockActive:  a.wakeLockActive,
		DBReady:         true,
		RelayConfigured: a.cfg.RelayBaseURL != "",
		MindLoaded:      a.mindLoaded,
		RuntimeID:       a.cfg.RuntimeID,
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
	snap, err := a.engine.StartTask(r.Context(), req.Prompt)
	if err != nil {
		writeJSON(w, http.StatusConflict, map[string]any{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusAccepted, map[string]any{"task_id": snap.TaskID, "immediate_state": snap})
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
	var req struct {
		Content string `json:"content"`
	}
	if err := decodeJSON(r, &req); err != nil || req.Content == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "content required"})
		return
	}
	if err := a.store.SetRuntimeConfig("active_context", req.Content); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (a *App) handleContextClear(w http.ResponseWriter, r *http.Request) {
	if err := a.store.SetRuntimeConfig("active_context", ""); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (a *App) handleTaskCancel(w http.ResponseWriter, r *http.Request) {
	if err := a.engine.InterruptTask("cancel"); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (a *App) handleThoughtRequest(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, a.engine.ThoughtRequest())
}

func (a *App) handleState(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, a.engine.Snapshot())
}

func (a *App) handleStorageStats(w http.ResponseWriter, r *http.Request) {
	stats, err := a.store.StorageStats(a.cfg.DBPath)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, stats)
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

func (a *App) acquireWakeLock() error {
	cmd := exec.Command("termux-wake-lock")
	if err := cmd.Run(); err != nil {
		return err
	}
	a.wakeLockActive = true
	return nil
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

// jwtEntitledClaim extracts the entitled bool from a JWT without signature verification.
// We trust the relay issued it correctly; this is only used for the /auth/check response.
func jwtEntitledClaim(token string) bool {
	parts := strings.SplitN(token, ".", 3)
	if len(parts) != 3 {
		return false
	}
	payload := parts[1]
	// Pad to multiple of 4 for standard base64
	for len(payload)%4 != 0 {
		payload += "="
	}
	data, err := base64.URLEncoding.DecodeString(payload)
	if err != nil {
		return false
	}
	var claims map[string]any
	if err := json.Unmarshal(data, &claims); err != nil {
		return false
	}
	entitled, _ := claims["entitled"].(bool)
	return entitled
}

// bootstrapRelaySession calls POST /session/start and stores the returned JWT.
func bootstrapRelaySession(cfg config.Config, store *db.Store) error {
	reqBody, _ := json.Marshal(map[string]string{"device_name": "xmilo-sidecar"})
	httpClient := &http.Client{Timeout: 15 * time.Second}
	resp, err := httpClient.Post(cfg.RelayBaseURL+"/session/start", "application/json", bytes.NewReader(reqBody))
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	if resp.StatusCode >= 400 {
		return fmt.Errorf("session/start failed: %s", string(raw))
	}
	var out struct {
		SessionJWT string `json:"session_jwt"`
		ExpiresAt  string `json:"expires_at"`
	}
	if err := json.Unmarshal(raw, &out); err != nil {
		return err
	}
	if out.SessionJWT == "" {
		return fmt.Errorf("session/start returned empty jwt")
	}
	_ = store.SetRuntimeConfig("relay_session_jwt", out.SessionJWT)
	_ = store.SetRuntimeConfig("relay_expires_at", out.ExpiresAt)
	return nil
}
