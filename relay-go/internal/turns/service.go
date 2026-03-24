package turns

import (
	"context"
	"time"

	"xmilo/relay-go/internal/db"
	llmclient "xmilo/relay-go/internal/openai"
	"xmilo/relay-go/shared/contracts"
)

type Service struct {
	Store *db.Store
	LLM   *llmclient.Client
}

func (s *Service) Execute(ctx context.Context, req contracts.RelayTurnRequest, deviceUserID string) (contracts.RelayTurnResponse, error) {
	resp, err := s.LLM.Turn(ctx, req)
	if err != nil {
		return contracts.RelayTurnResponse{}, err
	}

	_, _ = s.Store.Pool.Exec(ctx, `
        INSERT INTO llm_turns(device_user_id, task_id, phase, prompt, intent, target_room, created_at)
        VALUES(NULLIF($1, ''), $2, $3, $4, $5, $6, $7)
    `, deviceUserID, req.TaskID, req.Phase, req.Prompt, resp.Intent, resp.TargetRoom, time.Now().UTC())

	return resp, nil
}
