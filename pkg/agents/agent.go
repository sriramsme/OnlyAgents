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
	"github.com/sriramsme/OnlyAgents/pkg/config"
	"github.com/sriramsme/OnlyAgents/pkg/core"
	"github.com/sriramsme/OnlyAgents/pkg/llm"
	"github.com/sriramsme/OnlyAgents/pkg/soul"
	"github.com/sriramsme/OnlyAgents/pkg/tools"
)

// NewAgent creates an agent. Kernel calls this and injects the shared bus + tool definitions.
func NewAgent(cfg config.Config, llmClient llm.Client, tools []tools.ToolDef, outbox chan<- core.Event) (*Agent, error) {
	if llmClient == nil {
		return nil, fmt.Errorf("llm client is required")
	}

	agentSoul := soul.NewSoul(cfg.Soul)

	userConfigPath := "configs/user.yaml"
	userCfg, err := config.LoadUserConfig(userConfigPath)
	if err != nil {
		return nil, fmt.Errorf("load user config: %w", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	return &Agent{
		id:             cfg.ID,
		name:           cfg.Name,
		isExecutive:    cfg.IsExecutive,
		maxConcurrency: cfg.MaxConcurrency,
		llmClient:      llmClient,
		soul:           agentSoul,
		user:           userCfg,
		tools:          tools,
		outbox:         outbox,
		inbox:          make(chan core.Event, cfg.BufferSize),
		ctx:            ctx,
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
	a.cancel()

	done := make(chan struct{})
	go func() {
		a.wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		return nil
	case <-time.After(5 * time.Second):
		return fmt.Errorf("agent %s shutdown timeout", a.id)
	}
}

// Inbox returns the channel kernel sends events to.
func (a *Agent) Inbox() chan<- core.Event {
	return a.inbox
}

// ID returns agent ID.
func (a *Agent) ID() string { return a.id }

// --- Async event loop ---

func (a *Agent) processEvents() {
	defer a.wg.Done()
	for {
		select {
		case evt := <-a.inbox:
			a.handleEvent(evt)
		case <-a.ctx.Done():
			return
		}
	}
}

func (a *Agent) handleEvent(evt core.Event) {
	switch evt.Type {
	case core.AgentExecute:
		payload := evt.Payload.(core.AgentExecutePayload)
		response, err := a.execute(a.ctx, payload.UserMessage, evt.CorrelationID)
		if err != nil {
			a.logger.Error("execute failed", "error", err)
			return
		}

		if evt.ReplyTo != nil {
			// HTTP path — reply directly
			evt.ReplyTo <- core.Event{
				Type:          core.AgentExecute,
				CorrelationID: evt.CorrelationID,
				Payload:       response,
			}
		} else {
			// Async path (Telegram etc.) — fire OutboundMessage
			a.outbox <- core.Event{
				Type:          core.OutboundMessage,
				CorrelationID: evt.CorrelationID,
				Payload: core.OutboundMessagePayload{
					ChannelName: payload.Metadata["channel"],
					ChatID:      payload.ChatID,
					Content:     response,
				},
			}
		}
	}
}

// --- Sync HTTP path ---

// Execute is called directly by HTTP handlers (sync request/response).
func (a *Agent) Execute(ctx context.Context, userMessage string) (string, error) {
	return a.execute(ctx, userMessage, uuid.NewString())
}

// execute is the internal LLM loop, shared by both sync and async paths.
func (a *Agent) execute(ctx context.Context, userMessage string, correlationID string) (string, error) {
	a.logger.Debug("executing", "message_length", len(userMessage))

	messages := []llm.Message{
		llm.SystemMessage(a.soul.SystemPrompt(ctx)),
		llm.SystemMessage(a.formatUserProfile()),
		llm.UserMessage(userMessage),
	}

	for {
		resp, err := a.llmClient.Chat(ctx, &llm.Request{
			Messages: messages,
			Tools:    a.tools,
			Metadata: map[string]string{"agent_id": a.id},
		})
		if err != nil {
			return "", fmt.Errorf("llm request failed: %w", err)
		}

		a.logger.Debug("llm response",
			"stop_reason", resp.StopReason,
			"tool_calls", len(resp.ToolCalls),
			"tokens", resp.Usage.TotalTokens)

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
	replyCh := make(chan core.Event, 1)

	a.outbox <- core.Event{
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

	select {
	case resultEvt := <-replyCh:
		result := resultEvt.Payload.(core.ToolCallResultPayload)
		if result.Error != "" {
			return nil, fmt.Errorf("tool call error: %s", result.Error)
		}
		return result.Result, nil
	case <-ctx.Done():
		return nil, ctx.Err()
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
