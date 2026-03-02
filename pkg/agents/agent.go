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

	"github.com/sriramsme/OnlyAgents/internal/config"
	"github.com/sriramsme/OnlyAgents/pkg/core"
	"github.com/sriramsme/OnlyAgents/pkg/llm"
	"github.com/sriramsme/OnlyAgents/pkg/logger"
	"github.com/sriramsme/OnlyAgents/pkg/memory"
	"github.com/sriramsme/OnlyAgents/pkg/tools"
	"github.com/sriramsme/OnlyAgents/pkg/workflow"
)

// NewAgent creates an agent. Kernel calls this and injects the shared bus + tool definitions.
func NewAgent(
	ctx context.Context, // ← Parent context (kernel's context)
	cfg config.AgentConfig,
	llmClient llm.Client,
	tools []tools.ToolDef,
	outbox chan<- core.Event,
	cm *memory.ConversationManager,
) (*Agent, error) {
	if llmClient == nil {
		return nil, fmt.Errorf("llm client is required")
	}

	agentSoul := NewSoul(cfg.Soul)

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
		tools:          tools,
		outbox:         outbox,
		cm:             cm,
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

func (a *Agent) Name() string { return a.name }

func (a *Agent) IsExecutive() bool { return a.isExecutive }

func (a *Agent) IsGeneral() bool { return a.isGeneral }

func (a *Agent) GetSkillNames() []string { return a.skills }

func (a *Agent) SetTools(tools []tools.ToolDef) { a.tools = tools }

func (a *Agent) SetFindBestAgent(fn tools.FindBestAgentFunc) {
	a.findBestAgent = fn
}

func (a *Agent) SetFindSkill(fn findSkillFunc) {
	a.findSkill = fn
}

func (a *Agent) SetUseSkillTool(fn useSkillToolFunc) {
	a.useSkillTool = fn
}

func (a *Agent) SetSystemPrompt(userSection string, availableAgents string) {
	parts := []string{
		a.soul.SystemPrompt(availableAgents),
	}
	parts = append(parts, userSection)
	a.systemPrompt = strings.Join(parts, "\n\n")
}

func (a *Agent) GetSystemPrompt() string {
	return a.systemPrompt
}

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

// handleEvent is the event handler that processes delegation/workflow results
func (a *Agent) handleEvent(evt core.Event) {
	switch evt.Type {
	case core.AgentExecute:
		a.handleAgentExecute(evt)

	case core.DelegationResult:
		// This shouldn't arrive here - it goes to ReplyTo channel
		// But log for debugging
		a.logger.Debug("delegation result received (via event bus)",
			"correlation_id", evt.CorrelationID)

	case core.WorkflowCompleted:
		// This shouldn't arrive here - it goes to ReplyTo channel
		// But log for debugging
		a.logger.Debug("workflow completed (via event bus)",
			"correlation_id", evt.CorrelationID)

	default:
		a.logger.Warn("unhandled event type",
			"type", evt.Type,
			"correlation_id", evt.CorrelationID)
	}
}

// handleAgentExecute processes AgentExecute events
func (a *Agent) handleAgentExecute(evt core.Event) {
	payload, ok := evt.Payload.(core.AgentExecutePayload)
	if !ok {
		a.logger.Error("invalid AgentExecute payload",
			"actual_type", fmt.Sprintf("%T", evt.Payload),
			"correlation_id", evt.CorrelationID)
		return
	}
	agentPhase := fmt.Sprintf("%s_execution", a.id)
	logger.Timing.StartPhase(evt.CorrelationID, agentPhase)

	requestCtx, cancel := context.WithTimeout(a.ctx, 5*time.Minute)
	defer cancel()

	a.logger.Debug("processing agent execute event",
		"correlation_id", evt.CorrelationID,
		"message_type", payload.MessageType,
		"message_length", len(payload.Message))

	// Regular agent uses standard execute
	response, err := a.execute(requestCtx, payload, evt.CorrelationID)
	logger.Timing.EndPhase(evt.CorrelationID, agentPhase)

	if err != nil {
		a.logger.Error("execute failed",
			"error", err,
			"correlation_id", evt.CorrelationID)

		if evt.ReplyTo != nil {
			a.sendError(evt.ReplyTo, evt.CorrelationID, err)
		}
		return
	}

	// Determine how to respond based on message type
	messageType := payload.MessageType

	switch messageType {

	case core.MessageTypeDelegation:
		// Check if this is a delegation with direct user response
		sendDirectlyToUser := false
		if payload.Delegation != nil {
			sendDirectlyToUser = payload.Delegation.SendDirectlyToUser
		}
		if sendDirectlyToUser {
			// Task was delegated to this agent - send result back
			a.sendOutboundMessage(payload, evt.CorrelationID, response)
		} else {
			// Task was delegated to this agent - send result back
			a.sendDelegationResult(evt.ReplyTo, evt.CorrelationID, response)
		}

	case core.MessageTypeWorkflowTask:
		// Task from workflow engine - send result back
		a.sendTaskResult(evt.ReplyTo, evt.CorrelationID, response)

	default:

		// Regular user message - send to channel
		if evt.ReplyTo != nil {
			// Sync response (HTTP)
			a.sendSyncResponse(evt.ReplyTo, evt.CorrelationID, response)
		} else {
			// Async response (channel)
			a.sendOutboundMessage(payload, evt.CorrelationID, response)
		}
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
// func (a *Agent) Execute(ctx context.Context, userMessage string) (string, error) {
// 	return a.execute(ctx, userMessage, uuid.NewString())
// }

// nolint:gocyclo
// execute is the internal LLM loop, shared by both sync and async paths.
func (a *Agent) execute(ctx context.Context, payload core.AgentExecutePayload, correlationID string) (string, error) {
	a.logger.Debug("executing with meta-tools",
		"message_length", len(payload.Message),
		"correlation_id", correlationID,
		"is_executive", a.isExecutive)

	//persist incoming user message
	if err := a.cm.SaveUserMessage(ctx, a.id, payload.Message); err != nil {
		// Non-fatal: log and continue. Losing one message is better than
		// crashing the entire request.
		a.logger.Warn("failed to save user message", "err", err, "correlation_id", correlationID)
	}

	// build messages from history instead of scratch
	history, err := a.cm.GetHistory(ctx, a.id, memory.DefaultHistoryLimit)
	if err != nil {
		a.logger.Warn("failed to load history, falling back to empty", "err", err)
		history = []llm.Message{}
	}
	messages := make([]llm.Message, 0, len(history)+1)
	messages = append(messages, llm.SystemMessage(a.systemPrompt))
	messages = append(messages, history...)

	llmCallCount := 0
	for {
		select {
		case <-ctx.Done():
			return "", fmt.Errorf("request cancelled: %w", ctx.Err())
		default:
		}

		llmCallCount++
		llmPhase := fmt.Sprintf("%s_llm_%d", a.id, llmCallCount)
		logger.Timing.StartPhase(correlationID, llmPhase)

		resp, err := a.llmClient.Chat(ctx, &llm.Request{
			Messages: messages,
			Tools:    a.tools,
			Metadata: map[string]string{
				"agent_id":       a.id,
				"correlation_id": correlationID,
			},
		})
		if err != nil {
			logger.Timing.EndPhaseWithMetadata(correlationID, llmPhase, map[string]any{"error": "failed"})
			return "", fmt.Errorf("llm request failed: %w", err)
		}
		logger.Timing.EndPhaseWithMetadata(correlationID, llmPhase, map[string]any{
			"model":            a.llmClient.Model(),
			"input_tokens":     resp.Usage.InputTokens,
			"output_tokens":    resp.Usage.OutputTokens,
			"cached_tokens":    resp.Usage.CachedTokens,
			"total_tokens":     resp.Usage.TotalTokens,
			"stop_reason":      resp.StopReason,
			"tool_calls_count": len(resp.ToolCalls),
		})
		a.logger.Debug("llm response",
			"msg", truncate(payload.Message, 100),
			"stop_reason", resp.StopReason,
			"tool_calls", len(resp.ToolCalls),
			"tokens", resp.Usage.TotalTokens,
			"correlation_id", correlationID)

		// No tool calls — final response.
		if !resp.HasToolCalls() {
			// persist final assistant response
			if err := a.cm.SaveAssistantMessage(ctx, a.id, resp.Content, resp.ReasoningContent, nil); err != nil {
				a.logger.Warn("failed to save assistant message", "err", err, "correlation_id", correlationID)
			}
			// If this was a workflow task, fire completion
			if payload.Workflow != nil {
				a.fireTaskCompletion(ctx, correlationID, payload.Workflow, resp.Content, nil)
			}
			return resp.Content, nil
		}

		// Has tool calls — persist assistant turn and add to local slice.
		//persist assistant turn with tool calls
		if err := a.cm.SaveAssistantMessage(ctx, a.id, resp.Content, resp.ReasoningContent, resp.ToolCalls); err != nil {
			a.logger.Warn("failed to save assistant tool-call message", "err", err, "correlation_id", correlationID)
		}

		messages = append(messages, llm.AssistantMessageWithTools(
			resp.Content, resp.ReasoningContent, resp.ToolCalls,
		))

		for _, tc := range resp.ToolCalls {
			var result any
			var toolErr error

			if a.isExecutive && isMetaTool(tc.Function.Name) {
				result, toolErr = a.handleMetaTool(ctx, correlationID, tc, payload.Message, payload.Channel)
			} else {
				result, toolErr = a.requestToolCall(ctx, correlationID, tc)
			}

			if toolErr != nil {
				a.logger.Warn("tool call failed",
					"tool", tc.Function.Name,
					"error", toolErr,
					"correlation_id", correlationID)
				errContent := fmt.Sprintf(`{"error": "%s"}`, toolErr.Error())

				if err := a.cm.SaveToolResult(ctx, a.id, tc.ID, tc.Function.Name, toolErr.Error(), true); err != nil {
					a.logger.Warn("failed to save tool error result", "err", err)
				}

				messages = append(messages, llm.ToolResultMessage(tc.ID, tc.Function.Name, errContent))
				continue
			}

			resultJSON, err := json.Marshal(result)
			if err != nil {
				return "", fmt.Errorf("marshal tool result: %w", err)
			}
			resultStr := string(resultJSON)

			if err := a.cm.SaveToolResult(ctx, a.id, tc.ID, tc.Function.Name, resultStr, false); err != nil {
				a.logger.Warn("failed to save tool result", "err", err)
			}

			messages = append(messages, llm.ToolResultMessage(tc.ID, tc.Function.Name, resultStr))
		}
		// Loop: LLM will see tool results and continue
	}
}

// requestToolCall fires a ToolCallRequest to kernel and blocks until result arrives.
// Uses a per-call reply channel so concurrent tool calls don't mix up results.
func (a *Agent) requestToolCall(ctx context.Context, correlationID string, tc tools.ToolCall) (any, error) {

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
			Arguments:  []byte(tc.Function.Arguments), // direct cast, no parsing
		},
	}

	a.logger.Debug("requesting tool call",
		"tool", tc.Function.Name,
		"args", tc.Function.Arguments,
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

// Add new method to fire task completion
func (a *Agent) fireTaskCompletion(ctx context.Context, correlationID string, wf *core.WorkflowMetadata, response string, err error) {
	resultMap := map[string]interface{}{
		"response": response,
		"agent_id": a.id,
		"task_id":  wf.TaskID,
	}

	resultJSON, marshalErr := json.Marshal(resultMap)

	errorMsg := ""
	if err != nil {
		errorMsg = err.Error()
	}
	if marshalErr != nil {
		errorMsg = fmt.Sprintf("marshal result: %s", marshalErr.Error())
	}

	completionEvent := core.Event{
		Type:          core.TaskCompleted,
		CorrelationID: correlationID,
		Payload: workflow.TaskCompletedPayload{
			WorkflowID: wf.WorkflowID,
			TaskID:     wf.TaskID,
			Result:     json.RawMessage(resultJSON),
			Error:      errorMsg,
		},
	}

	// Send to kernel bus
	select {
	case a.outbox <- completionEvent:
		a.logger.Debug("task completion sent",
			"task_id", wf.TaskID,
			"workflow_id", wf.WorkflowID)
	case <-time.After(5 * time.Second):
		a.logger.Warn("failed to send task completion - bus timeout")
	case <-ctx.Done():
		a.logger.Debug("context cancelled during task completion")
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
	ticker := time.NewTicker(3 * time.Minute)
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

// skillNameFromTool extracts skill name from tool name convention "skillname_action"
// e.g. "email_send" → "email", "calendar_create_event" → "calendar"
func skillNameFromTool(toolName string) string {
	parts := strings.SplitN(toolName, "_", 2)
	if len(parts) > 0 {
		return parts[0]
	}
	return toolName
}

func truncate(s string, max int) string {
	r := []rune(s)
	if len(r) <= max {
		return s
	}
	return string(r[:max]) + "..."
}
