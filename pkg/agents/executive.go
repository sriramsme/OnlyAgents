package agents

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/sriramsme/OnlyAgents/pkg/core"
	"github.com/sriramsme/OnlyAgents/pkg/tools"
)

// ====================
// Executive Agent Extensions
// ====================

// These methods are used by executive agents to delegate and create workflows
// They're called from the agent's execute() loop when LLM calls meta-tools

// requestDelegation delegates a task to another agent and waits for result
func (a *Agent) requestDelegation(ctx context.Context, correlationID string, tc tools.ToolCall) (any, error) {
	args, err := tools.ParseArguments(tc.Function.Arguments)
	if err != nil {
		return nil, fmt.Errorf("invalid delegation args: %w", err)
	}

	agentID, _ := args["agent_id"].(string)
	taskDesc, _ := args["task"].(string)
	capsRaw, _ := args["capabilities"].([]interface{})
	contextData, _ := args["context"].(map[string]interface{})

	// Convert capabilities to proper type
	var capabilities []core.Capability
	for _, cap := range capsRaw {
		if capStr, ok := cap.(string); ok {
			capabilities = append(capabilities, core.Capability(capStr))
		}
	}

	a.logger.Info("delegating task",
		"agent_id", agentID,
		"task", taskDesc,
		"capabilities", capabilities,
		"correlation_id", correlationID)

	// Create reply channel for result
	replyCh := make(chan core.Event, 1)

	delegationID := uuid.NewString()
	event := core.Event{
		Type:          core.AgentDelegate,
		CorrelationID: correlationID,
		AgentID:       a.id,
		ReplyTo:       replyCh,
		Payload: core.AgentDelegatePayload{
			DelegationID: delegationID,
			AgentID:      agentID, // ← Executive specifies target agent
			Task:         taskDesc,
			Capabilities: capabilities,
			Context:      contextData,
			Timeout:      300, // 5 minutes default
		},
	}

	// Send delegation request
	select {
	case a.outbox <- event:
		a.logger.Debug("delegation request sent", "delegation_id", delegationID)
	case <-ctx.Done():
		return nil, fmt.Errorf("request cancelled")
	case <-a.ctx.Done():
		return nil, fmt.Errorf("agent shutting down")
	case <-time.After(5 * time.Second):
		return nil, fmt.Errorf("failed to send delegation request")
	}

	// Wait for result
	select {
	case resultEvt := <-replyCh:
		result, ok := resultEvt.Payload.(core.DelegationResultPayload)
		if !ok {
			return nil, fmt.Errorf("invalid delegation result payload")
		}

		if result.Error != "" {
			return nil, fmt.Errorf("delegation error: %s", result.Error)
		}

		a.logger.Info("delegation completed",
			"delegation_id", delegationID,
			"correlation_id", correlationID)

		return result.Result, nil

	case <-ctx.Done():
		return nil, fmt.Errorf("request cancelled")
	case <-a.ctx.Done():
		return nil, fmt.Errorf("agent shutting down")
	case <-time.After(5 * time.Minute):
		return nil, fmt.Errorf("delegation timeout")
	}
}

// requestWorkflow creates a workflow and waits for completion
func (a *Agent) requestWorkflow(ctx context.Context, correlationID string, tc tools.ToolCall) (any, error) {
	wf, err := a.parseWorkflow(correlationID, tc)
	if err != nil {
		return nil, err
	}
	return a.submitAndWait(ctx, correlationID, wf)
}

func (a *Agent) parseWorkflow(correlationID string, tc tools.ToolCall) (core.Workflow, error) {
	args, err := tools.ParseArguments(tc.Function.Arguments)
	if err != nil {
		return core.Workflow{}, fmt.Errorf("invalid workflow args: %w", err)
	}
	workflowName, _ := args["name"].(string)
	tasksRaw, _ := args["tasks"].([]interface{})

	a.logger.Info("creating workflow", "name", workflowName, "tasks", len(tasksRaw), "correlation_id", correlationID)

	tasks := make([]*core.Task, 0, len(tasksRaw))
	for _, raw := range tasksRaw {
		task, ok := parseTask(raw)
		if ok {
			tasks = append(tasks, task)
		}
	}

	return core.Workflow{
		ID:          uuid.NewString(),
		Name:        workflowName,
		Description: fmt.Sprintf("Workflow created by executive agent for: %s", workflowName),
		Tasks:       tasks,
		CreatedAt:   time.Now(),
		CreatedBy:   a.id,
		Status:      core.WorkflowStatusPending,
	}, nil
}

func parseTask(raw interface{}) (*core.Task, bool) {
	taskMap, ok := raw.(map[string]interface{})
	if !ok {
		return nil, false
	}
	taskID, _ := taskMap["id"].(string)
	if taskID == "" {
		taskID = uuid.NewString()
	}

	capsRaw, _ := taskMap["capabilities"].([]interface{})
	depsRaw, _ := taskMap["depends_on"].([]interface{})

	return &core.Task{
		ID:                   taskID,
		Name:                 taskMap["name"].(string),
		Description:          taskMap["description"].(string),
		Type:                 core.TaskTypeAgentExecution,
		DependsOn:            toStringSlice(depsRaw),
		RequiredCapabilities: toStringSlice(capsRaw),
		CreatedAt:            time.Now(),
		Status:               core.TaskStatusPending,
		MaxRetries:           3,
	}, true
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

func toStringSlice(raw []interface{}) []string {
	result := make([]string, 0, len(raw))
	for _, v := range raw {
		if s, ok := v.(string); ok {
			result = append(result, s)
		}
	}
	return result
} // requestCapabilityQuery queries available capabilities from kernel
func (a *Agent) requestCapabilityQuery(ctx context.Context, correlationID string, tc tools.ToolCall) (any, error) {
	// For now, return a simple result
	// In full implementation, kernel would respond with available capabilities

	// TODO: Implement proper capability query via kernel
	return map[string]any{
		"capabilities": []string{
			"email", "calendar", "web_search", "tasks",
			"git", "docker", "kubernetes",
		},
		"message": "Capability query not yet implemented - returning mock data",
	}, nil
}

// handleMetaTool routes meta-tool calls to appropriate handlers
func (a *Agent) handleMetaTool(ctx context.Context, correlationID string, tc tools.ToolCall) (any, error) {
	a.logger.Debug("handling meta-tool",
		"tool", tc.Function.Name,
		"correlation_id", correlationID)

	switch tc.Function.Name {
	case "delegate_to_agent":
		return a.requestDelegation(ctx, correlationID, tc)

	case "create_workflow":
		return a.requestWorkflow(ctx, correlationID, tc)

	case "query_capabilities":
		return a.requestCapabilityQuery(ctx, correlationID, tc)

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
			ChannelName: payload.Metadata["channel"],
			ChatID:      payload.ChatID,
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
