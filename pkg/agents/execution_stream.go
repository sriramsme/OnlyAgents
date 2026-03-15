package agents

import (
	"context"
	"fmt"
	"strings"

	"github.com/sriramsme/OnlyAgents/pkg/core"
	"github.com/sriramsme/OnlyAgents/pkg/llm"
	"github.com/sriramsme/OnlyAgents/pkg/logger"
	"github.com/sriramsme/OnlyAgents/pkg/tools"
)

func (a *Agent) executeStream(
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
		phase := fmt.Sprintf("%s_llm_stream_%d. Msg: %s", a.id, llmCallCount, truncate(payload.Message, 100))
		logger.Timing.StartPhase(correlationID, phase)

		// ── Consume the stream ────────────────────────────────────────────────
		streamCh := a.llmClient.ChatStream(ctx, &llm.Request{
			Messages: messages,
			Tools:    a.tools,
			Metadata: map[string]string{
				"agent_id":       a.id,
				"correlation_id": correlationID,
			},
		})

		fullContent, toolCalls, inputTokens, outputTokens, streamErr := collectToolCalls(
			streamCh,
			func(token, accumulated string) {
				a.safeSend(core.Event{
					Type:          core.OutboundToken,
					CorrelationID: correlationID,
					AgentID:       a.id,
					Payload: core.OutboundTokenPayload{
						Channel:            payload.Channel,
						Token:              token,
						AccumulatedContent: accumulated,
					},
				}, "agent token")
			},
		)

		if streamErr != nil {
			logger.Timing.EndPhaseWithMetadata(correlationID, phase, map[string]any{"error": "stream failed"})
			return "", fmt.Errorf("llm stream failed: %w", streamErr)
		}

		// Reconstruct a *Response so the rest of the loop is identical to execute.
		stopReason := "end_turn"
		if len(toolCalls) > 0 {
			stopReason = "tool_use"
		}
		resp := &llm.Response{
			Content:    fullContent,
			ToolCalls:  toolCalls,
			StopReason: stopReason,
			Usage: llm.Usage{
				InputTokens:  inputTokens,
				OutputTokens: outputTokens,
				TotalTokens:  inputTokens + outputTokens,
			},
		}

		logger.Timing.EndPhaseWithMetadata(correlationID, phase, map[string]any{
			"model":            a.llmClient.Model(),
			"agent":            a.name,
			"input_tokens":     resp.Usage.InputTokens,
			"output_tokens":    resp.Usage.OutputTokens,
			"total_tokens":     resp.Usage.TotalTokens,
			"stop_reason":      resp.StopReason,
			"tool_calls_count": len(resp.ToolCalls),
		})

		if !resp.HasToolCalls() {
			a.finaliseTurn(ctx, sessionID, correlationID, payload, resp)
			if resp.Content == "" {
				// LLM returned empty content — send a fallback so user isn't left waiting
				a.safeSend(core.Event{
					Type:          core.OutboundMessage,
					CorrelationID: correlationID,
					AgentID:       a.id,
					Payload: core.OutboundMessagePayload{
						Channel: payload.Channel,
						Content: "I completed the task but had nothing to add.",
					},
				}, "empty response fallback")
			}
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

func (a *Agent) shouldStream(payload core.AgentExecutePayload) bool {
	if !a.streamingEnabled {
		return false
	}
	// Executive always streams — response goes directly to user
	if a.isExecutive {
		return true
	}
	// Sub-agent only streams if responding directly to user
	if payload.Delegation != nil {
		return payload.Delegation.SendDirectlyToUser
	}
	// Workflow tasks go back to engine — no streaming
	return false
}

func collectToolCalls(
	streamCh <-chan llm.StreamChunk,
	onToken func(token, accumulated string), // nil = no-op
) (content string, toolCalls []tools.ToolCall, inputTokens, outputTokens int, err error) {
	var fullContent strings.Builder
	builders := map[string]*toolCallBuilder{}
	var order []string

	for chunk := range streamCh {
		if chunk.Error != nil {
			err = chunk.Error
			return
		}
		if chunk.Content != "" {
			fullContent.WriteString(chunk.Content)
			if onToken != nil {
				onToken(chunk.Content, fullContent.String())
			}
		}
		for _, tc := range chunk.ToolCalls {
			b, ok := builders[tc.ID]
			if !ok {
				b = &toolCallBuilder{ID: tc.ID, Name: tc.Function.Name}
				builders[tc.ID] = b
				order = append(order, tc.ID)
			}
			b.Args.WriteString(tc.Function.Arguments)
		}
		if chunk.Done {
			inputTokens = chunk.Usage.InputTokens
			outputTokens = chunk.Usage.OutputTokens
			break
		}
	}

	content = fullContent.String()
	for _, id := range order {
		b := builders[id]
		toolCalls = append(toolCalls, tools.ToolCall{
			ID:       b.ID,
			Function: tools.FunctionCall{Name: b.Name, Arguments: b.Args.String()},
		})
	}
	return
}
