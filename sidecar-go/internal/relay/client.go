package relay

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"

	"xmilo/sidecar-go/shared/contracts"
)

type Client struct {
	BaseURL string
	JWT     func() (string, error) // reads relay_session_jwt from SQLite
	HTTP    *http.Client
}

func New(baseURL string, jwtGetter func() (string, error)) *Client {
	return &Client{
		BaseURL: baseURL,
		JWT:     jwtGetter,
		HTTP:    &http.Client{Timeout: 12 * time.Minute},
	}
}

// Turn calls /llm/turn with 3-attempt retry (2s, 5s, 10s backoff).
// 4xx responses are NOT retried — they indicate auth/entitlement issues.
func (c *Client) Turn(ctx context.Context, req contracts.RelayTurnRequest) (contracts.RelayTurnResponse, error) {
	var out contracts.RelayTurnResponse
	body, err := json.Marshal(req)
	if err != nil {
		return out, err
	}

	delays := []time.Duration{2 * time.Second, 5 * time.Second, 10 * time.Second}
	var lastErr error

	for attempt := 0; attempt <= len(delays); attempt++ {
		if attempt > 0 {
			select {
			case <-ctx.Done():
				return out, ctx.Err()
			case <-time.After(delays[attempt-1]):
			}
		}

		jwt, err := c.JWT()
		if err != nil {
			return out, err
		}

		httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.BaseURL+"/llm/turn", bytes.NewReader(body))
		if err != nil {
			return out, err
		}
		httpReq.Header.Set("Content-Type", "application/json")
		if jwt != "" {
			httpReq.Header.Set("Authorization", "Bearer "+jwt)
		}

		resp, err := c.HTTP.Do(httpReq)
		if err != nil {
			lastErr = err
			continue
		}

		raw, readErr := io.ReadAll(resp.Body)
		resp.Body.Close()
		if readErr != nil {
			lastErr = readErr
			continue
		}

		if resp.StatusCode >= 500 {
			lastErr = fmt.Errorf("relay server error: %s", resp.Status)
			continue
		}
		if resp.StatusCode >= 400 {
			// 4xx: don't retry — entitlement_lost, bad JWT, etc.
			return out, fmt.Errorf("relay error: %s: %s", resp.Status, string(raw))
		}
		if err := json.Unmarshal(raw, &out); err != nil {
			return out, err
		}
		return out, nil
	}
	return out, fmt.Errorf("relay retry exhausted: %w", lastErr)
}

// Refresh calls POST /auth/refresh using the current stored JWT.
// The relay re-computes entitled from DB and issues a fresh 60-min JWT.
// Returns (newJWT, expiresAt, error).
func (c *Client) Refresh(ctx context.Context) (string, string, error) {
	currentJWT, err := c.JWT()
	if err != nil || currentJWT == "" {
		return "", "", errors.New("no JWT to refresh")
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.BaseURL+"/auth/refresh", http.NoBody)
	if err != nil {
		return "", "", err
	}
	req.Header.Set("Authorization", "Bearer "+currentJWT)
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.HTTP.Do(req)
	if err != nil {
		return "", "", fmt.Errorf("refresh request: %w", err)
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", "", err
	}
	if resp.StatusCode >= 400 {
		return "", "", fmt.Errorf("refresh failed %s: %s", resp.Status, string(raw))
	}

	var out struct {
		SessionJWT string `json:"session_jwt"`
		ExpiresAt  string `json:"expires_at"`
		Entitled   bool   `json:"entitled"`
	}
	if err := json.Unmarshal(raw, &out); err != nil {
		return "", "", err
	}
	return out.SessionJWT, out.ExpiresAt, nil
}

// Register calls POST /auth/register on the relay to initiate email verification.
func (c *Client) Register(ctx context.Context, email, deviceUserID string) error {
	body, _ := json.Marshal(map[string]string{
		"email":          email,
		"device_user_id": deviceUserID,
	})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.BaseURL+"/auth/register", bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.HTTP.Do(req)
	if err != nil {
		return fmt.Errorf("register request: %w", err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)

	if resp.StatusCode >= 400 {
		var e struct{ Error string `json:"error"` }
		_ = json.Unmarshal(raw, &e)
		if e.Error != "" {
			return errors.New(e.Error)
		}
		return fmt.Errorf("register failed: %s", resp.Status)
	}
	return nil
}

// RedeemInvite calls POST /auth/invite on the relay to consume an invite code.
func (c *Client) RedeemInvite(ctx context.Context, code, deviceUserID string) error {
	body, _ := json.Marshal(map[string]string{
		"code":           code,
		"device_user_id": deviceUserID,
	})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.BaseURL+"/auth/invite", bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.HTTP.Do(req)
	if err != nil {
		return fmt.Errorf("invite request: %w", err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)

	if resp.StatusCode >= 400 {
		var e struct{ Error string `json:"error"` }
		_ = json.Unmarshal(raw, &e)
		if e.Error != "" {
			return errors.New(e.Error)
		}
		return fmt.Errorf("invite failed: %s", resp.Status)
	}
	return nil
}
