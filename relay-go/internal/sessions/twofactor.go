package sessions

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/pquerna/otp/totp"
)

type identityState struct {
	VerifiedEmail    string
	EmailVerified    bool
	TwoFactorEnabled bool
	TwoFactorOK      bool
}

func (s *Service) identityState(ctx context.Context, deviceUserID string) (identityState, error) {
	state := identityState{}

	var verifiedEmail string
	err := s.Store.Pool.QueryRow(ctx, `
		SELECT email
		FROM email_verifications
		WHERE device_user_id = $1
		  AND verified_at IS NOT NULL
		ORDER BY verified_at DESC
		LIMIT 1
	`, deviceUserID).Scan(&verifiedEmail)
	if err != nil {
		return state, nil
	}

	state.VerifiedEmail = strings.ToLower(strings.TrimSpace(verifiedEmail))
	state.EmailVerified = state.VerifiedEmail != ""
	if !state.EmailVerified {
		return state, nil
	}

	var enabled bool
	_ = s.Store.Pool.QueryRow(ctx, `
		SELECT enabled
		FROM two_factor_credentials
		WHERE email = $1
	`, state.VerifiedEmail).Scan(&enabled)
	state.TwoFactorEnabled = enabled
	if !enabled {
		state.TwoFactorOK = true
		return state, nil
	}

	var trustedDeviceUserID string
	_ = s.Store.Pool.QueryRow(ctx, `
		SELECT device_user_id
		FROM two_factor_trusted_devices
		WHERE email = $1
	`, state.VerifiedEmail).Scan(&trustedDeviceUserID)
	state.TwoFactorOK = trustedDeviceUserID == deviceUserID
	return state, nil
}

func (s *Service) IdentityState(ctx context.Context, deviceUserID string) (identityState, error) {
	return s.identityState(ctx, deviceUserID)
}

func (s *Service) BeginTwoFactorSetup(ctx context.Context, deviceUserID string) (string, string, error) {
	identity, err := s.identityState(ctx, deviceUserID)
	if err != nil {
		return "", "", err
	}
	if !identity.EmailVerified {
		return "", "", errors.New("verify your email before enabling 2FA")
	}

	key, err := totp.Generate(totp.GenerateOpts{
		Issuer:      "xMilo",
		AccountName: identity.VerifiedEmail,
	})
	if err != nil {
		return "", "", err
	}

	now := time.Now().UTC()
	_, err = s.Store.Pool.Exec(ctx, `
		INSERT INTO two_factor_credentials (email, secret, enabled, created_at, updated_at)
		VALUES ($1, $2, FALSE, $3, $3)
		ON CONFLICT (email)
		DO UPDATE SET secret = EXCLUDED.secret, enabled = FALSE, enabled_at = NULL, updated_at = EXCLUDED.updated_at
	`, identity.VerifiedEmail, key.Secret(), now)
	if err != nil {
		return "", "", err
	}
	_, _ = s.Store.Pool.Exec(ctx, `DELETE FROM two_factor_recovery_codes WHERE email = $1`, identity.VerifiedEmail)
	_, _ = s.Store.Pool.Exec(ctx, `DELETE FROM two_factor_trusted_devices WHERE email = $1`, identity.VerifiedEmail)

	return key.Secret(), key.URL(), nil
}

func (s *Service) ConfirmTwoFactorSetup(ctx context.Context, deviceUserID, code string) ([]string, error) {
	identity, err := s.identityState(ctx, deviceUserID)
	if err != nil {
		return nil, err
	}
	if !identity.EmailVerified {
		return nil, errors.New("verify your email before enabling 2FA")
	}

	var secret string
	var enabled bool
	err = s.Store.Pool.QueryRow(ctx, `
		SELECT secret, enabled
		FROM two_factor_credentials
		WHERE email = $1
	`, identity.VerifiedEmail).Scan(&secret, &enabled)
	if err != nil || secret == "" {
		return nil, errors.New("start 2FA setup first")
	}
	if enabled {
		return nil, errors.New("2FA is already enabled")
	}
	if !totp.Validate(strings.TrimSpace(code), secret) {
		return nil, errors.New("invalid authenticator code")
	}

	recoveryCodes, err := generateRecoveryCodes(8)
	if err != nil {
		return nil, err
	}

	tx, err := s.Store.Pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)

	now := time.Now().UTC()
	if _, err := tx.Exec(ctx, `
		UPDATE two_factor_credentials
		SET enabled = TRUE, enabled_at = $2, updated_at = $2
		WHERE email = $1
	`, identity.VerifiedEmail, now); err != nil {
		return nil, err
	}
	if _, err := tx.Exec(ctx, `DELETE FROM two_factor_recovery_codes WHERE email = $1`, identity.VerifiedEmail); err != nil {
		return nil, err
	}
	for _, recoveryCode := range recoveryCodes {
		if _, err := tx.Exec(ctx, `
			INSERT INTO two_factor_recovery_codes (email, code_hash, created_at)
			VALUES ($1, $2, $3)
		`, identity.VerifiedEmail, hashRecoveryCode(recoveryCode), now); err != nil {
			return nil, err
		}
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO two_factor_trusted_devices (email, device_user_id, verified_at)
		VALUES ($1, $2, $3)
		ON CONFLICT (email)
		DO UPDATE SET device_user_id = EXCLUDED.device_user_id, verified_at = EXCLUDED.verified_at
	`, identity.VerifiedEmail, deviceUserID, now); err != nil {
		return nil, err
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return recoveryCodes, nil
}

func (s *Service) VerifyTwoFactor(ctx context.Context, deviceUserID, code, recoveryCode string) error {
	identity, err := s.identityState(ctx, deviceUserID)
	if err != nil {
		return err
	}
	if !identity.EmailVerified {
		return errors.New("verify your email before using 2FA")
	}
	if !identity.TwoFactorEnabled {
		return errors.New("2FA is not enabled for this account")
	}

	ok, err := s.consumeRecoveryCode(ctx, identity.VerifiedEmail, recoveryCode)
	if err != nil {
		return err
	}
	if !ok {
		var secret string
		err = s.Store.Pool.QueryRow(ctx, `SELECT secret FROM two_factor_credentials WHERE email = $1 AND enabled = TRUE`, identity.VerifiedEmail).Scan(&secret)
		if err != nil || secret == "" {
			return errors.New("2FA secret is unavailable")
		}
		if !totp.Validate(strings.TrimSpace(code), secret) {
			return errors.New("invalid authenticator or recovery code")
		}
	}

	_, err = s.Store.Pool.Exec(ctx, `
		INSERT INTO two_factor_trusted_devices (email, device_user_id, verified_at)
		VALUES ($1, $2, $3)
		ON CONFLICT (email)
		DO UPDATE SET device_user_id = EXCLUDED.device_user_id, verified_at = EXCLUDED.verified_at
	`, identity.VerifiedEmail, deviceUserID, time.Now().UTC())
	return err
}

func (s *Service) RegenerateRecoveryCodes(ctx context.Context, deviceUserID string) ([]string, error) {
	identity, err := s.identityState(ctx, deviceUserID)
	if err != nil {
		return nil, err
	}
	if !identity.TwoFactorEnabled || !identity.TwoFactorOK {
		return nil, errors.New("2FA must already be verified on this device")
	}

	recoveryCodes, err := generateRecoveryCodes(8)
	if err != nil {
		return nil, err
	}

	tx, err := s.Store.Pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)

	now := time.Now().UTC()
	if _, err := tx.Exec(ctx, `DELETE FROM two_factor_recovery_codes WHERE email = $1`, identity.VerifiedEmail); err != nil {
		return nil, err
	}
	for _, recoveryCode := range recoveryCodes {
		if _, err := tx.Exec(ctx, `
			INSERT INTO two_factor_recovery_codes (email, code_hash, created_at)
			VALUES ($1, $2, $3)
		`, identity.VerifiedEmail, hashRecoveryCode(recoveryCode), now); err != nil {
			return nil, err
		}
	}
	if _, err := tx.Exec(ctx, `
		UPDATE two_factor_credentials
		SET updated_at = $2
		WHERE email = $1
	`, identity.VerifiedEmail, now); err != nil {
		return nil, err
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return recoveryCodes, nil
}

func (s *Service) DisableTwoFactor(ctx context.Context, deviceUserID, code, recoveryCode string) error {
	identity, err := s.identityState(ctx, deviceUserID)
	if err != nil {
		return err
	}
	if !identity.EmailVerified {
		return errors.New("verify your email before changing 2FA")
	}
	if !identity.TwoFactorEnabled {
		return errors.New("2FA is not enabled")
	}

	ok, err := s.consumeRecoveryCode(ctx, identity.VerifiedEmail, recoveryCode)
	if err != nil {
		return err
	}
	if !ok {
		var secret string
		err = s.Store.Pool.QueryRow(ctx, `SELECT secret FROM two_factor_credentials WHERE email = $1 AND enabled = TRUE`, identity.VerifiedEmail).Scan(&secret)
		if err != nil || secret == "" {
			return errors.New("2FA secret is unavailable")
		}
		if !totp.Validate(strings.TrimSpace(code), secret) {
			return errors.New("invalid authenticator or recovery code")
		}
	}

	tx, err := s.Store.Pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	now := time.Now().UTC()
	if _, err := tx.Exec(ctx, `
		UPDATE two_factor_credentials
		SET enabled = FALSE, enabled_at = NULL, updated_at = $2
		WHERE email = $1
	`, identity.VerifiedEmail, now); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `DELETE FROM two_factor_recovery_codes WHERE email = $1`, identity.VerifiedEmail); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `DELETE FROM two_factor_trusted_devices WHERE email = $1`, identity.VerifiedEmail); err != nil {
		return err
	}

	return tx.Commit(ctx)
}

func (s *Service) CreateWebsiteHandoff(ctx context.Context, claims jwt.MapClaims) (string, time.Time, error) {
	deviceUserID, _ := claims["sub"].(string)
	identity, err := s.identityState(ctx, deviceUserID)
	if err != nil {
		return "", time.Time{}, err
	}
	if !identity.EmailVerified {
		return "", time.Time{}, errors.New("verify your email before opening the website")
	}
	if !identity.TwoFactorOK {
		return "", time.Time{}, errors.New("complete 2FA on this device before opening the website")
	}

	expiresAt := time.Now().UTC().Add(time.Duration(s.HandoffMaxAgeSeconds) * time.Second)
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"sub":            identity.VerifiedEmail,
		"aud":            s.HandoffAudience,
		"iss":            "xmilo-relay",
		"device_user_id": deviceUserID,
		"email_verified": true,
		"two_factor_ok":  true,
		"handoff_type":   "website_session",
		"jti":            "handoff_" + uuid.NewString(),
		"iat":            time.Now().UTC().Unix(),
		"exp":            expiresAt.Unix(),
	})
	signed, err := token.SignedString(s.HandoffSecret)
	if err != nil {
		return "", time.Time{}, err
	}
	return signed, expiresAt, nil
}

func generateRecoveryCodes(count int) ([]string, error) {
	codes := make([]string, 0, count)
	for i := 0; i < count; i++ {
		raw := strings.ToUpper(strings.ReplaceAll(uuid.NewString(), "-", ""))
		code := raw[:4] + "-" + raw[4:8] + "-" + raw[8:12]
		codes = append(codes, code)
	}
	return codes, nil
}

func hashRecoveryCode(code string) string {
	sum := sha256.Sum256([]byte(normalizeRecoveryCode(code)))
	return hex.EncodeToString(sum[:])
}

func normalizeRecoveryCode(code string) string {
	return strings.ToUpper(strings.ReplaceAll(strings.TrimSpace(code), "-", ""))
}

func (s *Service) consumeRecoveryCode(ctx context.Context, email, recoveryCode string) (bool, error) {
	normalized := normalizeRecoveryCode(recoveryCode)
	if normalized == "" {
		return false, nil
	}

	result, err := s.Store.Pool.Exec(ctx, `
		UPDATE two_factor_recovery_codes
		SET used_at = $3
		WHERE email = $1
		  AND code_hash = $2
		  AND used_at IS NULL
	`, email, hashRecoveryCode(normalized), time.Now().UTC())
	if err != nil {
		return false, err
	}
	return result.RowsAffected() > 0, nil
}
