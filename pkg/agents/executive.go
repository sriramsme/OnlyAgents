package agents

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/sriramsme/OnlyAgents/pkg/core"
	"github.com/sriramsme/OnlyAgents/pkg/logger"
	"github.com/sriramsme/OnlyAgents/pkg/tools"
)

// These methods are used by executive agents to delegate and create workflows
// They're called from the agent's execute() loop when LLM calls meta-tools

// requestDelegation delegates a task to another agent and waits for result
func (a *Agent) requestDelegation(ctx context.Context, correlationID string,
	tc tools.ToolCall, channelMetadata *core.ChannelMetadata) (any, error) {
	var input tools.DelegateInput
	if err := json.Unmarshal([]byte(tc.Function.Arguments), &input); err != nil {
		return nil, fmt.Errorf("invalid delegation args: %w", err)
	}

	a.logger.Info("delegating task",
		"agent_id", input.AgentID,
		"task", input.Task,
		"send_directly_to_user", input.SendDirectlyToUser,
		"capabilities", input.Capabilities,
		"correlation_id", correlationID)

	// Create reply channel for result
	replyCh := make(chan core.Event, 1)

	delegationID := uuid.NewString()

	delegationPhase := fmt.Sprintf("delegation_%s", input.AgentID)
	logger.Timing.StartPhase(correlationID, delegationPhase)

	event := core.Event{
		Type:          core.AgentDelegate,
		CorrelationID: correlationID,
		AgentID:       a.id,
		ReplyTo:       replyCh,
		Payload: core.AgentDelegatePayload{
			DelegationID:       delegationID,
			AgentID:            input.AgentID, // ← Executive specifies target agent
			Task:               input.Task,
			Capabilities:       input.Capabilities,
			Context:            input.Context,
			SendDirectlyToUser: input.SendDirectlyToUser,
			Timeout:            300,             // 5 minutes default
			Channel:            channelMetadata, // In case is sending directly to user, sub-agent needs chatID, channelName etc
		},
	}

	// Send delegation request
	select {
	case a.outbox <- event:
		a.logger.Debug("delegation request sent", "delegation_id", delegationID)
	case <-ctx.Done():
		logger.Timing.EndPhase(correlationID, delegationPhase)
		return nil, fmt.Errorf("request cancelled")
	case <-a.ctx.Done():
		logger.Timing.EndPhase(correlationID, delegationPhase)
		return nil, fmt.Errorf("agent shutting down")
	case <-time.After(5 * time.Second):
		logger.Timing.EndPhase(correlationID, delegationPhase)
		return nil, fmt.Errorf("failed to send delegation request")
	}

	// If sending directly to user, return immediately
	// Executive doesn't wait for response
	if input.SendDirectlyToUser {
		logger.Timing.EndPhaseWithMetadata(correlationID, delegationPhase, map[string]any{
			"direct_response": true,
		})
		return map[string]any{
			"status":                "delegated",
			"message":               fmt.Sprintf("Task delegated to %s. Response will be sent directly to user.", input.AgentID),
			"delegation_id":         delegationID,
			"send_directly_to_user": true,
		}, nil
	}

	// Wait for result
	select {
	case resultEvt := <-replyCh:
		result, ok := resultEvt.Payload.(core.DelegationResultPayload)
		if !ok {
			logger.Timing.EndPhase(correlationID, delegationPhase)
			return nil, fmt.Errorf("invalid delegation result payload")
		}

		if result.Error != "" {
			logger.Timing.EndPhaseWithMetadata(correlationID, delegationPhase, map[string]any{
				"error": "failed",
			})
			return nil, fmt.Errorf("delegation error: %s", result.Error)
		}

		logger.Timing.EndPhaseWithMetadata(correlationID, delegationPhase, map[string]any{
			"agent": input.AgentID,
		})
		a.logger.Info("delegation completed",
			"delegation_id", delegationID,
			"correlation_id", correlationID)

		return result.Result, nil

	case <-ctx.Done():
		logger.Timing.EndPhase(correlationID, delegationPhase)
		return nil, fmt.Errorf("request cancelled")
	case <-a.ctx.Done():
		logger.Timing.EndPhase(correlationID, delegationPhase)
		return nil, fmt.Errorf("agent shutting down")
	case <-time.After(5 * time.Minute):
		logger.Timing.EndPhase(correlationID, delegationPhase)
		return nil, fmt.Errorf("delegation timeout")
	}
}

// requestWorkflow creates a workflow and waits for completion
func (a *Agent) requestWorkflow(ctx context.Context, correlationID string, tc tools.ToolCall) (any, error) {
	logger.Timing.StartPhase(correlationID, "workflow_creation")
	wf, err := a.parseWorkflow(correlationID, tc)
	if err != nil {
		return nil, err
	}
	logger.Timing.EndPhase(correlationID, "workflow_creation")
	return a.submitAndWait(ctx, correlationID, wf)
}

func (a *Agent) parseWorkflow(correlationID string, tc tools.ToolCall) (core.Workflow, error) {
	var input tools.CreateWorkflowInput
	if err := json.Unmarshal([]byte(tc.Function.Arguments), &input); err != nil {
		return core.Workflow{}, fmt.Errorf("invalid workflow args: %w", err)
	}

	a.logger.Info("creating workflow", "name", input.Name, "tasks", len(input.Tasks), "correlation_id", correlationID)

	tasks := make([]*core.Task, 0, len(input.Tasks))
	for _, t := range input.Tasks {
		taskID := t.ID
		if taskID == "" {
			taskID = uuid.NewString()
		}
		tasks = append(tasks, &core.Task{
			ID:                   taskID,
			Name:                 t.Name,
			Description:          t.Description,
			Type:                 core.TaskTypeAgentExecution,
			DependsOn:            t.DependsOn,
			RequiredCapabilities: t.RequiredCapabilities,
			CreatedAt:            time.Now(),
			Status:               core.TaskStatusPending,
			MaxRetries:           3,
		})
	}

	return core.Workflow{
		ID:          uuid.NewString(),
		Name:        input.Name,
		Description: fmt.Sprintf("Workflow created by executive agent for: %s", input.Name),
		Tasks:       tasks,
		CreatedAt:   time.Now(),
		CreatedBy:   a.id,
		Status:      core.WorkflowStatusPending,
	}, nil
}

func (a *Agent) submitAndWait(ctx context.Context, correlationID string, wf core.Workflow) (any, error) {
	replyCh := make(chan core.Event, 1)
	event := core.Event{
		Type:          core.WorkflowSubmitted,
		CorrelationID: correlationID,
		AgentID:       a.id,
		ReplyTo:       replyCh,
		Payload:       core.WorkflowPayload{Workflow: wf},
	}

	select {
	case a.outbox <- event:
		a.logger.Debug("workflow submitted", "workflow_id", wf.ID)
	case <-ctx.Done():
		return nil, fmt.Errorf("request cancelled")
	case <-a.ctx.Done():
		return nil, fmt.Errorf("agent shutting down")
	case <-time.After(5 * time.Second):
		return nil, fmt.Errorf("failed to submit workflow")
	}

	select {
	case resultEvt := <-replyCh:
		return a.handleWorkflowResult(correlationID, resultEvt)
	case <-ctx.Done():
		return nil, fmt.Errorf("request cancelled")
	case <-a.ctx.Done():
		return nil, fmt.Errorf("agent shutting down")
	case <-time.After(10 * time.Minute):
		return nil, fmt.Errorf("workflow timeout")
	}
}

func (a *Agent) handleWorkflowResult(correlationID string, evt core.Event) (any, error) {
	result, ok := evt.Payload.(core.WorkflowResultPayload)
	if !ok {
		return nil, fmt.Errorf("invalid workflow result payload")
	}
	if result.Error != "" {
		return nil, fmt.Errorf("workflow error: %s", result.Error)
	}
	a.logger.Info("workflow completed",
		"workflow_id", result.WorkflowID,
		"status", result.Status,
		"correlation_id", correlationID)
	return result.Results, nil
}

// requestCapabilityQuery queries available capabilities from kernel
func (a *Agent) requestAgentSelection(ctx context.Context, correlationID string, tc tools.ToolCall) (any, error) {
	var input tools.FindBestAgentInput
	if err := json.Unmarshal([]byte(tc.Function.Arguments), &input); err != nil {
		return nil, fmt.Errorf("invalid find_best_agent args: %w", err)
	}
	if a.findBestAgent == nil {
		return nil, fmt.Errorf("findBestAgent not configured")
	}
	return a.findBestAgent(ctx, input.Task, input.Capabilities)
}

// handleMetaTool routes meta-tool calls to appropriate handlers
func (a *Agent) handleMetaTool(ctx context.Context, correlationID string, tc tools.ToolCall, channelMetadata *core.ChannelMetadata) (any, error) {
	a.logger.Debug("handling meta-tool",
		"tool", tc.Function.Name,
		"correlation_id", correlationID)

	switch tc.Function.Name {
	case "delegate_to_agent":
		return a.requestDelegation(ctx, correlationID, tc, channelMetadata)

	case "create_workflow":
		return a.requestWorkflow(ctx, correlationID, tc)

	case "find_best_agent":
		return a.requestAgentSelection(ctx, correlationID, tc)

	default:
		return nil, fmt.Errorf("unknown meta-tool: %s", tc.Function.Name)
	}
}

// isMetaTool checks if a tool name is a meta-tool
func isMetaTool(toolName string) bool {
	metaTools := map[string]bool{
		"delegate_to_agent":  true,
		"create_workflow":    true,
		"query_capabilities": true,
	}
	return metaTools[toolName]
}

// Helper methods for sending different types of responses

func (a *Agent) sendDelegationResult(replyCh chan<- core.Event, correlationID string, result any) {
	if replyCh == nil {
		return
	}

	evt := core.Event{
		Type:          core.DelegationResult,
		CorrelationID: correlationID,
		Payload: core.DelegationResultPayload{
			Result: result,
		},
	}

	a.safeReply(replyCh, evt, "delegation result")
}

func (a *Agent) sendTaskResult(replyCh chan<- core.Event, correlationID string, result any) {
	if replyCh == nil {
		return
	}

	// Task result goes back to workflow engine
	// For now, use same structure as delegation result
	evt := core.Event{
		Type:          core.DelegationResult, // Workflow engine expects this
		CorrelationID: correlationID,
		Payload: core.DelegationResultPayload{
			Result: result,
		},
	}

	a.safeReply(replyCh, evt, "task result")
}

func (a *Agent) sendSyncResponse(replyCh chan<- core.Event, correlationID string, response string) {
	if replyCh == nil {
		return
	}

	evt := core.Event{
		Type:          core.AgentExecute,
		CorrelationID: correlationID,
		Payload:       response,
	}

	a.safeReply(replyCh, evt, "sync response")
}

func (a *Agent) sendOutboundMessage(payload core.AgentExecutePayload, correlationID string, response string) {
	evt := core.Event{
		Type:          core.OutboundMessage,
		CorrelationID: correlationID,
		Payload: core.OutboundMessagePayload{
			ChannelName: payload.Channel.Name,
			ChatID:      payload.Channel.ChatID,
			Content:     response,
		},
	}

	a.safeSend(evt, "outbound message")
}

func (a *Agent) sendError(replyCh chan<- core.Event, correlationID string, err error) {
	if replyCh == nil {
		return
	}

	evt := core.Event{
		Type:          core.DelegationResult,
		CorrelationID: correlationID,
		Payload: core.DelegationResultPayload{
			Error: err.Error(),
		},
	}

	a.safeReply(replyCh, evt, "error response")
}
