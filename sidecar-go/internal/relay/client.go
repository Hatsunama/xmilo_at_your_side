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

	var out contracts.SessionStartResponse
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
		var e struct {
			Error string `json:"error"`
		}
		_ = json.Unmarshal(raw, &e)
		if e.Error != "" {
			return errors.New(e.Error)
		}
		return fmt.Errorf("register failed: %s", resp.Status)
	}
	return nil
}

// RedeemInvite calls POST /auth/invite on the relay to consume an access code.
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
		var e struct {
			Error string `json:"error"`
		}
		_ = json.Unmarshal(raw, &e)
		if e.Error != "" {
			return errors.New(e.Error)
		}
		return fmt.Errorf("invite failed: %s", resp.Status)
	}
	return nil
}

func (c *Client) requestAuthedJSON(ctx context.Context, method, path string, body any, out any) error {
	jwt, err := c.JWT()
	if err != nil {
		return err
	}

	var reader io.Reader = http.NoBody
	if body != nil {
		raw, err := json.Marshal(body)
		if err != nil {
			return err
		}
		reader = bytes.NewReader(raw)
	}

	req, err := http.NewRequestWithContext(ctx, method, c.BaseURL+path, reader)
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	if jwt != "" {
		req.Header.Set("Authorization", "Bearer "+jwt)
	}

	resp, err := c.HTTP.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	if resp.StatusCode >= 400 {
		var e struct {
			Error string `json:"error"`
		}
		_ = json.Unmarshal(raw, &e)
		if e.Error != "" {
			return errors.New(e.Error)
		}
		return fmt.Errorf("%s failed: %s", path, resp.Status)
	}
	if out == nil {
		return nil
	}
	return json.Unmarshal(raw, out)
}

func (c *Client) TwoFactorStatus(ctx context.Context) (map[string]any, error) {
	var out map[string]any
	err := c.requestAuthedJSON(ctx, http.MethodGet, "/auth/2fa/status", nil, &out)
	return out, err
}

func (c *Client) TwoFactorBegin(ctx context.Context) (map[string]any, error) {
	var out map[string]any
	err := c.requestAuthedJSON(ctx, http.MethodPost, "/auth/2fa/setup/begin", map[string]any{}, &out)
	return out, err
}

func (c *Client) TwoFactorConfirm(ctx context.Context, code string) (map[string]any, error) {
	var out map[string]any
	err := c.requestAuthedJSON(ctx, http.MethodPost, "/auth/2fa/setup/confirm", map[string]string{"code": code}, &out)
	return out, err
}

func (c *Client) TwoFactorVerify(ctx context.Context, code, recoveryCode string) (map[string]any, error) {
	var out map[string]any
	err := c.requestAuthedJSON(ctx, http.MethodPost, "/auth/2fa/verify", map[string]string{
		"code":          code,
		"recovery_code": recoveryCode,
	}, &out)
	return out, err
}

func (c *Client) TwoFactorDisable(ctx context.Context, code, recoveryCode string) (map[string]any, error) {
	var out map[string]any
	err := c.requestAuthedJSON(ctx, http.MethodPost, "/auth/2fa/disable", map[string]string{
		"code":          code,
		"recovery_code": recoveryCode,
	}, &out)
	return out, err
}

func (c *Client) TwoFactorRegenerateRecoveryCodes(ctx context.Context) (map[string]any, error) {
	var out map[string]any
	err := c.requestAuthedJSON(ctx, http.MethodPost, "/auth/2fa/recovery/regenerate", map[string]any{}, &out)
	return out, err
}

func (c *Client) CreateWebsiteHandoff(ctx context.Context) (map[string]any, error) {
	var out map[string]any
	err := c.requestAuthedJSON(ctx, http.MethodPost, "/auth/website/handoff/create", map[string]any{}, &out)
	return out, err
}

func (c *Client) DeleteAccount(ctx context.Context) error {
	return c.requestAuthedJSON(ctx, http.MethodPost, "/auth/account/delete", map[string]any{}, nil)
}

func (c *Client) SubmitAIReport(ctx context.Context, payload map[string]any) error {
	return c.requestAuthedJSON(ctx, http.MethodPost, "/report/ai", payload, nil)
}
