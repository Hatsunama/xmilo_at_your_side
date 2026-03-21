package openai

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"xmilo/relay-go/shared/contracts"
)

const defaultXAIBaseURL = "https://api.x.ai/v1"

type Client struct {
	APIKey  string
	Model   string
	BaseURL string
	HTTP    *http.Client
}

func New(apiKey, model, baseURL string) *Client {
	if strings.TrimSpace(baseURL) == "" {
		baseURL = defaultXAIBaseURL
	}
	return &Client{
		APIKey:  apiKey,
		Model:   model,
		BaseURL: strings.TrimRight(baseURL, "/"),
		HTTP:    &http.Client{Timeout: 10 * time.Minute},
	}
}

func (c *Client) Turn(ctx context.Context, req contracts.RelayTurnRequest) (contracts.RelayTurnResponse, error) {
	if c.APIKey == "" {
		return contracts.RelayTurnResponse{}, errors.New("XAI_API_KEY is not configured")
	}

	rawBody, err := json.Marshal(buildResponsesBody(req, c.Model))
	if err != nil {
		return contracts.RelayTurnResponse{}, err
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.BaseURL+"/responses", bytes.NewReader(rawBody))
	if err != nil {
		return contracts.RelayTurnResponse{}, err
	}
	httpReq.Header.Set("Authorization", "Bearer "+c.APIKey)
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := c.HTTP.Do(httpReq)
	if err != nil {
		return contracts.RelayTurnResponse{}, err
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return contracts.RelayTurnResponse{}, err
	}
	if resp.StatusCode >= 400 {
		return contracts.RelayTurnResponse{}, fmt.Errorf("xai responses error: %s: %s", resp.Status, string(raw))
	}

	text, err := extractOutputText(raw)
	if err != nil {
		return contracts.RelayTurnResponse{}, err
	}

	var out contracts.RelayTurnResponse
	if err := json.Unmarshal([]byte(text), &out); err != nil {
		return contracts.RelayTurnResponse{}, fmt.Errorf("parse model json: %w; output=%s", err, text)
	}
	return out, nil
}

func buildResponsesBody(req contracts.RelayTurnRequest, model string) map[string]any {
	messages := []map[string]any{
		{
			"role": "system",
			"content": []map[string]string{
				{"type": "input_text", "text": req.SystemPrompt},
			},
		},
		{
			"role": "user",
			"content": []map[string]string{
				{"type": "input_text", "text": buildPrompt(req)},
			},
		},
	}

	body := map[string]any{
		"model": model,
		"store": false,
		"input": messages,
		"text": map[string]any{
			"format": map[string]any{
				"type": "json_object",
			},
		},
	}

	return sanitizeForXAI(body)
}

func sanitizeForXAI(body map[string]any) map[string]any {
	forbidden := []string{
		"presence_penalty",
		"frequency_penalty",
		"stop",
		"reasoning_effort",
		"logprobs",
		"top_logprobs",
		"temperature",
	}
	for _, key := range forbidden {
		delete(body, key)
	}
	return body
}

func buildPrompt(req contracts.RelayTurnRequest) string {
	var b strings.Builder
	b.WriteString("Return JSON only. The word JSON is mandatory.\n")
	b.WriteString("You are the relay planner for Milo.\n")
	b.WriteString("Generate a JSON object with keys: intent, target_room, thought_text, summary, report_text, requires_user_choice, choices.\n")
	b.WriteString("Use concise but useful values. Do not include extra keys.\n")
	b.WriteString("Phase: " + req.Phase + "\n")
	b.WriteString("Prompt: " + req.Prompt + "\n")
	return b.String()
}

func extractOutputText(raw []byte) (string, error) {
	var payload map[string]any
	if err := json.Unmarshal(raw, &payload); err != nil {
		return "", err
	}

	output, _ := payload["output"].([]any)
	for _, item := range output {
		msg, _ := item.(map[string]any)
		contents, _ := msg["content"].([]any)
		for _, content := range contents {
			piece, _ := content.(map[string]any)
			if piece["type"] == "output_text" {
				if text, ok := piece["text"].(string); ok {
					return text, nil
				}
			}
		}
	}
	return "", errors.New("no output_text found in responses payload")
}
