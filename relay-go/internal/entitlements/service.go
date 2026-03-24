package entitlements

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"math/big"
	"strings"
	"time"

	"xmilo/relay-go/internal/db"
)

// Service manages the full entitlement lifecycle: trial, access-code, subscription.
//
// Entitlement model (locked product rules):
//   - trial:        email verified → trial grant created with started_at NULL.
//     On first /llm/turn: started_at = now, expires_at = now + TrialHours.
//   - invite:       access code redeemed → grant with started_at = now, expires_at = now + days.
//   - subscription: RevenueCat INITIAL_PURCHASE → active grant, expires_at NULL.
//     RevenueCat CANCELLATION/EXPIRATION → active = FALSE.
type Service struct {
	Store      *db.Store
	TrialHours int
	InviteDays int
}

// IsEntitled returns true if device_user_id has any currently active entitlement.
// A trial with started_at NULL (verified but trial not started) is also entitled —
// the trial clock starts on their first /llm/turn call.
func (s *Service) IsEntitled(ctx context.Context, deviceUserID string) (bool, error) {
	now := time.Now().UTC()
	row := s.Store.Pool.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1 FROM entitlement_grants
			WHERE device_user_id = $1
			  AND active = TRUE
			  AND (
			        (grant_type = 'trial' AND started_at IS NULL)
			     OR (grant_type = 'trial' AND started_at IS NOT NULL AND expires_at > $2)
			     OR (grant_type = 'invite' AND expires_at > $2)
			     OR (grant_type = 'subscription' AND (expires_at IS NULL OR expires_at > $2))
			  )
		)
	`, deviceUserID, now)
	var entitled bool
	err := row.Scan(&entitled)
	return entitled, err
}

// GrantTrial creates a pending trial grant after email verification.
// No-op if a trial grant already exists for this device.
func (s *Service) GrantTrial(ctx context.Context, deviceUserID string) error {
	now := time.Now().UTC()
	_, err := s.Store.Pool.Exec(ctx, `
		INSERT INTO entitlement_grants (device_user_id, grant_type, active, created_at)
		SELECT $1, 'trial', TRUE, $2
		WHERE NOT EXISTS (
			SELECT 1 FROM entitlement_grants
			WHERE device_user_id = $1 AND grant_type = 'trial'
		)
	`, deviceUserID, now)
	return err
}

// StartTrialIfNeeded starts the 12-hour trial clock on the user's first real task.
// Called inside /llm/turn. No-op if trial already started or no trial grant exists.
func (s *Service) StartTrialIfNeeded(ctx context.Context, deviceUserID string) error {
	now := time.Now().UTC()
	expiresAt := now.Add(time.Duration(s.TrialHours) * time.Hour)
	_, err := s.Store.Pool.Exec(ctx, `
		UPDATE entitlement_grants
		SET started_at = $1, expires_at = $2
		WHERE device_user_id = $3
		  AND grant_type = 'trial'
		  AND started_at IS NULL
		  AND active = TRUE
	`, now, expiresAt, deviceUserID)
	return err
}

// RedeemInvite validates and consumes a single-use access code, granting access.
// Returns an error if the code is invalid or already used.
func (s *Service) RedeemInvite(ctx context.Context, code, deviceUserID string) error {
	now := time.Now().UTC()

	row := s.Store.Pool.QueryRow(ctx,
		`SELECT granted_days FROM invite_codes WHERE code = $1 AND used_by IS NULL`, code)
	var days int
	if err := row.Scan(&days); err != nil {
		return errors.New("invalid or already-used access code")
	}
	expiresAt := now.Add(time.Duration(days) * 24 * time.Hour)

	tx, err := s.Store.Pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	if _, err := tx.Exec(ctx,
		`UPDATE invite_codes SET used_by = $1, used_at = $2 WHERE code = $3`,
		deviceUserID, now, code); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO entitlement_grants
			(device_user_id, grant_type, started_at, expires_at, active, created_at)
		VALUES ($1, 'invite', $2, $3, TRUE, $2)
	`, deviceUserID, now, expiresAt); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// ProcessRevenueCatEvent handles a RevenueCat webhook event type for a device_user_id.
// The RevenueCat app_user_id must be set to device_user_id at purchase time in the app.
func (s *Service) ProcessRevenueCatEvent(ctx context.Context, eventType, deviceUserID string) error {
	if deviceUserID == "" {
		return nil
	}
	now := time.Now().UTC()
	switch eventType {
	case "INITIAL_PURCHASE":
		_, err := s.Store.Pool.Exec(ctx, `
			INSERT INTO entitlement_grants
				(device_user_id, grant_type, started_at, expires_at, active, created_at)
			SELECT $1, 'subscription', $2, NULL, TRUE, $2
			WHERE NOT EXISTS (
				SELECT 1 FROM entitlement_grants
				WHERE device_user_id = $1 AND grant_type = 'subscription' AND active = TRUE
			)
		`, deviceUserID, now)
		return err
	case "RENEWAL", "UNCANCELLATION":
		// Ensure there's an active subscription grant (re-activate if needed)
		_, _ = s.Store.Pool.Exec(ctx, `
			UPDATE entitlement_grants SET active = TRUE
			WHERE device_user_id = $1 AND grant_type = 'subscription'
		`, deviceUserID)
		_, err := s.Store.Pool.Exec(ctx, `
			INSERT INTO entitlement_grants
				(device_user_id, grant_type, started_at, expires_at, active, created_at)
			SELECT $1, 'subscription', $2, NULL, TRUE, $2
			WHERE NOT EXISTS (
				SELECT 1 FROM entitlement_grants
				WHERE device_user_id = $1 AND grant_type = 'subscription' AND active = TRUE
			)
		`, deviceUserID, now)
		return err
	case "CANCELLATION", "EXPIRATION":
		_, err := s.Store.Pool.Exec(ctx, `
			UPDATE entitlement_grants SET active = FALSE
			WHERE device_user_id = $1 AND grant_type = 'subscription'
		`, deviceUserID)
		return err
	}
	return nil // ignore unknown event types
}

// CreateInviteCodes generates n single-use codes with the given access duration.
// Returns the list of created codes.
func (s *Service) CreateInviteCodes(ctx context.Context, n, days int, createdBy string) ([]string, error) {
	now := time.Now().UTC()
	codes := make([]string, 0, n)
	for i := 0; i < n; i++ {
		code, err := generateCode()
		if err != nil {
			return codes, err
		}
		if _, err := s.Store.Pool.Exec(ctx,
			`INSERT INTO invite_codes (code, granted_days, created_at, created_by) VALUES ($1, $2, $3, $4)`,
			code, days, now, createdBy); err != nil {
			return codes, fmt.Errorf("insert code %s: %w", code, err)
		}
		codes = append(codes, code)
	}
	return codes, nil
}

// Stats returns a snapshot of user/entitlement counts for the admin page.
func (s *Service) Stats(ctx context.Context) (map[string]any, error) {
	result := map[string]any{}
	queries := []struct {
		key string
		sql string
	}{
		{"total_device_users", `SELECT COUNT(*) FROM device_users`},
		{"verified_emails", `SELECT COUNT(*) FROM email_verifications WHERE verified_at IS NOT NULL`},
		{"active_trials", `SELECT COUNT(*) FROM entitlement_grants WHERE grant_type='trial' AND active=TRUE AND started_at IS NOT NULL AND expires_at > NOW()`},
		{"pending_trials", `SELECT COUNT(*) FROM entitlement_grants WHERE grant_type='trial' AND active=TRUE AND started_at IS NULL`},
		{"active_subscriptions", `SELECT COUNT(*) FROM entitlement_grants WHERE grant_type='subscription' AND active=TRUE`},
		{"active_invites", `SELECT COUNT(*) FROM entitlement_grants WHERE grant_type='invite' AND active=TRUE AND expires_at > NOW()`},
		{"unused_invite_codes", `SELECT COUNT(*) FROM invite_codes WHERE used_by IS NULL`},
		{"total_llm_turns", `SELECT COUNT(*) FROM llm_turns`},
	}
	for _, q := range queries {
		var n int64
		if err := s.Store.Pool.QueryRow(ctx, q.sql).Scan(&n); err != nil {
			result[q.key] = "err"
		} else {
			result[q.key] = n
		}
	}
	return result, nil
}

// ListInviteCodes returns all access codes with their usage state.
func (s *Service) ListInviteCodes(ctx context.Context) ([]map[string]any, error) {
	rows, err := s.Store.Pool.Query(ctx,
		`SELECT code, granted_days, created_at, created_by, used_by, used_at FROM invite_codes ORDER BY created_at DESC LIMIT 200`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var list []map[string]any
	for rows.Next() {
		var code, createdBy string
		var usedBy *string
		var usedAt *time.Time
		var days int
		var createdAt time.Time
		if err := rows.Scan(&code, &days, &createdAt, &createdBy, &usedBy, &usedAt); err != nil {
			continue
		}
		item := map[string]any{
			"code":         code,
			"granted_days": days,
			"created_at":   createdAt.Format(time.RFC3339),
			"created_by":   createdBy,
			"used":         usedBy != nil,
		}
		if usedBy != nil {
			item["used_by"] = *usedBy
		}
		if usedAt != nil {
			item["used_at"] = usedAt.Format(time.RFC3339)
		}
		list = append(list, item)
	}
	return list, rows.Err()
}

// ListErrorReports returns recent error reports.
func (s *Service) ListErrorReports(ctx context.Context) ([]map[string]any, error) {
	rows, err := s.Store.Pool.Query(ctx,
		`SELECT id, device_user_id, error_type, message, created_at FROM error_reports ORDER BY created_at DESC LIMIT 100`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var list []map[string]any
	for rows.Next() {
		var id int64
		var deviceUserID, errType, message string
		var createdAt time.Time
		if err := rows.Scan(&id, &deviceUserID, &errType, &message, &createdAt); err != nil {
			continue
		}
		list = append(list, map[string]any{
			"id":             id,
			"device_user_id": deviceUserID,
			"error_type":     errType,
			"message":        message,
			"created_at":     createdAt.Format(time.RFC3339),
		})
	}
	return list, rows.Err()
}

// ListAIContentReports returns recent AI-content reports from in-app reporting.
func (s *Service) ListAIContentReports(ctx context.Context) ([]map[string]any, error) {
	rows, err := s.Store.Pool.Query(ctx, `
		SELECT id, device_user_id, task_id, event_type, report_reason, report_note, output_text, prompt_text, runtime_id, model_name, status, review_note, created_at
		FROM ai_content_reports
		ORDER BY created_at DESC
		LIMIT 100`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var list []map[string]any
	for rows.Next() {
		var (
			id           int64
			deviceUserID string
			taskID       *string
			eventType    string
			reason       string
			note         string
			outputText   string
			promptText   string
			runtimeID    string
			modelName    string
			status       string
			reviewNote   string
			createdAt    time.Time
		)
		if err := rows.Scan(&id, &deviceUserID, &taskID, &eventType, &reason, &note, &outputText, &promptText, &runtimeID, &modelName, &status, &reviewNote, &createdAt); err != nil {
			continue
		}
		item := map[string]any{
			"id":             id,
			"device_user_id": deviceUserID,
			"event_type":     eventType,
			"report_reason":  reason,
			"report_note":    note,
			"output_text":    outputText,
			"prompt_text":    promptText,
			"runtime_id":     runtimeID,
			"model_name":     modelName,
			"status":         status,
			"review_note":    reviewNote,
			"created_at":     createdAt.Format(time.RFC3339),
		}
		if taskID != nil {
			item["task_id"] = *taskID
		}
		list = append(list, item)
	}
	return list, rows.Err()
}

func (s *Service) UpdateAIContentReportStatus(ctx context.Context, reportID int64, status, reviewNote string) error {
	status = strings.TrimSpace(strings.ToLower(status))
	switch status {
	case "new", "triaged", "confirmed", "dismissed", "rule_update_needed":
	default:
		return fmt.Errorf("invalid ai report status: %s", status)
	}

	_, err := s.Store.Pool.Exec(ctx, `
		UPDATE ai_content_reports
		SET status = $2, review_note = $3
		WHERE id = $1
	`, reportID, status, strings.TrimSpace(reviewNote))
	return err
}

// generateCode returns a random 8-char alphanumeric access code (no ambiguous chars).
func generateCode() (string, error) {
	const chars = "ABCDEFGHJKLMNPQRSTUVWXYZ23456789"
	b := make([]byte, 8)
	for i := range b {
		n, err := rand.Int(rand.Reader, big.NewInt(int64(len(chars))))
		if err != nil {
			return "", err
		}
		b[i] = chars[n.Int64()]
	}
	return string(b), nil
}
