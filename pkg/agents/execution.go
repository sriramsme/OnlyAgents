package agents

import (
	"context"
	"fmt"

	"github.com/sriramsme/OnlyAgents/pkg/core"
	"github.com/sriramsme/OnlyAgents/pkg/llm"
	"github.com/sriramsme/OnlyAgents/pkg/logger"
	"github.com/sriramsme/OnlyAgents/pkg/memory"
)

// nolint:gocyclo
// execute is the non-streaming internal LLM loop, shared by both sync and async paths.
func (a *Agent) execute(
	ctx context.Context,
	payload core.AgentExecutePayload,
	correlationID string,
) (string, error) {
	if payload.Channel == nil {
		return "", fmt.Errorf("execute: nil channel in payload (correlation_id: %s)", correlationID)
	}
	sessionID := payload.Channel.SessionID

	// Serialize turns for this agent+session — prevents turn 2 reading
	// history before turn 1 has finished writing all its tool results
	lock := a.turnLockFor(sessionID)
	lock.Lock()
	defer lock.Unlock()

	sessionID, messages, err := a.prepareExecution(ctx, payload, correlationID)
	if err != nil {
		return "", err
	}

	llmCallCount := 0
	var halt bool
	for {
		select {
		case <-ctx.Done():
			return "", fmt.Errorf("request cancelled: %w", ctx.Err())
		default:
		}

		llmCallCount++
		phase := fmt.Sprintf("%s_llm_%d. Msg: %s", a.id, llmCallCount, truncate(payload.Message, 100))
		logger.Timing.StartPhase(correlationID, phase)

		resp, err := a.llmClient.Chat(ctx, &llm.Request{
			Messages: messages,
			Tools:    a.tools,
			Metadata: map[string]string{
				"agent_id":       a.id,
				"correlation_id": correlationID,
			},
		})
		if err != nil {
			logger.Timing.EndPhaseWithMetadata(correlationID, phase, map[string]any{"error": "failed"})
			return "", fmt.Errorf("llm request failed: %w", err)
		}

		logger.Timing.EndPhaseWithMetadata(correlationID, phase, map[string]any{
			"model":            a.llmClient.Model(),
			"agent":            a.name,
			"input_tokens":     resp.Usage.InputTokens,
			"output_tokens":    resp.Usage.OutputTokens,
			"cached_tokens":    resp.Usage.CachedTokens,
			"total_tokens":     resp.Usage.TotalTokens,
			"stop_reason":      resp.StopReason,
			"tool_calls_count": len(resp.ToolCalls),
		})

		if !resp.HasToolCalls() {
			a.finaliseTurn(ctx, sessionID, correlationID, payload, resp)
			return resp.Content, nil
		}
		var processErr error
		messages, halt, processErr = a.processToolCalls(ctx, sessionID, correlationID, payload, messages, resp)
		if processErr != nil {
			return "", processErr
		}
		if halt {
			return "", nil
		}
	}
}

// prepareExecution loads history and builds the initial messages slice.
// Called by both execute and executeStream at the start of each request.
func (a *Agent) prepareExecution(
	ctx context.Context,
	payload core.AgentExecutePayload,
	correlationID string,
) (sessionID string, messages []llm.Message, err error) {
	sessionID = payload.Channel.SessionID

	if err = a.cm.SaveUserMessage(ctx, sessionID, a.id, payload.Message); err != nil {
		a.logger.Warn("failed to save user message", "err", err, "correlation_id", correlationID)
	}

	history, histErr := a.cm.GetHistory(ctx, sessionID, a.id, memory.DefaultHistoryTurns)
	if histErr != nil {
		a.logger.Warn("failed to load history, falling back to empty", "err", histErr)
		history = []llm.Message{}
	}

	memCtx, memErr := a.mm.GetRelevantMemory(ctx, a.id, payload.Message)
	if memErr != nil {
		a.logger.Warn("failed to load memory context", "err", memErr)
	}

	messages = make([]llm.Message, 0, len(history)+2)
	messages = append(messages, llm.SystemMessage(a.systemPrompt))
	if formatted := memory.FormatMemoryContext(memCtx); formatted != "" {
		messages = append(messages, llm.SystemMessage(formatted))
	}
	messages = append(messages, history...)

	return sessionID, messages, nil
}

// finaliseTurn persists the final assistant response and fires workflow
// completion if applicable. Called when there are no tool calls.
func (a *Agent) finaliseTurn(
	ctx context.Context,
	sessionID string,
	correlationID string,
	payload core.AgentExecutePayload,
	resp *llm.Response,
) {
	if err := a.cm.SaveAssistantMessage(ctx, sessionID, a.id, resp.Content, resp.ReasoningContent, nil); err != nil {
		a.logger.Warn("failed to save assistant message", "err", err, "correlation_id", correlationID)
	}
	if payload.Workflow != nil {
		a.fireTaskCompletion(ctx, correlationID, payload.Workflow, resp.Content, nil)
	}
}
