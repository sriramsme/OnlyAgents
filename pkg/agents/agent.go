// Agent execution model:
//
//  Sync path  (HTTP handler → Execute):
//    Execute() builds messages, calls LLM, fires ToolCallRequest events to kernel,
//    blocks on reply channel until kernel returns ToolCallResult, then resumes LLM loop.
//
//  Async path (A2A / kernel → agent):
//    Kernel sends AgentExecute event to agent.inbox.
//    processEvents() picks it up and calls execute() internally.
//    Outbound response goes back as OutboundMessage event → kernel → channel.

package agents

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/sriramsme/OnlyAgents/internal/config"
	"github.com/sriramsme/OnlyAgents/pkg/core"
	"github.com/sriramsme/OnlyAgents/pkg/llm"
	"github.com/sriramsme/OnlyAgents/pkg/soul"
	"github.com/sriramsme/OnlyAgents/pkg/tools"
)

// NewAgent creates an agent. Kernel calls this and injects the shared bus + tool definitions.
func NewAgent(
	ctx context.Context, // ← Parent context (kernel's context)
	cfg config.Config,
	llmClient llm.Client,
	tools []tools.ToolDef,
	outbox chan<- core.Event,
) (*Agent, error) {
	if llmClient == nil {
		return nil, fmt.Errorf("llm client is required")
	}

	agentSoul := soul.NewSoul(cfg.Soul)

	userConfigPath := "configs/user.yaml"
	userCfg, err := config.LoadUserConfig(userConfigPath)
	if err != nil {
		return nil, fmt.Errorf("load user config: %w", err)
	}
	// Create agent context from parent - ties agent lifecycle to kernel
	agentCtx, cancel := context.WithCancel(ctx)

	return &Agent{
		id:             cfg.ID,
		name:           cfg.Name,
		isExecutive:    cfg.IsExecutive,
		isGeneral:      cfg.IsGeneral,
		maxConcurrency: cfg.MaxConcurrency,
		skills:         cfg.Skills,
		llmClient:      llmClient,
		soul:           agentSoul,
		user:           userCfg,
		tools:          tools,
		outbox:         outbox,
		inbox:          make(chan core.Event, cfg.BufferSize),
		ctx:            agentCtx,
		cancel:         cancel,
		logger:         slog.With("agent_id", cfg.ID),
	}, nil
}

// --- Lifecycle ---

func (a *Agent) Start() error {
	a.logger.Info("starting agent", "model", a.llmClient.Model())
	a.wg.Add(2)
	go a.processEvents()
	go a.healthCheck()
	return nil
}

func (a *Agent) Stop() error {
	a.logger.Info("stopping agent")

	// Cancel context to signal shutdown
	a.cancel()

	done := make(chan struct{})
	go func() {
		a.wg.Wait()
		close(done)
	}()

	// Wait with timeout
	timeout := 10 * time.Second
	select {
	case <-done:
		a.logger.Info("agent stopped gracefully")
		return nil
	case <-time.After(timeout):
		a.logger.Error("agent shutdown timeout",
			"timeout", timeout,
			"warning", "goroutines may still be running - check for blocked LLM calls or stuck tool executions")
		return fmt.Errorf("agent %s shutdown timeout after %v", a.id, timeout)
	}
}

// Inbox returns the channel kernel sends events to.
func (a *Agent) Inbox() chan<- core.Event {
	return a.inbox
}

// ID returns agent ID.
func (a *Agent) ID() string { return a.id }

func (a *Agent) IsExecutive() bool { return a.isExecutive }

func (a *Agent) IsGeneral() bool { return a.isGeneral }

func (a *Agent) GetSkillNames() []string { return a.skills }

func (a *Agent) SetTools(tools []tools.ToolDef) { a.tools = tools }

// --- Async event loop ---

func (a *Agent) processEvents() {
	defer a.wg.Done()
	for {
		select {
		case evt := <-a.inbox:
			a.handleEvent(evt)
		case <-a.ctx.Done():
			a.logger.Info("event processor shutting down")
			return
		}
	}
}

func (a *Agent) handleEvent(evt core.Event) {
	switch evt.Type {
	case core.AgentExecute:

		payload, ok := evt.Payload.(core.AgentExecutePayload)
		if !ok {
			a.logger.Error("invalid AgentExecute payload",
				"actual_type", fmt.Sprintf("%T", evt.Payload),
				"correlation_id", evt.CorrelationID)
			return
		}

		requestCtx, cancel := context.WithTimeout(a.ctx, 5*time.Minute)
		defer cancel()

		// Add correlation ID to context for tracing
		// (In production, use opentelemetry or similar)
		// requestCtx = context.WithValue(requestCtx, "correlation_id", evt.CorrelationID)

		a.logger.Debug("processing agent execute event",
			"correlation_id", evt.CorrelationID,
			"message_length", len(payload.UserMessage))
		response, err := a.execute(requestCtx, payload.UserMessage, evt.CorrelationID)
		if err != nil {
			a.logger.Error("execute failed",
				"error", err,
				"correlation_id", evt.CorrelationID)

			// Send error response if this is a sync request
			if evt.ReplyTo != nil {
				errorEvt := core.Event{
					Type:          core.AgentExecute,
					CorrelationID: evt.CorrelationID,
					Payload:       fmt.Sprintf("Error: %v", err),
				}
				a.safeReply(evt.ReplyTo, errorEvt, "execute error")
			}
			return
		}
		if evt.ReplyTo != nil {
			// HTTP path — reply directly
			replyEvt := core.Event{
				Type:          core.AgentExecute,
				CorrelationID: evt.CorrelationID,
				Payload:       response,
			}
			a.safeReply(evt.ReplyTo, replyEvt, "http response")
		} else {
			// Async path (Telegram etc.) — fire OutboundMessage
			outboundEvt := core.Event{
				Type:          core.OutboundMessage,
				CorrelationID: evt.CorrelationID,
				Payload: core.OutboundMessagePayload{
					ChannelName: payload.Metadata["channel"],
					ChatID:      payload.ChatID,
					Content:     response,
				},
			}
			a.safeSend(outboundEvt, "outbound message")
		}

	default:
		a.logger.Warn("unhandled event type",
			"type", evt.Type,
			"correlation_id", evt.CorrelationID)
	}
}

// safeReply sends to a reply channel with timeout and context checks
func (a *Agent) safeReply(replyCh chan<- core.Event, evt core.Event, description string) {
	select {
	case replyCh <- evt:
		// Success
	case <-time.After(5 * time.Second):
		a.logger.Error("failed to send reply - timeout",
			"description", description,
			"correlation_id", evt.CorrelationID)
	case <-a.ctx.Done():
		a.logger.Info("failed to send reply - agent shutting down",
			"description", description,
			"correlation_id", evt.CorrelationID)
	}
}

// safeSend sends to outbox with timeout and context checks
func (a *Agent) safeSend(evt core.Event, description string) {
	select {
	case a.outbox <- evt:
		// Success
	case <-time.After(5 * time.Second):
		a.logger.Error("failed to send to outbox - timeout",
			"description", description,
			"event_type", evt.Type,
			"correlation_id", evt.CorrelationID)
	case <-a.ctx.Done():
		a.logger.Info("failed to send to outbox - agent shutting down",
			"description", description,
			"event_type", evt.Type,
			"correlation_id", evt.CorrelationID)
	}
}

// --- Sync HTTP path ---

// Execute is called directly by HTTP handlers (sync request/response).
func (a *Agent) Execute(ctx context.Context, userMessage string) (string, error) {
	return a.execute(ctx, userMessage, uuid.NewString())
}

// execute is the internal LLM loop, shared by both sync and async paths.
func (a *Agent) execute(ctx context.Context, userMessage string, correlationID string) (string, error) {
	a.logger.Debug("executing",
		"message_length", len(userMessage),
		"correlation_id", correlationID)

	messages := []llm.Message{
		llm.SystemMessage(a.soul.SystemPrompt(ctx)),
		llm.SystemMessage(a.formatUserProfile()),
		llm.UserMessage(userMessage),
	}

	for {
		// Check context before making LLM call
		select {
		case <-ctx.Done():
			return "", fmt.Errorf("request cancelled: %w", ctx.Err())
		default:
		}

		resp, err := a.llmClient.Chat(ctx, &llm.Request{
			Messages: messages,
			Tools:    a.tools,
			Metadata: map[string]string{
				"agent_id":       a.id,
				"correlation_id": correlationID,
			},
		})
		if err != nil {
			return "", fmt.Errorf("llm request failed: %w", err)
		}

		a.logger.Debug("llm response",
			"stop_reason", resp.StopReason,
			"tool_calls", len(resp.ToolCalls),
			"tokens", resp.Usage.TotalTokens,
			"correlation_id", correlationID)

		if !resp.HasToolCalls() {
			return resp.Content, nil
		}

		// Add assistant turn
		messages = append(messages, llm.AssistantMessageWithTools(
			resp.Content, resp.ReasoningContent, resp.ToolCalls,
		))

		// Execute each tool call via kernel
		for _, tc := range resp.ToolCalls {
			result, err := a.requestToolCall(ctx, correlationID, tc)
			if err != nil {
				return "", fmt.Errorf("tool call %s failed: %w", tc.Function.Name, err)
			}

			resultJSON, err := json.Marshal(result)
			if err != nil {
				return "", fmt.Errorf("marshal tool result: %w", err)
			}
			messages = append(messages, llm.ToolResultMessage(
				tc.ID, tc.Function.Name, string(resultJSON),
			))
		}
		// Loop: LLM will now see tool results and either call more tools or return final response
	}
}

// requestToolCall fires a ToolCallRequest to kernel and blocks until result arrives.
// Uses a per-call reply channel so concurrent tool calls don't mix up results.
func (a *Agent) requestToolCall(ctx context.Context, correlationID string, tc tools.ToolCall) (any, error) {
	args, err := llm.ParseToolArguments(tc.Function.Arguments)
	if err != nil {
		return nil, fmt.Errorf("invalid tool args: %w", err)
	}

	// Kernel will send the result back on this channel
	// Buffer of 1 ensures non-blocking send from kernel
	replyCh := make(chan core.Event, 1)

	event := core.Event{
		Type:          core.ToolCallRequest,
		CorrelationID: correlationID,
		AgentID:       a.id,
		ReplyTo:       replyCh,
		Payload: core.ToolCallRequestPayload{
			ToolCallID: tc.ID,
			SkillName:  skillNameFromTool(tc.Function.Name), // e.g. "email" from "email_send"
			ToolName:   tc.Function.Name,
			Params:     args,
		},
	}

	a.logger.Debug("requesting tool call",
		"tool", tc.Function.Name,
		"correlation_id", correlationID)

	// Safe send with timeout and context checks
	select {
	case a.outbox <- event:
		// Successfully sent request
	case <-ctx.Done():
		return nil, fmt.Errorf("request cancelled: %w", ctx.Err())
	case <-a.ctx.Done():
		return nil, fmt.Errorf("agent shutting down")
	case <-time.After(5 * time.Second):
		return nil, fmt.Errorf("failed to send tool call request (timeout): %s", tc.Function.Name)
	}

	// Wait for result with timeout
	select {
	case resultEvt := <-replyCh:
		result, ok := resultEvt.Payload.(core.ToolCallResultPayload)
		if !ok {
			return nil, fmt.Errorf("invalid tool result payload type: %T", resultEvt.Payload)
		}

		if result.Error != "" {
			return nil, fmt.Errorf("tool call error: %s", result.Error)
		}

		a.logger.Debug("tool call succeeded",
			"tool", tc.Function.Name,
			"correlation_id", correlationID)

		return result.Result, nil

	case <-ctx.Done():
		return nil, fmt.Errorf("request cancelled: %w", ctx.Err())

	case <-a.ctx.Done():
		return nil, fmt.Errorf("agent shutting down")

	case <-time.After(30 * time.Second):
		return nil, fmt.Errorf("tool call timeout: %s", tc.Function.Name)
	}
}

// AskLLM is a helper for skills that need LLM assistance (e.g. drafting text).
func (a *Agent) AskLLM(ctx context.Context, system, prompt string) (string, error) {
	resp, err := a.llmClient.Chat(ctx, &llm.Request{
		Messages: []llm.Message{
			llm.SystemMessage(system),
			llm.UserMessage(prompt),
		},
		Metadata: map[string]string{"agent_id": a.id, "context": "skill_helper"},
	})
	if err != nil {
		return "", err
	}
	return resp.Content, nil
}

func (a *Agent) healthCheck() {
	defer a.wg.Done()
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			a.logger.Debug("health check ok")
		case <-a.ctx.Done():
			a.logger.Info("health check shutting down")
			return
		}
	}
}

func (a *Agent) formatUserProfile() string {
	if a.user == nil {
		return ""
	}
	return fmt.Sprintf(`
=== Who the user is ===
Name: %s (preferred: "%s")
Job: %s
Background: %s
Daily Routine: %s
Values: %s
Technical: %v | Collaboration: %s`,
		a.user.Identity.Name,
		a.user.Identity.PreferredName,
		a.user.Identity.Role,
		a.user.Background.Professional,
		a.user.DailyRoutine,
		strings.Join(a.user.Preferences.WhatIValue, ", "),
		a.user.Preferences.Technical,
		a.user.Preferences.Collaboration,
	)
}

// skillNameFromTool extracts skill name from tool name convention "skillname_action"
// e.g. "email_send" → "email", "calendar_create_event" → "calendar"
func skillNameFromTool(toolName string) string {
	parts := strings.SplitN(toolName, "_", 2)
	if len(parts) > 0 {
		return parts[0]
	}
	return toolName
}
