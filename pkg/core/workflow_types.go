package core

import (
	"encoding/json"
	"time"
)

// ExecutiveAnalysis represents the executive agent's task analysis
type ExecutiveAnalysis struct {
	// Task breakdown
	Tasks []Task `json:"tasks"`

	// Capabilities needed across all tasks
	RequiredCapabilities []Capability `json:"required_capabilities"`

	// Suggested agent routing
	RoutingDecision RoutingDecision `json:"routing_decision"`
}

// RoutingDecision tells kernel how to route
type RoutingDecision struct {
	Strategy     RoutingStrategy `json:"strategy"`      // specialized, general, parallel
	AgentID      string          `json:"agent_id"`      // Specific agent (if specialized)
	TaskSequence []TaskRouting   `json:"task_sequence"` // For complex workflows
}

type RoutingStrategy string

const (
	RoutingSpecialized RoutingStrategy = "specialized" // Use specialized agent
	RoutingGeneral     RoutingStrategy = "general"     // Use general agent with dynamic tools
	RoutingParallel    RoutingStrategy = "parallel"    // Execute multiple tasks in parallel
	RoutingSequential  RoutingStrategy = "sequential"  // Execute tasks one by one
)

// TaskRouting maps a task to an agent
type TaskRouting struct {
	TaskID  string   `json:"task_id"`
	AgentID string   `json:"agent_id"`
	Tools   []string `json:"tools,omitempty"` // Optional: specific tools to enable
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
