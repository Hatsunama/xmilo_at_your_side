package sessions

import (
	"context"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"

	"xmilo/relay-go/internal/db"
	"xmilo/relay-go/internal/entitlements"
	"xmilo/relay-go/shared/contracts"
)

type Service struct {
	Store        *db.Store
	JWTSecret    []byte
	DevEntitled  bool // set true for local dev; bypasses DB entitlement check
	Entitlements *entitlements.Service
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

	entitled := s.computeEntitled(ctx, deviceUserID)
	expiresAt := time.Now().UTC().Add(60 * time.Minute)
	sessionID := "sess_" + uuid.NewString()

	_, err := s.Store.Pool.Exec(ctx,
		`INSERT INTO sessions(session_id, device_user_id, expires_at, entitled, created_at) VALUES($1, $2, $3, $4, $5)`,
		sessionID, deviceUserID, expiresAt, entitled, time.Now().UTC())
	if err != nil {
		return contracts.SessionStartResponse{}, err
	}

	token, err := s.issueJWT(deviceUserID, sessionID, entitled, expiresAt)
	if err != nil {
		return contracts.SessionStartResponse{}, err
	}

	return contracts.SessionStartResponse{
		DeviceUserID: deviceUserID,
		SessionJWT:   token,
		ExpiresAt:    expiresAt.Format(time.RFC3339),
		Entitled:     entitled,
	}, nil
}

// Refresh re-computes entitled from DB and issues a fresh JWT.
// Called by the sidecar's proactive background refresher (every ~50 min).
func (s *Service) Refresh(ctx context.Context, deviceUserID, sessionID string) (contracts.SessionStartResponse, error) {
	entitled := s.computeEntitled(ctx, deviceUserID)
	expiresAt := time.Now().UTC().Add(60 * time.Minute)
	token, err := s.issueJWT(deviceUserID, sessionID, entitled, expiresAt)
	if err != nil {
		return contracts.SessionStartResponse{}, err
	}
	return contracts.SessionStartResponse{
		DeviceUserID: deviceUserID,
		SessionJWT:   token,
		ExpiresAt:    expiresAt.Format(time.RFC3339),
		Entitled:     entitled,
	}, nil
}

func (s *Service) issueJWT(deviceUserID, sessionID string, entitled bool, expiresAt time.Time) (string, error) {
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"sub":            deviceUserID,
		"session_id":     sessionID,
		"entitled":       entitled,
		"exp":            expiresAt.Unix(),
		"issued_at_unix": time.Now().UTC().Unix(),
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
