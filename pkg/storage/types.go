package storage

import (
	"database/sql"
	"time"
)

// TopicEntry is a topic extracted from a day's conversation along with
// its relative share of the day's message volume and the user's sentiment.
type TopicEntry struct {
	Topic        string  `json:"topic"`
	MessageShare float64 `json:"message_share"` // 0.0 – 1.0
	Sentiment    string  `json:"sentiment"`     // "enthusiastic", "neutral", "frustrated", etc.
}

// DailySummary is an LLM-generated compression of all messages from one day.
type DailySummary struct {
	ID              string                `db:"id" json:"id"`
	Date            DBTime                `db:"date" json:"date"`
	Summary         string                `db:"summary" json:"summary,omitempty"`
	KeyEvents       JSONSlice[string]     `db:"key_events" json:"key_events,omitempty"`
	Topics          JSONSlice[TopicEntry] `db:"topics" json:"topics,omitempty"`
	ConversationIDs JSONSlice[string]     `db:"conversation_ids" json:"conversation_ids,omitempty"`
}

// WeeklySummary compresses daily summaries for one week.
type WeeklySummary struct {
	ID           string            `db:"id" json:"id"`
	WeekStart    DBTime            `db:"week_start" json:"week_start"`
	WeekEnd      DBTime            `db:"week_end" json:"week_end"`
	Summary      string            `db:"summary" json:"summary,omitempty"`
	Themes       JSONSlice[string] `db:"themes" json:"themes,omitempty"`
	Achievements JSONSlice[string] `db:"achievements" json:"achievements,omitempty"`
}

// MonthlySummary compresses weekly summaries for one month.
type MonthlySummary struct {
	ID         string            `db:"id" json:"id"`
	Year       int               `db:"year" json:"year"`
	Month      int               `db:"month" json:"month"`
	Summary    string            `db:"summary" json:"summary,omitempty"`
	Highlights JSONSlice[string] `db:"highlights" json:"highlights,omitempty"`
	Statistics JSONMap           `db:"statistics" json:"statistics,omitempty"`
}

// YearlyArchive is the final compression tier — kept forever.
type YearlyArchive struct {
	ID          string            `db:"id" json:"id"`
	Year        int               `db:"year" json:"year"`
	Summary     string            `db:"summary" json:"summary,omitempty"`
	MajorEvents JSONSlice[string] `db:"major_events" json:"major_events,omitempty"`
	Statistics  JSONMap           `db:"statistics" json:"statistics,omitempty"`
}

// Fact is a persistent piece of knowledge about an entity extracted during summarisation.
// SupersededBy points to a newer conflicting fact's ID, or "".
type Fact struct {
	ID                   string  `db:"id" json:"id"`
	Entity               string  `db:"entity" json:"entity"`
	EntityType           string  `db:"entity_type" json:"entity_type,omitempty"`
	Fact                 string  `db:"fact" json:"fact"`
	Confidence           float64 `db:"confidence" json:"confidence,omitempty"`
	TimesSeen            int     `db:"times_seen" json:"times_seen,omitempty"`
	SourceConversationID string  `db:"source_conversation_id" json:"source_conversation_id,omitempty"`
	SourceSummaryDate    string  `db:"source_summary_date" json:"source_summary_date,omitempty"`
	SupersededBy         string  `db:"superseded_by" json:"superseded_by,omitempty"`
	FirstSeen            DBTime  `db:"first_seen" json:"first_seen"`
	LastConfirmed        DBTime  `db:"last_confirmed" json:"last_confirmed"`
}

// AgentState holds persistent per-agent runtime state.
type AgentState struct {
	AgentID               string            `db:"agent_id" json:"agent_id"`
	CurrentConversationID string            `db:"current_conversation_id" json:"current_conversation_id,omitempty"`
	Context               JSONMap           `db:"context" json:"context,omitempty"`
	Preferences           JSONMap           `db:"preferences" json:"preferences,omitempty"`
	Goals                 JSONSlice[string] `db:"goals" json:"goals,omitempty"`
	LastActive            DBTime            `db:"last_active" json:"last_active"`
}

// CalendarEvent is a native calendar entry.
type CalendarEvent struct {
	ID          string            `db:"id" json:"id"`
	Title       string            `db:"title" json:"title"`
	Description string            `db:"description" json:"description,omitempty"`
	StartTime   DBTime            `db:"start_time" json:"start_time"`
	EndTime     DBTime            `db:"end_time" json:"end_time"`
	AllDay      bool              `db:"all_day" json:"all_day,omitempty"`
	Location    string            `db:"location" json:"location,omitempty"`
	Recurrence  string            `db:"recurrence" json:"recurrence,omitempty"`
	Tags        JSONSlice[string] `db:"tags" json:"tags,omitempty"`
	CreatedAt   DBTime            `db:"created_at" json:"created_at"`
	UpdatedAt   DBTime            `db:"updated_at" json:"updated_at"`
}

// Note is a Markdown note.
type Note struct {
	ID        string            `db:"id" json:"id"`
	Title     string            `db:"title" json:"title"`
	Content   string            `db:"content" json:"content,omitempty"`
	Tags      JSONSlice[string] `db:"tags" json:"tags,omitempty"`
	Pinned    bool              `db:"pinned" json:"pinned,omitempty"`
	CreatedAt DBTime            `db:"created_at" json:"created_at"`
	UpdatedAt DBTime            `db:"updated_at" json:"updated_at"`
}

// Reminder is a one-shot or recurring reminder delivered via the agent's channel.
type Reminder struct {
	ID        string     `db:"id" json:"id"`
	Title     string     `db:"title" json:"title"`
	Body      string     `db:"body" json:"body,omitempty"`
	DueAt     DBTime     `db:"due_at" json:"due_at"`
	SentAt    NullDBTime `db:"sent_at" json:"sent_at"`
	Recurring string     `db:"recurring" json:"recurring,omitempty"`
	CreatedAt DBTime     `db:"created_at" json:"created_at"`
}

// Project groups related tasks together.
type Project struct {
	ID          string `db:"id" json:"id"`
	Name        string `db:"name" json:"name"`
	Description string `db:"description" json:"description,omitempty"`
	Color       string `db:"color" json:"color,omitempty"`
	CreatedAt   DBTime `db:"created_at" json:"created_at"`
	UpdatedAt   DBTime `db:"updated_at" json:"updated_at"`
}

// Task is a work item with optional project grouping and due date.
// Status: todo | in_progress | done | cancelled
// Priority: low | medium | high
type Task struct {
	ID          string            `db:"id" json:"id"`
	ProjectID   string            `db:"project_id" json:"project_id,omitempty"`
	Title       string            `db:"title" json:"title"`
	Body        string            `db:"body" json:"body,omitempty"`
	Status      string            `db:"status" json:"status"`
	Priority    string            `db:"priority" json:"priority,omitempty"`
	DueAt       NullDBTime        `db:"due_at" json:"due_at"`
	CompletedAt NullDBTime        `db:"completed_at" json:"completed_at"`
	Tags        JSONSlice[string] `db:"tags" json:"tags,omitempty"`
	CreatedAt   DBTime            `db:"created_at" json:"created_at"`
	UpdatedAt   DBTime            `db:"updated_at" json:"updated_at"`
}

// TaskFilter is used by ListTasks. All fields are optional — nil = no filter.
type TaskFilter struct {
	ProjectID *string    // filter by project; use pointer to "" to filter unprojectd tasks
	Status    *string    // "todo" | "in_progress" | "done" | "cancelled"
	Priority  *string    // "low" | "medium" | "high"
	DueFrom   *time.Time // inclusive lower bound on due_at
	DueTo     *time.Time // inclusive upper bound on due_at
}

type CronJob struct {
	ID           string  `db:"id" json:"id"`
	Name         string  `db:"name" json:"name"`
	Description  string  `db:"description" json:"description,omitempty"`
	Schedule     string  `db:"schedule" json:"schedule"`
	Enabled      bool    `db:"enabled" json:"enabled,omitempty"`
	EventType    string  `db:"event_type" json:"event_type"`
	EventPayload string  `db:"event_payload" json:"event_payload,omitempty"`
	LastRun      *DBTime `db:"last_run" json:"last_run,omitempty"` // pointer — nil means never ran
	LastStatus   string  `db:"last_status" json:"last_status,omitempty"`
	LastError    string  `db:"last_error" json:"last_error,omitempty"`
	CreatedAt    DBTime  `db:"created_at" json:"created_at"`
	UpdatedAt    DBTime  `db:"updated_at" json:"updated_at"`
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
	CreatedAt       DBTime         `db:"created_at" json:"created_at"`
	UpdatedAt       DBTime         `db:"updated_at" json:"updated_at"`
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
	ID                   string         `db:"id" json:"id"`
	WorkflowID           string         `db:"workflow_id" json:"workflow_id"`
	Name                 string         `db:"name" json:"name"`
	Description          string         `db:"description" json:"description,omitempty"`
	Type                 WFTaskType     `db:"type" json:"type"`
	ChannelJSON          string         `db:"channel_json" json:"channel_json,omitempty"`
	DependsOn            string         `db:"depends_on" json:"depends_on,omitempty"`
	RequiredCapabilities string         `db:"required_capabilities" json:"required_capabilities,omitempty"`
	Payload              string         `db:"payload" json:"payload,omitempty"`
	Attachments          string         `db:"attachments" json:"attachments,omitempty"`
	Status               WFTaskStatus   `db:"status" json:"status"`
	Result               []byte         `db:"result" json:"result,omitempty"`
	Error                sql.NullString `db:"error" json:"error"`
	AssignedAgentID      string         `db:"assigned_agent_id" json:"assigned_agent_id,omitempty"`
	CreatedAt            DBTime         `db:"created_at" json:"created_at"`
	StartedAt            NullDBTime     `db:"started_at" json:"started_at"`
	CompletedAt          NullDBTime     `db:"completed_at" json:"completed_at"`
	RetryCount           int            `db:"retry_count" json:"retry_count,omitempty"`
	MaxRetries           int            `db:"max_retries" json:"max_retries,omitempty"`
	TimeoutSeconds       int            `db:"timeout_seconds" json:"timeout_seconds,omitempty"`
	Metadata             string         `db:"metadata" json:"metadata,omitempty"`
	UpdatedAt            DBTime         `db:"updated_at" json:"updated_at"`
}
