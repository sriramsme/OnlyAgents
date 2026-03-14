package kernel

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/sriramsme/OnlyAgents/pkg/core"
	"github.com/sriramsme/OnlyAgents/pkg/logger"
	"github.com/sriramsme/OnlyAgents/pkg/workflow"
)

// handleWorkflowSubmitted: Executive creates a multi-task workflow
func (k *Kernel) handleWorkflowSubmitted(evt core.Event) {
	payload, ok := evt.Payload.(workflow.WorkflowPayload)
	if !ok {
		k.logger.Error("invalid WorkflowSubmitted payload")
		return
	}
	workflowPhase := fmt.Sprintf("workflow_%s", payload.Workflow.ID)
	logger.Timing.StartPhase(evt.CorrelationID, workflowPhase)

	if k.workflow == nil {
		logger.Timing.EndPhase(evt.CorrelationID, workflowPhase)
		k.logger.Error("workflow engine not initialized")
		k.sendWorkflowError(evt, "Workflow engine unavailable")
		return
	}

	k.logger.Info("workflow submitted",
		"workflow_id", payload.Workflow.ID,
		"tasks", len(payload.Workflow.Tasks),
		"correlation_id", evt.CorrelationID)

	// Submit to workflow engine
	// Engine handles the DAG internally - no executive roundtrips per task
	if err := k.workflow.SubmitWorkflow(k.ctx, &payload.Workflow); err != nil {
		logger.Timing.EndPhase(evt.CorrelationID, workflowPhase)
		k.logger.Error("workflow submission failed",
			"workflow_id", payload.Workflow.ID,
			"error", err)
		k.sendWorkflowError(evt, err.Error())
		return
	}

	// Workflow engine now owns execution
	// It will fire WorkflowCompleted when ALL tasks done
	// Note: Phase ends when workflow completes
	k.logger.Debug("workflow accepted by engine",
		"workflow_id", payload.Workflow.ID)
}

// handleWorkflowCompleted: Workflow engine finished all tasks
func (k *Kernel) handleWorkflowCompleted(evt core.Event) {
	payload, ok := evt.Payload.(workflow.WorkflowResultPayload)
	if !ok {
		k.logger.Error("invalid WorkflowCompleted payload")
		return
	}

	workflowPhase := fmt.Sprintf("workflow_%s", payload.WorkflowID)
	logger.Timing.EndPhaseWithMetadata(evt.CorrelationID, workflowPhase, map[string]any{
		"status": payload.Status,
		"tasks":  len(payload.Results),
	})

	k.logger.Info("workflow completed",
		"workflow_id", payload.WorkflowID,
		"status", payload.Status,
		"tasks", len(payload.Results),
		"correlation_id", evt.CorrelationID)

	// Get the executive agent (who created the workflow)
	executive, err := k.agents.Get(payload.CreatedBy)
	if err != nil {
		k.logger.Error("executive agent not found", "agent_id", payload.CreatedBy)
		return
	}

	// Format workflow results into a prompt for executive to synthesize
	var message string
	if payload.Error != "" {
		message = fmt.Sprintf("The workflow (ID: %s) has failed with error: %s. Please inform the user.",
			payload.WorkflowID, payload.Error)
	} else {
		// Convert results to JSON for executive to process
		resultsJSON, err := json.Marshal(payload.Results)
		if err != nil {
			logger.Log.Warn("failed to marshal workflow results", "err", err)
			resultsJSON = []byte("failed to marshal results")
		}
		message = fmt.Sprintf("The user asked: \"%s\"\n\nYou created a workflow to handle this request. The workflow has completed successfully with the following task results:\n\n%s\n\nPlease synthesize these results into a coherent, natural response for the user that answers their original question.",
			payload.OriginalMessage, string(resultsJSON))
	}

	// Trigger executive to synthesize results
	synthesisEvent := core.Event{
		Type:          core.AgentExecute,
		CorrelationID: evt.CorrelationID,
		AgentID:       executive.ID(),
		Payload: core.AgentExecutePayload{
			Message:     message,
			MessageType: core.MessageTypeWorkflowCompleted,
			Channel:     payload.Channel,
		},
	}

	select {
	case executive.Inbox() <- synthesisEvent:
		k.logger.Debug("workflow synthesis task sent to executive")

	case <-time.After(5 * time.Second):
		k.logger.Error("failed to send workflow synthesis - executive inbox full",
			"workflow_id", payload.WorkflowID)

	case <-k.ctx.Done():
		k.logger.Info("shutdown in progress")
	}
}

// handleTaskAssigned: Workflow engine assigned task to agent
func (k *Kernel) handleTaskAssigned(evt core.Event) {
	payload, ok := evt.Payload.(workflow.WFTaskAssignedPayload)
	if !ok {
		k.logger.Error("invalid TaskAssigned payload")
		return
	}

	// Get target agent (determined by workflow engine based on task capabilities)
	agent, err := k.agents.Get(evt.AgentID)
	if err != nil {
		k.logger.Error("agent not found for task assignment",
			"agent_id", evt.AgentID,
			"task_id", payload.TaskID)

		// TODO: Notify workflow engine of failure
		return
	}

	k.logger.Debug("assigning task to agent",
		"agent_id", agent.ID(),
		"task_id", payload.TaskID,
		"workflow_id", payload.WorkflowID,
		"channel", payload.Channel)

	// Create agent execute event
	agentEvent := core.Event{
		Type:          core.AgentExecute,
		CorrelationID: evt.CorrelationID,
		AgentID:       agent.ID(),
		Payload: core.AgentExecutePayload{
			Message:     payload.Task,
			MessageType: core.MessageTypeWorkflowTask,
			Workflow: &core.WorkflowMetadata{
				WorkflowID: payload.WorkflowID,
				TaskID:     payload.TaskID,
				TaskName:   payload.TaskName,
			},
			Channel: payload.Channel,
		},
		ReplyTo: evt.ReplyTo, // Result goes back to workflow engine
	}

	// Send to agent
	select {
	case agent.Inbox() <- agentEvent:
		k.logger.Debug("task assigned to agent",
			"agent_id", agent.ID(),
			"task_id", payload.TaskID)

	case <-time.After(5 * time.Second):
		k.logger.Error("failed to assign task - agent inbox full",
			"agent_id", agent.ID(),
			"task_id", payload.TaskID)
		// TODO: Notify workflow engine

	case <-k.ctx.Done():
		return
	}
}

func (k *Kernel) handleTaskCompleted(evt core.Event) {
	payload, ok := evt.Payload.(workflow.WFTaskCompletedPayload)
	if !ok {
		k.logger.Error("invalid TaskCompleted payload")
		return
	}

	k.logger.Debug("task completed",
		"workflow_id", payload.WorkflowID,
		"task_id", payload.TaskID,
		"has_error", payload.Error != "")

	if k.workflow == nil {
		k.logger.Error("workflow engine not initialized")
		return
	}

	// Pass to workflow engine
	if err := k.workflow.HandleTaskCompleted(k.ctx, payload); err != nil {
		k.logger.Error("failed to handle task completion",
			"error", err,
			"workflow_id", payload.WorkflowID,
			"task_id", payload.TaskID)
	}
}

func (k *Kernel) sendWorkflowError(evt core.Event, errorMsg string) {
	// TODO: Send error back to executive
	k.logger.Error("workflow error", "error", errorMsg)
}
