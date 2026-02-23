package workflow

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/sriramsme/OnlyAgents/pkg/core"
)

// ====================
// Workflow Engine
// ====================

// Engine manages task orchestration with dependencies
type Engine struct {
	db       *sql.DB
	queue    *TaskQueue
	executor *TaskExecutor
	bus      chan<- core.Event
	ctx      context.Context
	cancel   context.CancelFunc
	wg       sync.WaitGroup
}

// NewEngine creates a workflow engine
func NewEngine(db *sql.DB, bus chan<- core.Event) (*Engine, error) {
	ctx, cancel := context.WithCancel(context.Background())

	queue, err := NewTaskQueue(db)
	if err != nil {
		cancel()
		return nil, err
	}

	executor := NewTaskExecutor(db, bus)

	return &Engine{
		db:       db,
		queue:    queue,
		executor: executor,
		bus:      bus,
		ctx:      ctx,
		cancel:   cancel,
	}, nil
}

// Start starts the workflow engine
func (e *Engine) Start() error {
	if err := e.queue.Initialize(); err != nil {
		return fmt.Errorf("initialize queue: %w", err)
	}

	// Start task processor
	e.wg.Add(1)
	go e.processLoop()

	return nil
}

// Stop stops the workflow engine
func (e *Engine) Stop() error {
	e.cancel()
	e.wg.Wait()
	return nil
}

// SubmitWorkflow submits a new workflow (collection of tasks)
func (e *Engine) SubmitWorkflow(ctx context.Context, workflow *Workflow) error {
	// Validate workflow
	if err := workflow.Validate(); err != nil {
		return fmt.Errorf("invalid workflow: %w", err)
	}

	// Store workflow
	if err := e.queue.StoreWorkflow(ctx, workflow); err != nil {
		return fmt.Errorf("store workflow: %w", err)
	}

	// Enqueue root tasks (tasks with no dependencies)
	rootTasks := workflow.GetRootTasks()
	for _, task := range rootTasks {
		if err := e.queue.EnqueueTask(ctx, task); err != nil {
			return fmt.Errorf("enqueue task %s: %w", task.ID, err)
		}
	}

	return nil
}

// processLoop processes tasks from the queue
func (e *Engine) processLoop() {
	defer e.wg.Done()

	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			e.processPendingTasks()
		case <-e.ctx.Done():
			return
		}
	}
}

// processPendingTasks processes all ready tasks
func (e *Engine) processPendingTasks() {
	ctx := context.Background()

	// Get ready tasks (dependencies satisfied)
	tasks, err := e.queue.GetReadyTasks(ctx, 10)
	if err != nil {
		// Log error
		return
	}

	for _, task := range tasks {
		// Execute task in goroutine
		go func(t *Task) {
			if err := e.executor.Execute(ctx, t); err != nil {
				// Mark task as failed
				err = e.queue.UpdateTaskStatus(ctx, t.ID, TaskStatusFailed, err.Error())
				if err != nil {
					fmt.Printf("update task status: %s", err)
					return
				}
				return
			}

			// Mark task as completed
			err = e.queue.UpdateTaskStatus(ctx, t.ID, TaskStatusCompleted, "")
			if err != nil {
				return
			}
			// Check for dependent tasks that are now ready
			e.checkAndEnqueueDependents(ctx, t)
		}(task)
	}
}

// checkAndEnqueueDependents enqueues tasks that depend on this completed task
func (e *Engine) checkAndEnqueueDependents(ctx context.Context, completedTask *Task) {
	// Get all tasks that depend on this task
	dependents, err := e.queue.GetDependentTasks(ctx, completedTask.ID)
	if err != nil {
		return
	}

	for _, dep := range dependents {
		// Check if all dependencies are satisfied
		if e.queue.AllDependenciesSatisfied(ctx, dep.ID) {
			err = e.queue.EnqueueTask(ctx, dep)
			if err != nil {
				fmt.Printf("enqueue dependent task: %s", err)
				return
			}
		}
	}
}

// ====================
// Workflow Definition
// ====================

// Workflow represents a collection of tasks with dependencies
type Workflow struct {
	ID          string            `json:"id"`
	Name        string            `json:"name"`
	Description string            `json:"description"`
	Tasks       []*Task           `json:"tasks"`
	CreatedAt   time.Time         `json:"created_at"`
	CreatedBy   string            `json:"created_by"` // Agent ID
	Status      WorkflowStatus    `json:"status"`
	Metadata    map[string]string `json:"metadata,omitempty"`
}

type WorkflowStatus string

const (
	WorkflowStatusPending   WorkflowStatus = "pending"
	WorkflowStatusRunning   WorkflowStatus = "running"
	WorkflowStatusCompleted WorkflowStatus = "completed"
	WorkflowStatusFailed    WorkflowStatus = "failed"
	WorkflowStatusCancelled WorkflowStatus = "cancelled"
)

// Validate checks if workflow is valid
func (w *Workflow) Validate() error {
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
func (w *Workflow) GetRootTasks() []*Task {
	var roots []*Task
	for _, task := range w.Tasks {
		if len(task.DependsOn) == 0 {
			roots = append(roots, task)
		}
	}
	return roots
}

// ====================
// Task Definition
// ====================

// Task represents a unit of work
type Task struct {
	ID                   string            `json:"id"`
	WorkflowID           string            `json:"workflow_id"`
	Name                 string            `json:"name"`
	Description          string            `json:"description"`
	Type                 TaskType          `json:"type"`
	DependsOn            []string          `json:"depends_on"` // Task IDs
	RequiredCapabilities []string          `json:"required_capabilities"`
	Payload              json.RawMessage   `json:"payload"` // Task-specific data
	Status               TaskStatus        `json:"status"`
	Result               json.RawMessage   `json:"result,omitempty"`
	Error                string            `json:"error,omitempty"`
	AssignedAgentID      string            `json:"assigned_agent_id,omitempty"`
	CreatedAt            time.Time         `json:"created_at"`
	StartedAt            *time.Time        `json:"started_at,omitempty"`
	CompletedAt          *time.Time        `json:"completed_at,omitempty"`
	RetryCount           int               `json:"retry_count"`
	MaxRetries           int               `json:"max_retries"`
	Timeout              time.Duration     `json:"timeout,omitempty"`
	Metadata             map[string]string `json:"metadata,omitempty"`
}

type TaskType string

const (
	TaskTypeAgentExecution TaskType = "agent_execution" // Execute via agent
	TaskTypeSkillExecution TaskType = "skill_execution" // Execute skill directly
	TaskTypeWebhook        TaskType = "webhook"         // HTTP callback
	TaskTypeDelay          TaskType = "delay"           // Wait for duration
	TaskTypeCondition      TaskType = "condition"       // Branch based on condition
)

type TaskStatus string

const (
	TaskStatusPending   TaskStatus = "pending"
	TaskStatusQueued    TaskStatus = "queued"
	TaskStatusRunning   TaskStatus = "running"
	TaskStatusCompleted TaskStatus = "completed"
	TaskStatusFailed    TaskStatus = "failed"
	TaskStatusCancelled TaskStatus = "cancelled"
	TaskStatusBlocked   TaskStatus = "blocked" // Waiting for dependencies
)

// ====================
// Task Queue (Persistence Layer)
// ====================

// TaskQueue manages task persistence and retrieval
type TaskQueue struct {
	db *sql.DB
}

// NewTaskQueue creates a task queue
func NewTaskQueue(db *sql.DB) (*TaskQueue, error) {
	return &TaskQueue{db: db}, nil
}

// Initialize creates database tables
func (q *TaskQueue) Initialize() error {
	schema := `
	CREATE TABLE IF NOT EXISTS workflows (
		id TEXT PRIMARY KEY,
		name TEXT NOT NULL,
		description TEXT,
		created_at TIMESTAMP NOT NULL,
		created_by TEXT NOT NULL,
		status TEXT NOT NULL,
		metadata TEXT,
		updated_at TIMESTAMP NOT NULL
	);

	CREATE TABLE IF NOT EXISTS tasks (
		id TEXT PRIMARY KEY,
		workflow_id TEXT NOT NULL,
		name TEXT NOT NULL,
		description TEXT,
		type TEXT NOT NULL,
		depends_on TEXT,  -- JSON array
		required_capabilities TEXT,  -- JSON array
		payload TEXT,  -- JSON
		status TEXT NOT NULL,
		result TEXT,  -- JSON
		error TEXT,
		assigned_agent_id TEXT,
		created_at TIMESTAMP NOT NULL,
		started_at TIMESTAMP,
		completed_at TIMESTAMP,
		retry_count INTEGER DEFAULT 0,
		max_retries INTEGER DEFAULT 3,
		timeout_seconds INTEGER,
		metadata TEXT,  -- JSON
		updated_at TIMESTAMP NOT NULL,
		FOREIGN KEY (workflow_id) REFERENCES workflows(id)
	);

	CREATE INDEX IF NOT EXISTS idx_tasks_workflow ON tasks(workflow_id);
	CREATE INDEX IF NOT EXISTS idx_tasks_status ON tasks(status);
	CREATE INDEX IF NOT EXISTS idx_tasks_created ON tasks(created_at);
	`

	_, err := q.db.Exec(schema)
	return err
}

// StoreWorkflow stores a workflow
func (q *TaskQueue) StoreWorkflow(ctx context.Context, workflow *Workflow) error {
	metadataJSON, err := json.Marshal(workflow.Metadata)
	if err != nil {
		return fmt.Errorf("marshal metadata: %w", err)
	}

	_, err = q.db.ExecContext(ctx, `
		INSERT INTO workflows (id, name, description, created_at, created_by, status, metadata, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
	`, workflow.ID, workflow.Name, workflow.Description, workflow.CreatedAt, workflow.CreatedBy,
		workflow.Status, metadataJSON, time.Now())

	if err != nil {
		return err
	}

	// Store tasks
	for _, task := range workflow.Tasks {
		task.WorkflowID = workflow.ID
		if err := q.storeTask(ctx, task); err != nil {
			return err
		}
	}

	return nil
}

// storeTask stores a single task
func (q *TaskQueue) storeTask(ctx context.Context, task *Task) error {
	dependsOnJSON, err := json.Marshal(task.DependsOn)
	if err != nil {
		return fmt.Errorf("marshal depends_on: %w", err)
	}
	capsJSON, err := json.Marshal(task.RequiredCapabilities)
	if err != nil {
		return fmt.Errorf("marshal required_capabilities: %w", err)
	}
	metadataJSON, err := json.Marshal(task.Metadata)
	if err != nil {
		return fmt.Errorf("marshal metadata: %w", err)
	}

	_, err = q.db.ExecContext(ctx, `
		INSERT INTO tasks (
			id, workflow_id, name, description, type, depends_on, required_capabilities,
			payload, status, assigned_agent_id, created_at, retry_count, max_retries,
			timeout_seconds, metadata, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, task.ID, task.WorkflowID, task.Name, task.Description, task.Type,
		dependsOnJSON, capsJSON, task.Payload, task.Status, task.AssignedAgentID,
		task.CreatedAt, task.RetryCount, task.MaxRetries,
		int(task.Timeout.Seconds()), metadataJSON, time.Now())

	return err
}

// EnqueueTask marks a task as queued (ready to execute)
func (q *TaskQueue) EnqueueTask(ctx context.Context, task *Task) error {
	task.Status = TaskStatusQueued
	return q.UpdateTaskStatus(ctx, task.ID, TaskStatusQueued, "")
}

// GetReadyTasks returns tasks that are ready to execute
func (q *TaskQueue) GetReadyTasks(ctx context.Context, limit int) ([]*Task, error) {
	rows, err := q.db.QueryContext(ctx, `
		SELECT id, workflow_id, name, description, type, depends_on, required_capabilities,
		       payload, status, assigned_agent_id, created_at, retry_count, max_retries,
		       timeout_seconds, metadata
		FROM tasks
		WHERE status = ?
		ORDER BY created_at ASC
		LIMIT ?
	`, TaskStatusQueued, limit)
	if err != nil {
		return nil, err
	}
	defer func() {
		if cerr := rows.Close(); cerr != nil {
			// log or ignore
			_ = cerr
		}
	}()

	var tasks []*Task
	for rows.Next() {
		task, err := q.scanTask(rows)
		if err != nil {
			return nil, err
		}
		tasks = append(tasks, task)
	}

	return tasks, nil
}

// UpdateTaskStatus updates task status
func (q *TaskQueue) UpdateTaskStatus(ctx context.Context, taskID string, status TaskStatus, errorMsg string) error {
	now := time.Now()
	var startedAt, completedAt interface{}

	if status == TaskStatusRunning {
		startedAt = now
	}
	if status == TaskStatusCompleted || status == TaskStatusFailed {
		completedAt = now
	}

	_, err := q.db.ExecContext(ctx, `
		UPDATE tasks
		SET status = ?, error = ?, started_at = COALESCE(started_at, ?),
		    completed_at = ?, updated_at = ?
		WHERE id = ?
	`, status, errorMsg, startedAt, completedAt, now, taskID)

	return err
}

// GetDependentTasks returns tasks that depend on the given task
func (q *TaskQueue) GetDependentTasks(ctx context.Context, taskID string) ([]*Task, error) {
	// Query tasks where depends_on JSON array contains taskID
	rows, err := q.db.QueryContext(ctx, `
		SELECT id, workflow_id, name, description, type, depends_on, required_capabilities,
		       payload, status, assigned_agent_id, created_at, retry_count, max_retries,
		       timeout_seconds, metadata
		FROM tasks
		WHERE depends_on LIKE ?
	`, "%"+taskID+"%")
	if err != nil {
		return nil, err
	}
	defer func() {
		if cerr := rows.Close(); cerr != nil {
			_ = cerr
		}
	}()

	var tasks []*Task
	for rows.Next() {
		task, err := q.scanTask(rows)
		if err != nil {
			return nil, err
		}

		// Verify taskID is actually in depends_on (not just a substring match)
		for _, dep := range task.DependsOn {
			if dep == taskID {
				tasks = append(tasks, task)
				break
			}
		}
	}

	return tasks, nil
}

// AllDependenciesSatisfied checks if all dependencies are completed
func (q *TaskQueue) AllDependenciesSatisfied(ctx context.Context, taskID string) bool {
	var dependsOnJSON string
	err := q.db.QueryRowContext(ctx, `
		SELECT depends_on FROM tasks WHERE id = ?
	`, taskID).Scan(&dependsOnJSON)
	if err != nil {
		return false
	}

	var dependsOn []string
	if err := json.Unmarshal([]byte(dependsOnJSON), &dependsOn); err != nil {
		return false
	}

	if len(dependsOn) == 0 {
		return true
	}

	// Check if all dependencies are completed
	for _, depID := range dependsOn {
		var status TaskStatus
		err := q.db.QueryRowContext(ctx, `
			SELECT status FROM tasks WHERE id = ?
		`, depID).Scan(&status)
		if err != nil || status != TaskStatusCompleted {
			return false
		}
	}

	return true
}

// scanTask scans a task from a row
func (q *TaskQueue) scanTask(rows *sql.Rows) (*Task, error) {
	var task Task
	var dependsOnJSON, capsJSON, metadataJSON string
	var timeoutSec int

	err := rows.Scan(
		&task.ID, &task.WorkflowID, &task.Name, &task.Description, &task.Type,
		&dependsOnJSON, &capsJSON, &task.Payload, &task.Status, &task.AssignedAgentID,
		&task.CreatedAt, &task.RetryCount, &task.MaxRetries, &timeoutSec, &metadataJSON,
	)
	if err != nil {
		return nil, err
	}

	if err := json.Unmarshal([]byte(dependsOnJSON), &task.DependsOn); err != nil {
		return nil, fmt.Errorf("unmarshal depends_on: %w", err)
	}
	if err := json.Unmarshal([]byte(capsJSON), &task.RequiredCapabilities); err != nil {
		return nil, fmt.Errorf("unmarshal required_capabilities: %w", err)
	}
	if err := json.Unmarshal([]byte(metadataJSON), &task.Metadata); err != nil {
		return nil, fmt.Errorf("unmarshal metadata: %w", err)
	}
	task.Timeout = time.Duration(timeoutSec) * time.Second

	return &task, nil
}

// ====================
// Task Executor
// ====================

// TaskExecutor executes tasks
type TaskExecutor struct {
	db  *sql.DB
	bus chan<- core.Event
}

// NewTaskExecutor creates a task executor
func NewTaskExecutor(db *sql.DB, bus chan<- core.Event) *TaskExecutor {
	return &TaskExecutor{db: db, bus: bus}
}

// Execute executes a task
func (e *TaskExecutor) Execute(ctx context.Context, task *Task) error {
	switch task.Type {
	case TaskTypeAgentExecution:
		return e.executeViaAgent(ctx, task)
	case TaskTypeSkillExecution:
		return e.executeViaSkill(ctx, task)
	case TaskTypeDelay:
		return e.executeDelay(ctx, task)
	default:
		return fmt.Errorf("unsupported task type: %s", task.Type)
	}
}

// executeViaAgent executes task via an agent
func (e *TaskExecutor) executeViaAgent(ctx context.Context, task *Task) error {
	// Parse payload
	var payload struct {
		Message string `json:"message"`
	}
	if err := json.Unmarshal(task.Payload, &payload); err != nil {
		return fmt.Errorf("unmarshal agent payload: %w", err)
	}

	// Fire AgentExecute event
	e.bus <- core.Event{
		Type:          core.AgentExecute,
		CorrelationID: task.ID,
		AgentID:       task.AssignedAgentID,
		Payload: core.AgentExecutePayload{
			UserMessage: payload.Message,
			Metadata: map[string]string{
				"task_id":     task.ID,
				"workflow_id": task.WorkflowID,
			},
		},
	}

	// Wait for result (async - executor should listen for TaskCompleted event)
	return nil
}

// executeViaSkill executes task directly via skill
func (e *TaskExecutor) executeViaSkill(ctx context.Context, task *Task) error {
	// Fire ToolCallRequest event
	var payload struct {
		SkillName string         `json:"skill_name"`
		ToolName  string         `json:"tool_name"`
		Params    map[string]any `json:"params"`
	}
	if err := json.Unmarshal(task.Payload, &payload); err != nil {
		return fmt.Errorf("unmarshal skill payload: %w", err)
	}

	e.bus <- core.Event{
		Type:          core.ToolCallRequest,
		CorrelationID: task.ID,
		Payload: core.ToolCallRequestPayload{
			ToolCallID: task.ID,
			SkillName:  payload.SkillName,
			ToolName:   payload.ToolName,
			Params:     payload.Params,
		},
	}

	return nil
}

// executeDelay waits for specified duration
func (e *TaskExecutor) executeDelay(ctx context.Context, task *Task) error {
	var payload struct {
		Duration time.Duration `json:"duration"`
	}
	if err := json.Unmarshal(task.Payload, &payload); err != nil {
		return fmt.Errorf("unmarshal delay payload: %w", err)
	}

	time.Sleep(payload.Duration)
	return nil
}

// ====================
// Utility Functions
// ====================

// hasCycle checks if task graph has cycles
func hasCycle(tasks []*Task) bool {
	// Build adjacency list
	graph := make(map[string][]string)
	for _, task := range tasks {
		graph[task.ID] = task.DependsOn
	}

	// DFS for cycle detection
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
