package workflow

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/sriramsme/OnlyAgents/pkg/core"
	"github.com/sriramsme/OnlyAgents/pkg/media"
)

// WorkflowDefinition represents a workflow at submission time (before storage)
type WorkflowDefinition struct {
	ID              string                `json:"id"`
	Name            string                `json:"name"`
	Description     string                `json:"description"`
	Tasks           []*WFTaskDefinition   `json:"tasks"`
	CreatedBy       string                `json:"created_by"` // Agent ID
	Status          string                `json:"status"`
	Channel         *core.ChannelMetadata `json:"channel,omitempty"` // ← explicit
	OriginalMessage string                `json:"original_message,omitempty"`
	Metadata        map[string]string     `json:"metadata,omitempty"`
}

// WFTaskDefinition represents a task at submission time
type WFTaskDefinition struct {
	ID              string                `json:"id"`
	Name            string                `json:"name"`
	Description     string                `json:"description"`
	Type            string                `json:"type"`
	DependsOn       []string              `json:"depends_on"`
	Payload         interface{}           `json:"payload"`
	AssignedAgentID string                `json:"assigned_agent_id,omitempty"`
	MaxRetries      int                   `json:"max_retries"`
	Timeout         time.Duration         `json:"timeout,omitempty"`
	Channel         *core.ChannelMetadata `json:"channel,omitempty"`
	OriginalMessage string                `json:"original_message,omitempty"`
	Metadata        map[string]string     `json:"metadata,omitempty"`
	Attachments     []*media.Attachment   `json:"attachments,omitempty"`
}

// WorkflowPayload: Submit workflow for execution
type WorkflowPayload struct {
	Workflow WorkflowDefinition `json:"workflow"` // *workflow.Workflow (avoid import cycle)
}

// WorkflowResultPayload: Workflow execution completed
type WorkflowResultPayload struct {
	WorkflowID      string                     `json:"workflow_id"`
	Status          string                     `json:"status"`  // completed, failed, cancelled
	Results         map[string]json.RawMessage `json:"results"` // Task ID → result
	Error           string                     `json:"error,omitempty"`
	CreatedBy       string                     `json:"created_by"` // Executive agent ID
	Channel         *core.ChannelMetadata      `json:"channel"`
	OriginalMessage string                     `json:"original_message"`
	Metadata        map[string]string          `json:"metadata"`
	Attachments     []*media.Attachment        `json:"attachments,omitempty"`
}

// TaskAssignedPayload: Workflow engine assigns task to agent
type WFTaskAssignedPayload struct {
	WorkflowID      string                `json:"workflow_id"`
	TaskID          string                `json:"task_id"`
	TaskName        string                `json:"task_name"`
	Task            string                `json:"task"` // Task description
	Context         map[string]any        `json:"context,omitempty"`
	Attachments     []*media.Attachment   `json:"attachments,omitempty"`
	Channel         *core.ChannelMetadata `json:"channel,omitempty"`
	OriginalMessage string                `json:"original_message,omitempty"`
}

type WFTaskCompletedPayload struct {
	WorkflowID string          `json:"workflow_id"`
	TaskID     string          `json:"task_id"`
	Result     json.RawMessage `json:"result,omitempty"`
	Error      string          `json:"error,omitempty"`
}

// Validate validates the workflow definition
func (w *WorkflowDefinition) Validate() error {
	if w.ID == "" {
		w.ID = uuid.NewString()
	}
	if len(w.Tasks) == 0 {
		return fmt.Errorf("workflow must have at least one task")
	}

	// Check for cyclic dependencies
	if hasCycle(w.Tasks) {
		return fmt.Errorf("workflow has cyclic dependencies")
	}

	return nil
}

// GetRootTasks returns tasks with no dependencies
func (w *WorkflowDefinition) GetRootTasks() []*WFTaskDefinition {
	var roots []*WFTaskDefinition
	for _, task := range w.Tasks {
		if len(task.DependsOn) == 0 {
			roots = append(roots, task)
		}
	}
	return roots
}

// hasCycle checks if task graph has cycles
func hasCycle(tasks []*WFTaskDefinition) bool {
	graph := make(map[string][]string)
	for _, task := range tasks {
		graph[task.ID] = task.DependsOn
	}

	visited := make(map[string]bool)
	recStack := make(map[string]bool)

	var dfs func(string) bool
	dfs = func(taskID string) bool {
		visited[taskID] = true
		recStack[taskID] = true

		for _, dep := range graph[taskID] {
			if !visited[dep] {
				if dfs(dep) {
					return true
				}
			} else if recStack[dep] {
				return true
			}
		}

		recStack[taskID] = false
		return false
	}

	for taskID := range graph {
		if !visited[taskID] {
			if dfs(taskID) {
				return true
			}
		}
	}

	return false
}
