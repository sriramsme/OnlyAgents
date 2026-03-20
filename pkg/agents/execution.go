package agents

import (
	"context"
	"fmt"
	"os"

	"github.com/sriramsme/OnlyAgents/pkg/core"
	"github.com/sriramsme/OnlyAgents/pkg/llm"
	"github.com/sriramsme/OnlyAgents/pkg/logger"
	"github.com/sriramsme/OnlyAgents/pkg/media"
	"github.com/sriramsme/OnlyAgents/pkg/memory"
)

// llmCaller is the function signature shared by callLLM and callLLMStream.
// callCount is the 1-based iteration index within the current execution loop,
// used for phase timing labels.
type llmCaller func(
	ctx context.Context,
	msgs []llm.Message,
	correlationID string,
	callCount int,
	payload core.AgentExecutePayload,
) (*llm.Response, error)

// execute runs the agent using a blocking LLM call.
func (a *Agent) execute(
	ctx context.Context,
	payload core.AgentExecutePayload,
	correlationID string,
) (string, []*media.Attachment, error) {
	return a.runExecutionLoop(ctx, payload, correlationID, a.callLLM)
}

// runExecutionLoop is the shared execution engine for both streaming and
// non-streaming paths. Accumulates agent-produced file attachments across
// all tool call rounds and returns them alongside the final response.
func (a *Agent) runExecutionLoop(
	ctx context.Context,
	payload core.AgentExecutePayload,
	correlationID string,
	caller llmCaller,
) (string, []*media.Attachment, error) {
	if payload.Channel == nil {
		return "", nil, fmt.Errorf("execute: nil channel in payload (correlation_id: %s)", correlationID)
	}

	// Serialize turns for this agent+session — prevents turn 2 reading
	// history before turn 1 has finished writing all its tool results.
	lock := a.turnLockFor(payload.Channel.SessionID)
	lock.Lock()
	defer lock.Unlock()

	sessionID, messages, err := a.prepareExecution(ctx, payload, correlationID)
	if err != nil {
		return "", nil, err
	}

	// Accumulate agent-produced files across all tool call rounds.
	var outboundAttachments []*media.Attachment

	callCount := 0
	for {
		select {
		case <-ctx.Done():
			return "", nil, fmt.Errorf("request cancelled: %w", ctx.Err())
		default:
		}

		callCount++
		resp, err := caller(ctx, messages, correlationID, callCount, payload)
		if err != nil {
			return "", nil, err
		}

		if !resp.HasToolCalls() {
			a.finaliseTurn(ctx, sessionID, correlationID, payload, resp)
			if resp.Content == "" {
				// LLM returned empty content — send a fallback so the user isn't left waiting.
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
			return resp.Content, outboundAttachments, nil
		}

		var produced []*media.Attachment
		var halt bool
		messages, produced, halt, err = a.processToolCalls(ctx, sessionID, correlationID, payload, messages, resp)
		if err != nil {
			return "", nil, err
		}

		outboundAttachments = append(outboundAttachments, produced...)

		if halt {
			return "", outboundAttachments, nil
		}
	}
}

// callLLM performs a single blocking LLM call and returns a unified Response.
func (a *Agent) callLLM(
	ctx context.Context,
	msgs []llm.Message,
	correlationID string,
	callCount int,
	payload core.AgentExecutePayload,
) (*llm.Response, error) {
	phase := fmt.Sprintf("%s_llm_%d. Msg: %s", a.id, callCount, truncate(payload.Message, 100))
	logger.Timing.StartPhase(correlationID, phase)

	resp, err := a.llmClient.Chat(ctx, &llm.Request{
		Messages: msgs,
		Tools:    a.tools,
		Metadata: map[string]string{
			"agent_id":       a.id,
			"correlation_id": correlationID,
		},
	})
	if err != nil {
		logger.Timing.EndPhaseWithMetadata(correlationID, phase, map[string]any{"error": "llm call failed"})
		return nil, fmt.Errorf("llm call failed: %w", err)
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

	return resp, nil
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

	userMsg, err := buildUserMessage(payload.Message, payload.Attachments)
	if err != nil {
		a.logger.Warn("failed to build user message", "err", err)
		userMsg = llm.UserMessage(payload.Message) // safe fallback
	}

	messages = append(messages, userMsg)
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

// buildUserMessage constructs the user turn for the current request.
// If the payload carries attachments, it returns a multimodal message with
// one text part and one part per supported attachment.
// Text-only payloads return a plain Message, identical to before.
func buildUserMessage(text string, attachments []*media.Attachment) (llm.Message, error) {
	if len(attachments) == 0 {
		return llm.UserMessage(text), nil
	}

	parts := make([]llm.ContentPart, 0, 1+len(attachments))

	// Text always comes first — models expect it leading the parts list.
	if text != "" {
		parts = append(parts, llm.TextPart(text))
	}

	for _, att := range attachments {
		if !media.IsSupportedByLLM(att.MIMEType) {
			// File type the LLM can't reason about (e.g. zip, video).
			// Include a text note so the model knows a file was present.
			parts = append(parts, llm.TextPart(
				fmt.Sprintf("[Attached file: %s (%s) — not directly readable by the model]",
					att.Filename, att.MIMEType),
			))
			continue
		}

		data, err := os.ReadFile(att.LocalPath)
		if err != nil {
			// Don't fail the whole turn — log is handled by caller, note it inline.
			parts = append(parts, llm.TextPart(
				fmt.Sprintf("[Attached file: %s — could not be read]", att.Filename),
			))
			continue
		}

		switch {
		case att.IsImage():
			parts = append(parts, llm.ImagePart(att.Filename, att.MIMEType, data))
		case att.IsDocument():
			parts = append(parts, llm.DocumentPart(att.Filename, att.MIMEType, data))
		}
	}

	return llm.UserMessageWithParts(parts), nil
}
