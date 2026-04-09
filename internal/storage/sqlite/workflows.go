package sqlite

import (
	"context"
	"encoding/json"
	"time"

	"github.com/sriramsme/OnlyAgents/pkg/dbtypes"
	wfPkg "github.com/sriramsme/OnlyAgents/pkg/workflow"
)

// CreateWorkflow creates a new workflow
func (d *DB) CreateWorkflow(ctx context.Context, workflow *wfPkg.Workflow) error {
	_, err := d.db.ExecContext(ctx, `
        INSERT INTO workflows (id, name, description, created_by, status, channel_json, original_message, metadata, created_at, updated_at)
        VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
    `, workflow.ID, workflow.Name, workflow.Description, workflow.CreatedBy,
		workflow.Status, workflow.ChannelJSON, workflow.OriginalMessage, workflow.Metadata, workflow.CreatedAt, workflow.UpdatedAt)

	return wrap(err, "CreateWorkflow")
}

// GetWorkflow retrieves a workflow by ID
func (d *DB) GetWorkflow(ctx context.Context, id string) (*wfPkg.Workflow, error) {
	var w wfPkg.Workflow
	err := d.db.GetContext(ctx, &w, `
        SELECT * FROM workflows WHERE id = ?
    `, id)
	if err != nil {
		return nil, wrap(err, "GetWorkflow")
	}
	return &w, nil
}

// UpdateWorkflowStatus updates workflow status
func (d *DB) UpdateWorkflowStatus(ctx context.Context, id string, status wfPkg.WorkflowStatus) error {
	_, err := d.db.ExecContext(ctx, `
        UPDATE workflows SET status = ?, updated_at = ? WHERE id = ?
    `, status, dbtypes.DBTime{Time: time.Now()}, id)

	return wrap(err, "UpdateWorkflowStatus")
}

// CreateTask creates a new task
func (d *DB) CreateWFTask(ctx context.Context, task *wfPkg.WFTask) error {
	_, err := d.db.ExecContext(ctx, `
        INSERT INTO wf_tasks (
            id, workflow_id, name, description, type, depends_on, channel_json, attachments,
            payload, status, assigned_agent_id, created_at, retry_count, max_retries,
            timeout_seconds, metadata, updated_at
        ) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
    `, task.ID, task.WorkflowID, task.Name, task.Description, task.Type,
		task.DependsOn, task.ChannelJSON, task.Attachments, task.Payload, task.Status,
		task.AssignedAgentID, task.CreatedAt, task.RetryCount, task.MaxRetries,
		task.TimeoutSeconds, task.Metadata, task.UpdatedAt)

	return wrap(err, "CreateWFTask")
}

// GetTask retrieves a task by ID
func (d *DB) GetWFTask(ctx context.Context, id string) (*wfPkg.WFTask, error) {
	var t wfPkg.WFTask
	err := d.db.GetContext(ctx, &t, `
        SELECT * FROM wf_tasks WHERE id = ?
    `, id)
	if err != nil {
		return nil, wrap(err, "GetWFTask")
	}
	return &t, nil
}

// UpdateTaskStatus updates task status and timestamps
func (d *DB) UpdateWFTaskStatus(ctx context.Context, id string, status wfPkg.WFTaskStatus, errorMsg string) error {
	now := dbtypes.DBTime{Time: time.Now()}

	// Build query based on status
	var query string
	var args []interface{}

	switch status {
	case wfPkg.WFTaskStatusRunning:
		query = `UPDATE wf_tasks SET status = ?, error = ?, started_at = ?, updated_at = ? WHERE id = ?`
		args = []interface{}{status, errorMsg, now, now, id}
	case wfPkg.WFTaskStatusCompleted, wfPkg.WFTaskStatusFailed:
		query = `UPDATE wf_tasks SET status = ?, error = ?, completed_at = ?, updated_at = ? WHERE id = ?`
		args = []interface{}{status, errorMsg, now, now, id}
	default:
		query = `UPDATE wf_tasks SET status = ?, error = ?, updated_at = ? WHERE id = ?`
		args = []interface{}{status, errorMsg, now, id}
	}

	_, err := d.db.ExecContext(ctx, query, args...)
	return wrap(err, "UpdateWFTaskStatus")
}

// UpdateTaskResult updates task result
func (d *DB) UpdateWFTaskResult(ctx context.Context, id string, result json.RawMessage) error {
	_, err := d.db.ExecContext(ctx, `
        UPDATE wf_tasks SET result = ?, updated_at = ? WHERE id = ?
    `, result, dbtypes.DBTime{Time: time.Now()}, id)

	return wrap(err, "UpdateWFTaskResult")
}

// GetWorkflowTasks returns all tasks for a workflow
func (d *DB) GetWFTasks(ctx context.Context, workflowID string) ([]*wfPkg.WFTask, error) {
	var tasks []*wfPkg.WFTask
	err := d.db.SelectContext(ctx, &tasks, `
        SELECT * FROM wf_tasks
        WHERE workflow_id = ?
        ORDER BY created_at ASC
    `, workflowID)
	if err != nil {
		return nil, wrap(err, "GetWFTasks")
	}
	return tasks, nil
}

// GetReadyTasks returns queued tasks ready to execute
func (d *DB) GetReadyWFTasks(ctx context.Context, limit int) ([]*wfPkg.WFTask, error) {
	var tasks []*wfPkg.WFTask
	err := d.db.SelectContext(ctx, &tasks, `
        SELECT * FROM wf_tasks
        WHERE status = ?
        ORDER BY created_at ASC
        LIMIT ?
    `, wfPkg.WFTaskStatusQueued, limit)
	if err != nil {
		return nil, wrap(err, "GetReadyWFTasks")
	}
	return tasks, nil
}

// GetDependentTasks returns tasks that depend on the given task
func (d *DB) GetDependentWFTasks(ctx context.Context, taskID string) ([]*wfPkg.WFTask, error) {
	var tasks []*wfPkg.WFTask
	err := d.db.SelectContext(ctx, &tasks, `
        SELECT * FROM wf_tasks
        WHERE depends_on LIKE ?
    `, "%"+taskID+"%")
	if err != nil {
		return nil, wrap(err, "GetDependentWFTasks")
	}

	// Filter to exact matches (not just substrings)
	var filtered []*wfPkg.WFTask
	for _, task := range tasks {
		var deps []string
		if err := json.Unmarshal([]byte(task.DependsOn), &deps); err != nil {
			continue
		}
		for _, dep := range deps {
			if dep == taskID {
				filtered = append(filtered, task)
				break
			}
		}
	}

	return filtered, nil
}

// AllDependenciesSatisfied checks if all task dependencies are completed
func (d *DB) AllDependenciesSatisfied(ctx context.Context, taskID string) (bool, error) {
	var dependsOnJSON string
	err := d.db.GetContext(ctx, &dependsOnJSON, `
        SELECT depends_on FROM wf_tasks WHERE id = ?
    `, taskID)
	if err != nil {
		return false, wrap(err, "AllDependenciesSatisfied")
	}

	var deps []string
	if err := json.Unmarshal([]byte(dependsOnJSON), &deps); err != nil {
		return false, wrap(err, "AllDependenciesSatisfied: unmarshal")
	}

	if len(deps) == 0 {
		return true, nil
	}

	// Check each dependency
	for _, depID := range deps {
		var status wfPkg.WFTaskStatus
		err := d.db.GetContext(ctx, &status, `SELECT status FROM wf_tasks WHERE id = ?`, depID)
		if err != nil || status != wfPkg.WFTaskStatusCompleted {
			return false, nil
		}
	}

	return true, nil
}
