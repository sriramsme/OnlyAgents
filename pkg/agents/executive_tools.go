package agents

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/sriramsme/OnlyAgents/pkg/core"
	"github.com/sriramsme/OnlyAgents/pkg/logger"
	"github.com/sriramsme/OnlyAgents/pkg/media"
	"github.com/sriramsme/OnlyAgents/pkg/tools"
	"github.com/sriramsme/OnlyAgents/pkg/workflow"
)

// These methods are used by executive agents to delegate and create workflows
// They're called from the agent's execute() loop when LLM calls meta-tools

// handleMetaTool routes meta-tool calls to appropriate handlers
func (a *Agent) handleExecutiveMetaTool(
	ctx context.Context,
	correlationID string,
	tc tools.ToolCall,
	originalMessage string,
	channelMetadata *core.ChannelMetadata,
	attachments []*media.Attachment,
) tools.ToolExecution {
	a.logger.Debug("handling meta-tool",
		"tool", tc.Function.Name,
		"correlation_id", correlationID)

	switch tc.Function.Name {
	case "delegate_to_agent":
		return a.requestDelegation(ctx, correlationID, tc, channelMetadata, attachments)

	case "create_workflow":
		return a.requestWorkflow(ctx, correlationID, tc, originalMessage, channelMetadata, attachments)

	default:
		return tools.ExecErr(fmt.Errorf("unknown meta-tool: %s", tc.Function.Name))
	}
}

// requestDelegation delegates a task to another agent and waits for result
func (a *Agent) requestDelegation(ctx context.Context, correlationID string,
	tc tools.ToolCall, channelMetadata *core.ChannelMetadata,
	attachments []*media.Attachment,
) tools.ToolExecution {
	var input tools.DelegateInput
	if err := json.Unmarshal([]byte(tc.Function.Arguments), &input); err != nil {
		return tools.ExecErr(fmt.Errorf("invalid delegation args: %w", err))
	}
	// Scheduled delegation — just save the cron job, no immediate execution.
	if input.Schedule != "" {
		err := a.submitCronJob(ctx, input.Task, input.Schedule, core.Event{
			Type:    core.AgentExecute,
			AgentID: input.AgentID,
			Payload: core.AgentExecutePayload{
				Message:     input.Task,
				Channel:     channelMetadata,
				Attachments: attachments,
			},
		})
		if err != nil {
			return tools.ExecErr(err)
		}
		return tools.ExecDone(fmt.Sprintf("Recurring job scheduled.\nSchedule: %s", input.Schedule))
	}
	a.logger.Info("delegating task",
		"agent_id", input.AgentID,
		"task", input.Task,
		"send_directly_to_user", input.SendDirectlyToUser,
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
			Context:            input.Context,
			Attachments:        attachments,
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
		return tools.ExecErr(fmt.Errorf("request cancelled"))
	case <-a.ctx.Done():
		logger.Timing.EndPhase(correlationID, delegationPhase)
		return tools.ExecErr(fmt.Errorf("agent shutting down"))
	case <-time.After(5 * time.Second):
		logger.Timing.EndPhase(correlationID, delegationPhase)
		return tools.ExecErr(fmt.Errorf("failed to send delegation request"))
	}

	// If sending directly to user, return immediately
	// Executive doesn't wait for response
	if input.SendDirectlyToUser {
		logger.Timing.EndPhaseWithMetadata(correlationID, delegationPhase, map[string]any{
			"direct_response": true,
		})
		delegationAck := a.delegationAck(input.AgentID)
		return tools.ExecDone(delegationAck)
	}

	// Wait for result
	select {
	case resultEvt := <-replyCh:
		result, ok := resultEvt.Payload.(core.DelegationResultPayload)
		if !ok {
			logger.Timing.EndPhase(correlationID, delegationPhase)
			return tools.ExecErr(fmt.Errorf("invalid delegation result payload"))
		}

		if result.Error != "" {
			logger.Timing.EndPhaseWithMetadata(correlationID, delegationPhase, map[string]any{
				"error": "failed",
			})
			return tools.ExecErr(fmt.Errorf("delegation error: %s", result.Error))
		}

		logger.Timing.EndPhaseWithMetadata(correlationID, delegationPhase, map[string]any{
			"agent": input.AgentID,
		})
		a.logger.Info("delegation completed",
			"delegation_id", delegationID,
			"correlation_id", correlationID)

		return tools.ExecOK(result.Result)

	case <-ctx.Done():
		logger.Timing.EndPhase(correlationID, delegationPhase)
		return tools.ExecErr(fmt.Errorf("request cancelled"))
	case <-a.ctx.Done():
		logger.Timing.EndPhase(correlationID, delegationPhase)
		return tools.ExecErr(fmt.Errorf("agent shutting down"))
	case <-time.After(5 * time.Minute):
		logger.Timing.EndPhase(correlationID, delegationPhase)
		return tools.ExecErr(fmt.Errorf("delegation timeout"))
	}
}

// requestWorkflow - pass original message and channel
func (a *Agent) requestWorkflow(ctx context.Context, correlationID string,
	tc tools.ToolCall, originalMessage string,
	channel *core.ChannelMetadata,
	attachments []*media.Attachment,
) tools.ToolExecution {
	logger.Timing.StartPhase(correlationID, "workflow_creation")
	wf, schedule, err := a.parseWorkflow(correlationID, tc, originalMessage, channel)
	if err != nil {
		return tools.ExecErr(err)
	}
	logger.Timing.EndPhase(correlationID, "workflow_creation")

	if err := a.submitWorkflow(ctx, correlationID, wf, attachments); err != nil {
		return tools.ExecErr(err)
	}

	if schedule != "" {
		err := a.submitCronJob(ctx, wf.Name, schedule, core.Event{
			Type:    core.WorkflowInstantiate,
			AgentID: a.id,
			Payload: workflow.WorkflowInstantiatePayload{
				TemplateID: wf.ID,
			},
		})
		if err != nil {
			return tools.ExecErr(err)
		}
	}
	return tools.ExecDone(workflowAck(wf))
}

// submitWorkflow sends workflow to kernel (non-blocking)
func (a *Agent) submitWorkflow(ctx context.Context, correlationID string,
	wf *workflow.WorkflowDefinition,
	attachments []*media.Attachment,
) error {
	event := core.Event{
		Type:          core.WorkflowSubmitted,
		CorrelationID: correlationID,
		AgentID:       a.id,
		Payload: workflow.WorkflowPayload{
			Workflow:    *wf,
			Attachments: attachments,
		},
	}

	select {
	case a.outbox <- event:
		a.logger.Debug("workflow submitted", "workflow_id", wf.ID, "channel", wf.Channel)
		return nil
	case <-ctx.Done():
		return fmt.Errorf("request cancelled")
	case <-a.ctx.Done():
		return fmt.Errorf("agent shutting down")
	case <-time.After(5 * time.Second):
		return fmt.Errorf("failed to submit workflow")
	}
}

// parseWorkflow - capture original user query and context
func (a *Agent) parseWorkflow(correlationID string, tc tools.ToolCall, originalMessage string, channel *core.ChannelMetadata) (*workflow.WorkflowDefinition, string, error) {
	var input tools.CreateWorkflowInput
	if err := json.Unmarshal([]byte(tc.Function.Arguments), &input); err != nil {
		return nil, "", fmt.Errorf("invalid workflow args: %w", err)
	}

	a.logger.Info("creating workflow",
		"name", input.Name,
		"tasks", len(input.Steps))

	// Map old IDs to new UUIDs
	idMap := make(map[string]string)
	for _, t := range input.Steps {
		oldID := t.ID
		if oldID == "" {
			oldID = uuid.NewString()
			t.ID = oldID
		}
		idMap[oldID] = uuid.NewString()
	}

	// Create tasks in a single pass with remapped dependencies
	tasks := make([]*workflow.WFTaskDefinition, 0, len(input.Steps))
	for _, t := range input.Steps {
		// Remap dependencies using the idMap
		newDeps := make([]string, len(t.DependsOn))
		for i, dep := range t.DependsOn {
			if newID, ok := idMap[dep]; ok {
				newDeps[i] = newID
			} else {
				newDeps[i] = dep // fallback, should rarely happen
			}
		}

		tasks = append(tasks, &workflow.WFTaskDefinition{
			ID:              idMap[t.ID],
			Name:            t.Name,
			Description:     t.Task,
			Type:            "agent_execution",
			DependsOn:       newDeps,
			AssignedAgentID: t.AgentID,
			MaxRetries:      3,
		})
	}

	// Store original context in metadata
	metadata := map[string]string{
		"correlation_id": correlationID,
	}

	return &workflow.WorkflowDefinition{
		ID:              uuid.NewString(),
		Name:            input.Name,
		IsTemplate:      input.Schedule != "",
		Description:     fmt.Sprintf("Workflow created by executive for: %s", input.Name),
		Tasks:           tasks,
		CreatedBy:       a.id,
		Status:          "pending",
		Channel:         channel,
		OriginalMessage: originalMessage,
		Metadata:        metadata,
	}, input.Schedule, nil
}

func (a *Agent) submitCronJob(ctx context.Context, name, schedule string, event core.Event) error {
	cronEvent := core.Event{
		Type:    core.CronJobScheduled,
		AgentID: a.id,
		Payload: core.CronJobScheduledPayload{
			ID:       uuid.NewString(),
			Name:     name,
			Schedule: schedule,
			Event:    event, // full event
		},
	}
	select {
	case a.outbox <- cronEvent:
		return nil
	case <-ctx.Done():
		return fmt.Errorf("request cancelled")
	case <-a.ctx.Done():
		return fmt.Errorf("agent shutting down")
	case <-time.After(5 * time.Second):
		return fmt.Errorf("failed to submit cron job")
	}
}

// Executive uses this to resolve agent names
func (a *Agent) SetResolveAgentName(fn AgentNameResolver) {
	a.resolveAgentName = fn
}
