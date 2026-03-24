package sessions

import (
	"context"
	"errors"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"

	"xmilo/relay-go/internal/db"
	"xmilo/relay-go/internal/entitlements"
	"xmilo/relay-go/shared/contracts"
)

type Service struct {
	Store                *db.Store
	JWTSecret            []byte
	DevEntitled          bool // set true for local dev; bypasses DB entitlement check
	Entitlements         *entitlements.Service
	AccessMode           string
	InviteDays           int
	HandoffSecret        []byte
	HandoffAudience      string
	HandoffMaxAgeSeconds int
}

// computeEntitled resolves whether a device is currently entitled.
// DevEntitled bypasses DB lookup so dev mode stays frictionless.
func (s *Service) computeEntitled(ctx context.Context, deviceUserID string) bool {
	if s.DevEntitled {
		return true
	}
	entitled, err := s.Entitlements.IsEntitled(ctx, deviceUserID)
	if err != nil {
		return false
	}
	return entitled
}

// Start creates a new device_user (if none provided), session, and issues a JWT.
// The entitled claim reflects DB state at issue time; sidecar refreshes every ~50 min.
func (s *Service) Start(ctx context.Context, req contracts.SessionStartRequest) (contracts.SessionStartResponse, error) {
	deviceUserID := req.DeviceUserID
	if deviceUserID == "" {
		deviceUserID = "du_" + uuid.NewString()
		_, err := s.Store.Pool.Exec(ctx,
			`INSERT INTO device_users(device_user_id, device_name, created_at) VALUES($1, $2, $3)`,
			deviceUserID, req.DeviceName, time.Now().UTC())
		if err != nil {
			return contracts.SessionStartResponse{}, err
		}
	}

	identity, err := s.identityState(ctx, deviceUserID)
	if err != nil {
		return contracts.SessionStartResponse{}, err
	}
	entitled := s.computeEntitled(ctx, deviceUserID)
	expiresAt := time.Now().UTC().Add(60 * time.Minute)
	sessionID := "sess_" + uuid.NewString()

	_, err = s.Store.Pool.Exec(ctx,
		`INSERT INTO sessions(session_id, device_user_id, expires_at, entitled, created_at) VALUES($1, $2, $3, $4, $5)`,
		sessionID, deviceUserID, expiresAt, entitled, time.Now().UTC())
	if err != nil {
		return contracts.SessionStartResponse{}, err
	}

	token, err := s.issueJWT(deviceUserID, sessionID, entitled, expiresAt, identity)
	if err != nil {
		return contracts.SessionStartResponse{}, err
	}

	return contracts.SessionStartResponse{
		DeviceUserID:        deviceUserID,
		SessionJWT:          token,
		ExpiresAt:           expiresAt.Format(time.RFC3339),
		Entitled:            entitled,
		AccessMode:          s.AccessMode,
		AccessCodeOnly:      s.AccessMode == "code_only",
		TrialAllowed:        s.AccessMode != "code_only",
		SubscriptionAllowed: s.AccessMode != "code_only",
		AccessCodeGrantDays: s.InviteDays,
		VerifiedEmail:       identity.VerifiedEmail,
		EmailVerified:       identity.EmailVerified,
		TwoFactorEnabled:    identity.TwoFactorEnabled,
		TwoFactorOK:         identity.TwoFactorOK,
		WebsiteHandoffReady: identity.EmailVerified && identity.TwoFactorOK,
	}, nil
}

// Refresh re-computes entitled from DB and issues a fresh JWT.
// Called by the sidecar's proactive background refresher (every ~50 min).
func (s *Service) Refresh(ctx context.Context, deviceUserID, sessionID string) (contracts.SessionStartResponse, error) {
	identity, err := s.identityState(ctx, deviceUserID)
	if err != nil {
		return contracts.SessionStartResponse{}, err
	}
	entitled := s.computeEntitled(ctx, deviceUserID)
	expiresAt := time.Now().UTC().Add(60 * time.Minute)
	token, err := s.issueJWT(deviceUserID, sessionID, entitled, expiresAt, identity)
	if err != nil {
		return contracts.SessionStartResponse{}, err
	}
	return contracts.SessionStartResponse{
		DeviceUserID:        deviceUserID,
		SessionJWT:          token,
		ExpiresAt:           expiresAt.Format(time.RFC3339),
		Entitled:            entitled,
		AccessMode:          s.AccessMode,
		AccessCodeOnly:      s.AccessMode == "code_only",
		TrialAllowed:        s.AccessMode != "code_only",
		SubscriptionAllowed: s.AccessMode != "code_only",
		AccessCodeGrantDays: s.InviteDays,
		VerifiedEmail:       identity.VerifiedEmail,
		EmailVerified:       identity.EmailVerified,
		TwoFactorEnabled:    identity.TwoFactorEnabled,
		TwoFactorOK:         identity.TwoFactorOK,
		WebsiteHandoffReady: identity.EmailVerified && identity.TwoFactorOK,
	}, nil
}

func (s *Service) issueJWT(deviceUserID, sessionID string, entitled bool, expiresAt time.Time, identity identityState) (string, error) {
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"sub":                    deviceUserID,
		"session_id":             sessionID,
		"entitled":               entitled,
		"access_mode":            s.AccessMode,
		"access_code_only":       s.AccessMode == "code_only",
		"trial_allowed":          s.AccessMode != "code_only",
		"subscription_allowed":   s.AccessMode != "code_only",
		"access_code_grant_days": s.InviteDays,
		"verified_email":         identity.VerifiedEmail,
		"email_verified":         identity.EmailVerified,
		"two_factor_enabled":     identity.TwoFactorEnabled,
		"two_factor_ok":          identity.TwoFactorOK,
		"website_handoff_ready":  identity.EmailVerified && identity.TwoFactorOK,
		"exp":                    expiresAt.Unix(),
		"issued_at_unix":         time.Now().UTC().Unix(),
	})
	return token.SignedString(s.JWTSecret)
}

func (s *Service) ParseJWT(raw string) (jwt.MapClaims, error) {
	token, err := jwt.Parse(raw, func(token *jwt.Token) (interface{}, error) {
		return s.JWTSecret, nil
	})
	if err != nil {
		return nil, err
	}
	claims, _ := token.Claims.(jwt.MapClaims)
	return claims, nil
}

func (s *Service) DeleteAccount(ctx context.Context, deviceUserID string) error {
	identity, err := s.identityState(ctx, deviceUserID)
	if err != nil {
		return err
	}
	if !identity.EmailVerified || identity.VerifiedEmail == "" {
		return errors.New("verify your email before deleting your account")
	}

	rows, err := s.Store.Pool.Query(ctx, `
		SELECT DISTINCT device_user_id
		FROM email_verifications
		WHERE email = $1
	`, identity.VerifiedEmail)
	if err != nil {
		return err
	}
	defer rows.Close()

	deviceIDs := make([]string, 0, 4)
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return err
		}
		deviceIDs = append(deviceIDs, id)
	}
	if err := rows.Err(); err != nil {
		return err
	}
	if len(deviceIDs) == 0 {
		deviceIDs = append(deviceIDs, deviceUserID)
	}

	tx, err := s.Store.Pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	if _, err := tx.Exec(ctx, `DELETE FROM sessions WHERE device_user_id = ANY($1)`, deviceIDs); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `DELETE FROM entitlement_grants WHERE device_user_id = ANY($1)`, deviceIDs); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `DELETE FROM error_reports WHERE device_user_id = ANY($1)`, deviceIDs); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `DELETE FROM ai_content_reports WHERE device_user_id = ANY($1)`, deviceIDs); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `DELETE FROM llm_turns WHERE device_user_id = ANY($1)`, deviceIDs); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `DELETE FROM two_factor_trusted_devices WHERE email = $1`, identity.VerifiedEmail); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `DELETE FROM two_factor_recovery_codes WHERE email = $1`, identity.VerifiedEmail); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `DELETE FROM two_factor_credentials WHERE email = $1`, identity.VerifiedEmail); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `DELETE FROM users WHERE email = $1`, identity.VerifiedEmail); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `DELETE FROM email_verifications WHERE email = $1`, identity.VerifiedEmail); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `
		UPDATE device_users
		SET device_name = 'deleted-account-device'
		WHERE device_user_id = ANY($1)
	`, deviceIDs); err != nil {
		return err
	}

	return tx.Commit(ctx)
}
