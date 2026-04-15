package agents

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/sriramsme/OnlyAgents/pkg/tools"
)

func (a *Agent) handleRecall(ctx context.Context, sessionID string, tc tools.ToolCall) tools.ToolExecution {
	var input tools.RecallInput
	if err := json.Unmarshal([]byte(tc.Function.Arguments), &input); err != nil {
		return tools.ExecErr(fmt.Errorf("invalid recall args: %w", err))
	}
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	result, err := a.memManager.Recall(ctx, input.Query)
	if err != nil {
		return tools.ExecErr(err)
	}

	return tools.ExecOK(result)
}

func (a *Agent) handleRemember(ctx context.Context, sessionID string, tc tools.ToolCall) tools.ToolExecution {
	var input tools.RememberInput
	if err := json.Unmarshal([]byte(tc.Function.Arguments), &input); err != nil {
		return tools.ExecErr(fmt.Errorf("invalid remember args: %w", err))
	}

	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	err := a.memManager.Remember(ctx, input)
	if err != nil {
		return tools.ExecErr(err)
	}

	return tools.ExecOK(map[string]any{
		"status": "ok",
	})
}
