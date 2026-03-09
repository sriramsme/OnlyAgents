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
	"math/rand"
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
	outbox chan<- core.Event,
	uiBus core.UIBus,
	cm *memory.ConversationManager,
	mm *memory.MemoryManager,
) (*Agent, error) {
	if llmClient == nil {
		return nil, fmt.Errorf("llm client is required")
	}

	agentSoul := NewSoul(cfg.Soul)

	// Create agent context from parent - ties agent lifecycle to kernel
	agentCtx, cancel := context.WithCancel(ctx)

	return &Agent{
		id:               cfg.ID,
		name:             cfg.Name,
		isExecutive:      cfg.IsExecutive,
		isGeneral:        cfg.IsGeneral,
		maxConcurrency:   cfg.MaxConcurrency,
		skills:           cfg.Skills,
		streamingEnabled: cfg.StreamingEnabled,
		llmClient:        llmClient,
		soul:             agentSoul,
		outbox:           outbox,
		uiBus:            uiBus,
		tools:            []tools.ToolDef{},
		toolSkillMap:     make(map[string]tools.SkillName),
		cm:               cm,
		mm:               mm,
		inbox:            make(chan core.Event, cfg.BufferSize),
		ctx:              agentCtx,
		cancel:           cancel,
		logger:           slog.With("agent_id", cfg.ID),
		state:            "idle",
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

func (a *Agent) Status() core.AgentStatus {
	a.stateMu.RLock()
	defer a.stateMu.RUnlock()
	return core.AgentStatus{
		ID:          a.id,
		Name:        a.name,
		State:       a.state,
		CurrentTask: a.currentTask,
		LastActive:  a.lastActive,
		Model:       a.llmClient.Model(),
		IsExecutive: a.isExecutive,
	}
}

func (a *Agent) setState(state core.AgentState, task string) {
	a.stateMu.Lock()
	a.state = state
	a.currentTask = task
	a.lastActive = time.Now()
	a.stateMu.Unlock()
}

// non-blocking, nil-safe UIBus write

func (a *Agent) emitUI(evt core.UIEvent) {
	if a.uiBus == nil {
		return
	}
	select {
	case a.uiBus <- evt:
	default: // drop — UI missing one frame is fine, blocking the agent is not
	}
}

func (a *Agent) updateUI(message string, maxLen int) {
	if len(message) > maxLen {
		message = message[:maxLen] + "…"
	}
	a.setState("active", message)
	a.activeSince = time.Now()

	a.emitUI(core.UIEvent{
		Type:      core.UIEventAgentActivated,
		Timestamp: time.Now(),
		AgentID:   a.id,
		Payload: core.AgentActivatedPayload{
			Task:  message,
			Model: a.llmClient.Model(),
		},
	})
	defer func() {
		a.setState("idle", "")
		a.emitUI(core.UIEvent{
			Type:      core.UIEventAgentIdle,
			Timestamp: time.Now(),
			AgentID:   a.id,
			Payload: core.AgentIdlePayload{
				DurationMs: time.Since(a.activeSince).Milliseconds(),
			},
		})
	}()
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

func (a *Agent) GetSkillNames() []tools.SkillName { return a.skills }

func (a *Agent) SetTools(tools []tools.ToolDef) {
	a.tools = tools
	for _, tool := range tools {
		a.toolSkillMap[tool.Name] = tool.Skill
	}
}

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
	sem := make(chan struct{}, a.maxConcurrency)

	for {
		select {
		case evt := <-a.inbox:
			sem <- struct{}{} // acquire slot (blocks if at max)
			go func(e core.Event) {
				defer func() { <-sem }() // release slot
				a.handleEvent(e)
			}(evt)
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

	a.updateUI(payload.Message, 60)

	agentPhase := fmt.Sprintf("%s_execution", a.id)
	logger.Timing.StartPhase(evt.CorrelationID, agentPhase)

	requestCtx, cancel := context.WithTimeout(a.ctx, 5*time.Minute)
	defer cancel()

	a.logger.Debug("processing agent execute event",
		"correlation_id", evt.CorrelationID,
		"message_type", payload.MessageType,
		"message_length", len(payload.Message))

	// Choose execute path based on whether this response goes to user directly
	var response string
	var err error
	if a.shouldStream(payload) {
		response, err = a.executeStream(requestCtx, payload, evt.CorrelationID)
	} else {
		response, err = a.execute(requestCtx, payload, evt.CorrelationID)
	}

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
// execute is the non-streaming internal LLM loop, shared by both sync and async paths.
func (a *Agent) execute(
	ctx context.Context,
	payload core.AgentExecutePayload,
	correlationID string,
) (string, error) {
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
			return "", err
		}
		if halt {
			return "", nil
		}
	}
}

// ── ExecuteStream (streaming) ─────────────────────────────────────────────────

func (a *Agent) executeStream(
	ctx context.Context,
	payload core.AgentExecutePayload,
	correlationID string,
) (string, error) {
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

		var (
			fullContent  strings.Builder
			toolCalls    []tools.ToolCall
			inputTokens  int
			outputTokens int
			streamErr    error
		)

		for chunk := range streamCh {
			if chunk.Error != nil {
				streamErr = chunk.Error
				break
			}
			if chunk.Done {
				inputTokens = chunk.Usage.InputTokens
				outputTokens = chunk.Usage.OutputTokens
				break
			}
			if chunk.Content != "" {
				fullContent.WriteString(chunk.Content)
				// Emit token to UI — only on the final turn (no tool calls expected yet).
				// Tool-call turns emit nothing; intermediate reasoning stays internal.
				a.safeSend(core.Event{
					Type:          core.OutboundToken,
					CorrelationID: correlationID,
					AgentID:       a.id,
					Payload: core.OutboundTokenPayload{
						Channel:            payload.Channel,
						Token:              chunk.Content,
						AccumulatedContent: fullContent.String(),
					},
				}, "agent token")
			}
			if len(chunk.ToolCalls) > 0 {
				toolCalls = append(toolCalls, chunk.ToolCalls...)
			}
		}

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
			Content:    fullContent.String(),
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
			return resp.Content, nil
		}
		var processErr error
		messages, halt, processErr = a.processToolCalls(ctx, sessionID, correlationID, payload, messages, resp)
		if processErr != nil {
			return "", err
		}
		if halt {
			return "", nil
		}
	}
}

// ── Shared helpers ────────────────────────────────────────────────────────────

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

// processToolCalls executes all tool calls from a single LLM turn.
// Appends the assistant message and all tool results to messages.
// Returns updated messages, whether to halt, and any error.
func (a *Agent) processToolCalls(
	ctx context.Context,
	sessionID string,
	correlationID string,
	payload core.AgentExecutePayload,
	messages []llm.Message,
	resp *llm.Response,
) (updated []llm.Message, halt bool, err error) {
	// Persist assistant turn with tool calls
	if err := a.cm.SaveAssistantMessage(ctx, sessionID, a.id, resp.Content, resp.ReasoningContent, resp.ToolCalls); err != nil {
		a.logger.Warn("failed to save assistant tool-call message", "err", err, "correlation_id", correlationID)
	}

	messages = append(messages, llm.AssistantMessageWithTools(
		resp.Content, resp.ReasoningContent, resp.ToolCalls,
	))

	for _, tc := range resp.ToolCalls {
		toolStart := time.Now()
		a.emitUI(core.UIEvent{
			Type:      core.UIEventToolCalled,
			Timestamp: time.Now(),
			AgentID:   a.id,
			Payload: core.ToolCalledPayload{
				ToolName: tc.Function.Name,
				Input:    tc.Function.Arguments,
			},
		})

		var exec tools.ToolExecution
		if a.isExecutive && isMetaTool(tc.Function.Name) {
			exec = a.handleMetaTool(ctx, correlationID, tc, payload.Message, payload.Channel)
		} else {
			exec = a.requestToolCall(ctx, correlationID, tc)
		}

		a.emitUI(core.UIEvent{
			Type:      core.UIEventToolResult,
			Timestamp: time.Now(),
			AgentID:   a.id,
			Payload: core.ToolResultPayload{
				ToolName:   tc.Function.Name,
				Success:    exec.Err == nil,
				DurationMs: time.Since(toolStart).Milliseconds(),
			},
		})

		if exec.Err != nil {
			a.logger.Warn("tool call failed",
				"tool", tc.Function.Name,
				"error", exec.Err,
				"correlation_id", correlationID)
			errContent := fmt.Sprintf(`{"error": "%s"}`, exec.Err.Error())
			if err := a.cm.SaveToolResult(ctx, sessionID, a.id, tc.ID, tc.Function.Name, exec.Err.Error(), true); err != nil {
				a.logger.Warn("failed to save tool error result", "err", err)
			}
			messages = append(messages, llm.ToolResultMessage(tc.ID, tc.Function.Name, errContent))
		} else {
			resultJSON, err := json.Marshal(exec.Result)
			if err != nil {
				return nil, false, fmt.Errorf("marshal tool result: %w", err)
			}
			resultStr := string(resultJSON)
			if err := a.cm.SaveToolResult(ctx, sessionID, a.id, tc.ID, tc.Function.Name, resultStr, false); err != nil {
				a.logger.Warn("failed to save tool result", "err", err)
			}
			messages = append(messages, llm.ToolResultMessage(tc.ID, tc.Function.Name, resultStr))
		}

		if exec.Control == tools.ExecHalt {
			a.logger.Info("halting execution loop",
				"tool", tc.Function.Name,
				"correlation_id", correlationID)
			if exec.DirectMessage != "" {
				if err := a.cm.SaveToolResult(ctx, sessionID, a.id, tc.ID, tc.Function.Name, exec.DirectMessage, false); err != nil {
					a.logger.Warn("failed to save tool result (direct message)", "err", err)
				}
				a.safeSend(core.Event{
					Type:          core.OutboundMessage,
					CorrelationID: correlationID,
					AgentID:       a.id,
					Payload: core.OutboundMessagePayload{
						Channel: payload.Channel,
						Content: exec.DirectMessage,
					},
				}, "delegation ack")
			}
			return messages, true, nil
		}
	}

	return messages, false, nil
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

// requestToolCall fires a ToolCallRequest to kernel and blocks until result arrives.
// Uses a per-call reply channel so concurrent tool calls don't mix up results.
func (a *Agent) requestToolCall(ctx context.Context, correlationID string, tc tools.ToolCall) tools.ToolExecution {
	skillName, ok := a.toolSkillMap[tc.Function.Name]
	if !ok {
		return tools.ExecErr(fmt.Errorf("agent: no skill registered for tool %q", tc.Function.Name))
	}
	// Kernel will send the result back on this channel
	// Buffer of 1 ensures non-blocking send from kernel
	replyCh := make(chan core.Event, 1)

	event := core.Event{
		Type:          core.ToolCallRequest,
		CorrelationID: correlationID,
		AgentID:       a.id,
		ReplyTo:       replyCh,
		Payload: tools.ToolCallRequestPayload{
			ToolCallID: tc.ID,
			SkillName:  skillName,
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
		return tools.ExecErr(fmt.Errorf("request cancelled: %w", ctx.Err()))
	case <-a.ctx.Done():
		return tools.ExecErr(fmt.Errorf("agent shutting down"))
	case <-time.After(5 * time.Second):
		return tools.ExecErr(fmt.Errorf("failed to send tool call request (timeout): %s", tc.Function.Name))
	}

	// Wait for result with timeout
	select {
	case resultEvt := <-replyCh:
		result, ok := resultEvt.Payload.(tools.ToolCallResultPayload)
		if !ok {
			return tools.ExecErr(fmt.Errorf("invalid tool result payload type: %T", resultEvt.Payload))
		}

		if result.Error != "" {
			return tools.ExecErr(fmt.Errorf("tool call error: %s", result.Error))
		}

		a.logger.Debug("tool call succeeded",
			"tool", tc.Function.Name,
			"correlation_id", correlationID)

		return tools.ExecOK(result.Result)

	case <-ctx.Done():
		return tools.ExecErr(fmt.Errorf("request cancelled: %w", ctx.Err()))

	case <-a.ctx.Done():
		return tools.ExecErr(fmt.Errorf("agent shutting down"))

	case <-time.After(30 * time.Second):
		return tools.ExecErr(fmt.Errorf("tool call timeout: %s", tc.Function.Name))
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
		Payload: workflow.WFTaskCompletedPayload{
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

func (a *Agent) delegationAck(agentName string) string {
	msgs := a.soul.DelegationAcknowledgments()
	if len(msgs) == 0 {
		// fallback if not configured
		return fmt.Sprintf("I've handed that off to %s.", agentName)
	}
	template := msgs[rand.Intn(len(msgs))] // nolint:gosec
	return strings.ReplaceAll(template, "{agent_name}", agentName)
}

func truncate(s string, max int) string {
	r := []rune(s)
	if len(r) <= max {
		return s
	}
	return string(r[:max]) + "..."
}
