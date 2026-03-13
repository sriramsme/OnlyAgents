package workflow

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"github.com/sriramsme/OnlyAgents/pkg/core"
	"github.com/sriramsme/OnlyAgents/pkg/storage"
)

// Engine manages workflow orchestration
type Engine struct {
	store  storage.Storage
	bus    chan<- core.Event
	logger *slog.Logger
	ctx    context.Context
	cancel context.CancelFunc
}

// NewEngine creates a workflow engine
func NewEngine(store storage.Storage, bus chan<- core.Event) *Engine {
	ctx, cancel := context.WithCancel(context.Background())
	return &Engine{
		store:  store,
		bus:    bus,
		logger: slog.Default().With("component", "workflow"),
		ctx:    ctx,
		cancel: cancel,
	}
}

// Start starts the workflow engine
func (e *Engine) Start() error {
	e.logger.Info("workflow engine started")
	return nil
}

// Stop stops the workflow engine
func (e *Engine) Stop() error {
	e.cancel()
	e.logger.Info("workflow engine stopped")
	return nil
}

// SubmitWorkflow submits a new workflow and assigns agents to tasks
func (e *Engine) SubmitWorkflow(ctx context.Context, workflow *WorkflowDefinition) error {
	// Validate
	if err := workflow.Validate(); err != nil {
		return fmt.Errorf("invalid workflow: %w", err)
	}

	e.logger.Debug("submitting workflow",
		"workflow_id", workflow.ID,
		"tasks", len(workflow.Tasks))

	// Create workflow record
	metadataJSON, err := json.Marshal(workflow.Metadata)
	if err != nil {
		return fmt.Errorf("marshal workflow metadata: %w", err)
	}
	channelJSON := ""
	if workflow.Channel != nil {
		b, err := json.Marshal(workflow.Channel)
		if err != nil {
			return fmt.Errorf("marshal channel: %w", err)
		}
		channelJSON = string(b)
	}

	w := &storage.Workflow{
		ID:              workflow.ID,
		Name:            workflow.Name,
		Description:     workflow.Description,
		CreatedBy:       workflow.CreatedBy,
		Status:          storage.WorkflowStatusRunning,
		ChannelJSON:     channelJSON,
		OriginalMessage: workflow.OriginalMessage,
		Metadata:        string(metadataJSON),
		CreatedAt:       storage.DBTime{Time: time.Now()},
		UpdatedAt:       storage.DBTime{Time: time.Now()},
	}

	if err := e.store.CreateWorkflow(ctx, w); err != nil {
		return fmt.Errorf("create workflow: %w", err)
	}

	// Create tasks and assign agents
	for _, taskDef := range workflow.Tasks {

		taskDef.Channel = workflow.Channel
		task := e.taskDefToStorage(taskDef, workflow.ID)

		if err := e.store.CreateWFTask(ctx, task); err != nil {
			return fmt.Errorf("create task %s: %w", task.ID, err)
		}

		e.logger.Debug("task created and assigned",
			"task_id", task.ID,
			"agent_id", taskDef.AssignedAgentID,
			"channel", taskDef.Channel)
	}

	// Fire TaskAssigned events for root tasks (no dependencies)
	for _, taskDef := range workflow.GetRootTasks() {
		e.fireTaskAssigned(taskDef, workflow.ID)
	}

	return nil
}

// HandleTaskCompleted processes task completion from sub-agents
func (e *Engine) HandleTaskCompleted(ctx context.Context, payload WFTaskCompletedPayload) error {
	e.logger.Debug("handling task completion",
		"workflow_id", payload.WorkflowID,
		"task_id", payload.TaskID,
		"has_error", payload.Error != "",
	)

	// Get task
	_, err := e.store.GetWFTask(ctx, payload.TaskID)
	if err != nil {
		return fmt.Errorf("get task: %w", err)
	}

	// Handle failure
	if payload.Error != "" {
		e.logger.Error("task failed",
			"task_id", payload.TaskID,
			"error", payload.Error)

		if err := e.store.UpdateWFTaskStatus(ctx, payload.TaskID, storage.WFTaskStatusFailed, payload.Error); err != nil {
			e.logger.Warn("failed to update task status to failed",
				"task_id", payload.TaskID,
				"error", err)
		}

		// TODO: Implement retry logic or workflow failure handling
		return nil
	}

	// Store result and mark complete
	var resultBytes []byte
	if payload.Result != nil {
		resultBytes, err = json.Marshal(payload.Result)
		if err != nil {
			e.logger.Warn("failed to marshal task result", "task_id", payload.TaskID, "error", err)
		}
	}
	if err := e.store.UpdateWFTaskResult(ctx, payload.TaskID, resultBytes); err != nil {
		e.logger.Warn("failed to update task result",
			"task_id", payload.TaskID,
			"error", err)
	}

	if err := e.store.UpdateWFTaskStatus(ctx, payload.TaskID, storage.WFTaskStatusCompleted, ""); err != nil {
		e.logger.Warn("failed to update task status to completed",
			"task_id", payload.TaskID,
			"error", err)
	}

	e.logger.Debug("task marked as completed", "task_id", payload.TaskID)

	// Check for dependent tasks
	dependents, err := e.store.GetDependentWFTasks(ctx, payload.TaskID)
	if err != nil {
		return fmt.Errorf("get dependent tasks: %w", err)
	}

	e.logger.Debug("checking dependents",
		"task_id", payload.TaskID,
		"dependents", len(dependents))

	for _, dep := range dependents {
		satisfied, err := e.store.AllDependenciesSatisfied(ctx, dep.ID)
		if err != nil {
			e.logger.Warn("failed to check dependencies",
				"task_id", dep.ID,
				"error", err)
			continue
		}

		if satisfied {
			e.logger.Debug("dependencies satisfied, firing task",
				"dependent_task_id", dep.ID)
			var channel *core.ChannelMetadata
			if dep.ChannelJSON != "" {
				err := json.Unmarshal([]byte(dep.ChannelJSON), &channel)
				if err != nil {
					e.logger.Warn("failed to unmarshal channel", "error", err)
				}
			}
			// Fire TaskAssigned for this dependent
			e.bus <- core.Event{
				Type:          core.TaskAssigned,
				CorrelationID: dep.ID,
				AgentID:       dep.AssignedAgentID,
				Payload: WFTaskAssignedPayload{
					WorkflowID: dep.WorkflowID,
					TaskID:     dep.ID,
					TaskName:   dep.Name,
					Task:       dep.Description,
					Channel:    channel,
				},
			}
		}
	}

	// Check if workflow is complete
	if err := e.checkWorkflowCompletion(ctx, payload.WorkflowID); err != nil {
		e.logger.Error("failed to check workflow completion", "error", err)
		return err
	}

	return nil
}

// checkWorkflowCompletion checks if all tasks are done and fires WorkflowCompleted
func (e *Engine) checkWorkflowCompletion(ctx context.Context, workflowID string) error {
	// Get all tasks for this workflow
	tasks, err := e.store.GetWFTasks(ctx, workflowID)
	if err != nil {
		return fmt.Errorf("get workflow tasks: %w", err)
	}

	allComplete := true
	anyFailed := false
	results := make(map[string]json.RawMessage)

	for _, task := range tasks {
		if task.Status == storage.WFTaskStatusFailed {
			anyFailed = true
			allComplete = false
			break
		}
		if task.Status != storage.WFTaskStatusCompleted {
			allComplete = false
			break
		}
		if task.Result != nil {
			results[task.ID] = json.RawMessage(task.Result)
		}
	}

	if !allComplete {
		e.logger.Debug("workflow not yet complete",
			"workflow_id", workflowID,
			"completed_tasks", len(results),
			"total_tasks", len(tasks))
		return nil
	}

	// All tasks complete - fire WorkflowCompleted
	workflow, err := e.store.GetWorkflow(ctx, workflowID)
	if err != nil {
		return fmt.Errorf("get workflow: %w", err)
	}

	// Parse metadata
	var metadata map[string]string
	if err := json.Unmarshal([]byte(workflow.Metadata), &metadata); err != nil {
		e.logger.Warn("failed to parse workflow metadata", "error", err)
		metadata = make(map[string]string)
	}

	// Extract channel metadata
	var channel *core.ChannelMetadata
	if workflow.ChannelJSON != "" {
		err := json.Unmarshal([]byte(workflow.ChannelJSON), &channel)
		if err != nil {
			e.logger.Warn("failed to unmarshal channel", "error", err)
		}
	}

	status := "completed"
	if anyFailed {
		status = "failed"
	}

	e.logger.Info("workflow completed",
		"workflow_id", workflowID,
		"status", status,
		"total_tasks", len(tasks))

	e.bus <- core.Event{
		Type: core.WorkflowCompleted,
		Payload: WorkflowResultPayload{
			WorkflowID:      workflowID,
			CreatedBy:       workflow.CreatedBy,
			Status:          status,
			Results:         results,
			Channel:         channel,
			OriginalMessage: workflow.OriginalMessage,
			Metadata:        metadata,
		},
	}

	workflowStatus := storage.WorkflowStatusCompleted
	if anyFailed {
		workflowStatus = storage.WorkflowStatusFailed
	}

	if err := e.store.UpdateWorkflowStatus(ctx, workflowID, workflowStatus); err != nil {
		e.logger.Warn("failed to update workflow status",
			"workflow_id", workflowID,
			"error", err)
		// Non-fatal - workflow already completed, this is just status tracking
	}

	return nil
}

// fireTaskAssigned fires a TaskAssigned event for a task
func (e *Engine) fireTaskAssigned(taskDef *WFTaskDefinition, workflowID string) {
	e.logger.Debug("firing task assigned",
		"task_id", taskDef.ID,
		"agent_id", taskDef.AssignedAgentID)

	e.bus <- core.Event{
		Type:          core.TaskAssigned,
		CorrelationID: taskDef.ID,
		AgentID:       taskDef.AssignedAgentID,
		Payload: WFTaskAssignedPayload{
			WorkflowID:      workflowID,
			TaskID:          taskDef.ID,
			TaskName:        taskDef.Name,
			Task:            taskDef.Description,
			Channel:         taskDef.Channel,
			OriginalMessage: taskDef.OriginalMessage,
		},
	}
}

// taskDefToStorage converts WFTaskDefinition to storage.Task
func (e *Engine) taskDefToStorage(def *WFTaskDefinition, workflowID string) *storage.WFTask {
	depsJSON, err := json.Marshal(def.DependsOn)
	if err != nil {
		e.logger.Warn("failed to marshal depends_on", "error", err)
		depsJSON = []byte("[]")
	}

	payloadJSON, err := json.Marshal(def.Payload)
	if err != nil {
		e.logger.Warn("failed to marshal payload", "error", err)
		payloadJSON = []byte("{}")
	}

	metadataJSON, err := json.Marshal(def.Metadata)
	if err != nil {
		e.logger.Warn("failed to marshal metadata", "error", err)
		metadataJSON = []byte("{}")
	}

	maxRetries := def.MaxRetries
	if maxRetries == 0 {
		maxRetries = 3 // default
	}

	channelJSON, err := json.Marshal(def.Channel)
	if err != nil {
		e.logger.Warn("failed to marshal channel", "error", err)
		channelJSON = []byte("{}")
	}

	return &storage.WFTask{
		ID:              def.ID,
		WorkflowID:      workflowID,
		Name:            def.Name,
		Description:     def.Description,
		Type:            storage.WFTaskType(def.Type),
		DependsOn:       string(depsJSON),
		ChannelJSON:     string(channelJSON),
		Payload:         string(payloadJSON),
		Status:          storage.WFTaskStatusPending,
		AssignedAgentID: def.AssignedAgentID,
		CreatedAt:       storage.DBTime{Time: time.Now()},
		RetryCount:      0,
		MaxRetries:      maxRetries,
		TimeoutSeconds:  int(def.Timeout.Seconds()),
		Metadata:        string(metadataJSON),
		UpdatedAt:       storage.DBTime{Time: time.Now()},
	}
}
