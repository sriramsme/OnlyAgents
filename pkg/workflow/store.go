package workflow

import (
	"context"
	"database/sql"
	"encoding/json"

	"github.com/sriramsme/OnlyAgents/pkg/dbtypes"
)

// WorkflowStore manages workflow orchestration
type Store interface {
	// Workflows
	CreateWorkflow(ctx context.Context, workflow *Workflow) error
	GetWorkflow(ctx context.Context, id string) (*Workflow, error)
	UpdateWorkflowStatus(ctx context.Context, id string, status WorkflowStatus) error

	// Tasks
	CreateWFTask(ctx context.Context, task *WFTask) error
	GetWFTask(ctx context.Context, id string) (*WFTask, error)
	UpdateWFTaskStatus(ctx context.Context, id string, status WFTaskStatus, errorMsg string) error
	UpdateWFTaskResult(ctx context.Context, id string, result json.RawMessage) error
	GetWFTasks(ctx context.Context, workflowID string) ([]*WFTask, error)
	GetReadyWFTasks(ctx context.Context, limit int) ([]*WFTask, error)
	GetDependentWFTasks(ctx context.Context, taskID string) ([]*WFTask, error)
	AllDependenciesSatisfied(ctx context.Context, taskID string) (bool, error)
}

// Workflow types
type WorkflowStatus string

const (
	WorkflowStatusPending   WorkflowStatus = "pending"
	WorkflowStatusRunning   WorkflowStatus = "running"
	WorkflowStatusCompleted WorkflowStatus = "completed"
	WorkflowStatusFailed    WorkflowStatus = "failed"
	WorkflowStatusCancelled WorkflowStatus = "cancelled"
)

type Workflow struct {
	ID              string         `db:"id" json:"id"`
	IsTemplate      bool           `db:"is_template" json:"is_template,omitempty"`
	Name            string         `db:"name" json:"name"`
	Description     string         `db:"description" json:"description,omitempty"`
	CreatedBy       string         `db:"created_by" json:"created_by,omitempty"`
	Status          WorkflowStatus `db:"status" json:"status"`
	ChannelJSON     string         `db:"channel_json" json:"channel_json,omitempty"`
	OriginalMessage string         `db:"original_message" json:"original_message,omitempty"`
	Metadata        string         `db:"metadata" json:"metadata,omitempty"`
	CreatedAt       dbtypes.DBTime `db:"created_at" json:"created_at"`
	UpdatedAt       dbtypes.DBTime `db:"updated_at" json:"updated_at"`
}

type WFTaskType string

const (
	WFTaskTypeAgentExecution WFTaskType = "agent_execution"
	WFTaskTypeSkillExecution WFTaskType = "skill_execution"
	WFTaskTypeWebhook        WFTaskType = "webhook"
	WFTaskTypeDelay          WFTaskType = "delay"
	WFTaskTypeCondition      WFTaskType = "condition"
)

type WFTaskStatus string

const (
	WFTaskStatusPending   WFTaskStatus = "pending"
	WFTaskStatusQueued    WFTaskStatus = "queued"
	WFTaskStatusRunning   WFTaskStatus = "running"
	WFTaskStatusCompleted WFTaskStatus = "completed"
	WFTaskStatusFailed    WFTaskStatus = "failed"
	WFTaskStatusCancelled WFTaskStatus = "cancelled"
	WFTaskStatusBlocked   WFTaskStatus = "blocked"
)

type WFTask struct {
	ID                   string             `db:"id" json:"id"`
	WorkflowID           string             `db:"workflow_id" json:"workflow_id"`
	Name                 string             `db:"name" json:"name"`
	Description          string             `db:"description" json:"description,omitempty"`
	Type                 WFTaskType         `db:"type" json:"type"`
	ChannelJSON          string             `db:"channel_json" json:"channel_json,omitempty"`
	DependsOn            string             `db:"depends_on" json:"depends_on,omitempty"`
	RequiredCapabilities string             `db:"required_capabilities" json:"required_capabilities,omitempty"`
	Payload              string             `db:"payload" json:"payload,omitempty"`
	Attachments          string             `db:"attachments" json:"attachments,omitempty"`
	Status               WFTaskStatus       `db:"status" json:"status"`
	Result               []byte             `db:"result" json:"result,omitempty"`
	Error                sql.NullString     `db:"error" json:"error"`
	AssignedAgentID      string             `db:"assigned_agent_id" json:"assigned_agent_id,omitempty"`
	CreatedAt            dbtypes.DBTime     `db:"created_at" json:"created_at"`
	StartedAt            dbtypes.NullDBTime `db:"started_at" json:"started_at"`
	CompletedAt          dbtypes.NullDBTime `db:"completed_at" json:"completed_at"`
	RetryCount           int                `db:"retry_count" json:"retry_count,omitempty"`
	MaxRetries           int                `db:"max_retries" json:"max_retries,omitempty"`
	TimeoutSeconds       int                `db:"timeout_seconds" json:"timeout_seconds,omitempty"`
	Metadata             string             `db:"metadata" json:"metadata,omitempty"`
	UpdatedAt            dbtypes.DBTime     `db:"updated_at" json:"updated_at"`
}
