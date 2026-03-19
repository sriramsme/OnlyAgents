package agents

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/sriramsme/OnlyAgents/pkg/core"
	"github.com/sriramsme/OnlyAgents/pkg/llm"
	"github.com/sriramsme/OnlyAgents/pkg/logger"
	"github.com/sriramsme/OnlyAgents/pkg/media"
	"github.com/sriramsme/OnlyAgents/pkg/tools"
	"github.com/sriramsme/OnlyAgents/pkg/workflow"
)

// processToolCalls executes all tool calls from a single LLM turn.
// Appends the assistant message and all tool results to messages.
// Returns updated messages, whether to halt, and any error.
// nolint:gocyclo
func (a *Agent) processToolCalls(
	ctx context.Context,
	sessionID string,
	correlationID string,
	payload core.AgentExecutePayload,
	messages []llm.Message,
	resp *llm.Response,
) (updated []llm.Message, produced []*media.Attachment, halt bool, err error) {
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

		if a.isExecutive && isExecutiveMetaTool(tc.Function.Name) {
			exec = a.handleExecutiveMetaTool(ctx, correlationID, tc, payload.Message, payload.Channel)
		} else if a.isGeneral && isGeneralMetaTool(tc.Function.Name) {
			exec = a.handleGeneralMetaTool(ctx, correlationID, tc)
		} else {
			exec = a.requestToolCall(ctx, sessionID, correlationID, tc)
		}

		// Collect any files the skill wrote to disk this round.
		if len(exec.ProducedFiles) > 0 {
			for _, path := range exec.ProducedFiles {
				att, err := media.AttachmentFromPath(path)
				if err != nil {
					a.logger.Warn("could not resolve produced file",
						"path", path,
						"tool", tc.Function.Name,
						"err", err)
					continue
				}
				produced = append(produced, att)
				a.logger.Debug("skill produced file",
					"tool", tc.Function.Name,
					"path", path,
					"mime_type", att.MIMEType)
			}
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

		// ── Always append a tool result message — OpenAI requires every
		// tool_call_id to have a response before the next assistant turn. ─────────
		switch {
		case exec.Err != nil:
			a.logger.Warn("tool call failed",
				"tool", tc.Function.Name,
				"error", exec.Err,
				"correlation_id", correlationID)
			errContent := fmt.Sprintf(`{"error": "%s"}`, exec.Err.Error())
			if err := a.cm.SaveToolResult(ctx, sessionID, a.id, tc.ID, tc.Function.Name, exec.Err.Error(), true); err != nil {
				a.logger.Warn("failed to save tool error result", "err", err)
			}
			messages = append(messages, llm.ToolResultMessage(tc.ID, tc.Function.Name, errContent))

		case exec.Control == tools.ExecHalt:
			// Must still add a tool result before halting — satisfies protocol
			haltMsg := exec.DirectMessage
			if haltMsg == "" {
				haltMsg = fmt.Sprintf(`{"status": "halted", "tool": "%s"}`, tc.Function.Name)
			}
			if err := a.cm.SaveToolResult(ctx, sessionID, a.id, tc.ID, tc.Function.Name, haltMsg, false); err != nil {
				a.logger.Warn("failed to save tool result (halt)", "err", err)
			}
			messages = append(messages, llm.ToolResultMessage(tc.ID, tc.Function.Name, haltMsg))

			a.logger.Info("halting execution loop",
				"tool", tc.Function.Name,
				"correlation_id", correlationID)

			if exec.DirectMessage != "" {
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
			return messages, nil, true, nil

		default:
			resultJSON, err := json.Marshal(exec.Result)
			if err != nil {
				return nil, nil, false, fmt.Errorf("marshal tool result: %w", err)
			}
			resultStr := string(resultJSON)
			if err := a.cm.SaveToolResult(ctx, sessionID, a.id, tc.ID, tc.Function.Name, resultStr, false); err != nil {
				a.logger.Warn("failed to save tool result", "err", err)
			}
			messages = append(messages, llm.ToolResultMessage(tc.ID, tc.Function.Name, resultStr))
		}
	}

	return messages, produced, false, nil
}

func (a *Agent) requestToolCall(
	ctx context.Context,
	sessionID string,
	correlationID string,
	tc tools.ToolCall,
) tools.ToolExecution {
	// Meta tools are handled internally — never routed to a skill
	if isSubAgentMetaTool(tc.Function.Name) {
		return a.handleMetaTool(ctx, sessionID, tc)
	}

	skill, ok := a.skills[a.toolSkillMap[tc.Function.Name]]
	if !ok {
		return tools.ExecErr(fmt.Errorf("no skill registered for tool %q", tc.Function.Name))
	}

	toolPhase := fmt.Sprintf("%s_tool_%s", a.id, tc.Function.Name)
	logger.Timing.StartPhase(correlationID, toolPhase)

	execCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	result, err := skill.Execute(execCtx, tc.Function.Name, []byte(tc.Function.Arguments))

	logger.Timing.EndPhaseWithMetadata(correlationID, toolPhase, map[string]any{
		"tool":  tc.Function.Name,
		"skill": skill.Name(),
		"error": err != nil,
	})

	if err != nil {
		a.logger.Error("tool execution failed",
			"tool", tc.Function.Name,
			"skill", skill.Name(),
			"error", err,
			"correlation_id", correlationID)
		return tools.ExecErr(err)
	}

	a.logger.Debug("tool execution succeeded",
		"tool", tc.Function.Name,
		"correlation_id", correlationID)
	return tools.ExecOK(result)
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
