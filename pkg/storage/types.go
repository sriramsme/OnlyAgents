package storage

// Conversation is a single session between a user and an agent.
type Conversation struct {
	ID          string     `db:"id"`
	AgentID     string     `db:"agent_id"`
	StartedAt   DBTime     `db:"started_at"`
	EndedAt     NullDBTime `db:"ended_at"`
	Context     JSONMap    `db:"context"`
	Summary     string     `db:"summary"`
	PeerAgentID string     `db:"peer_agent_id"` // A2A: remote agent ID, "" for local sessions
}

// Message is one turn within a Conversation.
type Message struct {
	ID               string `db:"id"`
	ConversationID   string `db:"conversation_id"`
	AgentID          string `db:"agent_id"`
	Role             string `db:"role"` // user | assistant | tool
	Content          string `db:"content"`
	ReasoningContent string `db:"reasoning_content"`
	ToolCalls        string `db:"tool_calls"`   // JSON []tools.ToolCall, for role=assistant
	ToolCallID       string `db:"tool_call_id"` // for role=tool, echoes the ToolCall.ID
	Timestamp        DBTime `db:"timestamp"`
}

// DailySummary is an LLM-generated compression of all messages from one day.
type DailySummary struct {
	ID              string            `db:"id"`
	AgentID         string            `db:"agent_id"`
	Date            DBTime            `db:"date"`
	Summary         string            `db:"summary"`
	KeyEvents       JSONSlice[string] `db:"key_events"`
	Topics          JSONSlice[string] `db:"topics"`
	ConversationIDs JSONSlice[string] `db:"conversation_ids"`
}

// WeeklySummary compresses daily summaries for one week.
type WeeklySummary struct {
	ID           string            `db:"id"`
	AgentID      string            `db:"agent_id"`
	WeekStart    DBTime            `db:"week_start"`
	WeekEnd      DBTime            `db:"week_end"`
	Summary      string            `db:"summary"`
	Themes       JSONSlice[string] `db:"themes"`
	Achievements JSONSlice[string] `db:"achievements"`
}

// MonthlySummary compresses weekly summaries for one month.
type MonthlySummary struct {
	ID         string            `db:"id"`
	AgentID    string            `db:"agent_id"`
	Year       int               `db:"year"`
	Month      int               `db:"month"`
	Summary    string            `db:"summary"`
	Highlights JSONSlice[string] `db:"highlights"`
	Statistics JSONMap           `db:"statistics"`
}

// YearlyArchive is the final compression tier — kept forever.
type YearlyArchive struct {
	ID          string            `db:"id"`
	AgentID     string            `db:"agent_id"`
	Year        int               `db:"year"`
	Summary     string            `db:"summary"`
	MajorEvents JSONSlice[string] `db:"major_events"`
	Statistics  JSONMap           `db:"statistics"`
}

// Fact is a persistent piece of knowledge about an entity extracted during summarisation.
// SupersededBy points to a newer conflicting fact's ID, or "".
type Fact struct {
	ID                   string  `db:"id"`
	AgentID              string  `db:"agent_id"`
	Entity               string  `db:"entity"`
	EntityType           string  `db:"entity_type"`
	Fact                 string  `db:"fact"`
	Confidence           float64 `db:"confidence"`
	SourceConversationID string  `db:"source_conversation_id"`
	SupersededBy         string  `db:"superseded_by"`
	FirstSeen            DBTime  `db:"first_seen"`
	LastConfirmed        DBTime  `db:"last_confirmed"`
}

// AgentState holds persistent per-agent runtime state.
type AgentState struct {
	AgentID               string            `db:"agent_id"`
	CurrentConversationID string            `db:"current_conversation_id"`
	Context               JSONMap           `db:"context"`
	Preferences           JSONMap           `db:"preferences"`
	Goals                 JSONSlice[string] `db:"goals"`
	LastActive            DBTime            `db:"last_active"`
}

// CalendarEvent is a native calendar entry.
type CalendarEvent struct {
	ID          string            `db:"id"`
	AgentID     string            `db:"agent_id"`
	Title       string            `db:"title"`
	Description string            `db:"description"`
	StartTime   DBTime            `db:"start_time"`
	EndTime     DBTime            `db:"end_time"`
	AllDay      bool              `db:"all_day"`
	Location    string            `db:"location"`
	Recurrence  string            `db:"recurrence"`
	Tags        JSONSlice[string] `db:"tags"`
	CreatedAt   DBTime            `db:"created_at"`
	UpdatedAt   DBTime            `db:"updated_at"`
}

// Note is a Markdown note.
type Note struct {
	ID        string            `db:"id"`
	AgentID   string            `db:"agent_id"`
	Title     string            `db:"title"`
	Content   string            `db:"content"`
	Tags      JSONSlice[string] `db:"tags"`
	Pinned    bool              `db:"pinned"`
	CreatedAt DBTime            `db:"created_at"`
	UpdatedAt DBTime            `db:"updated_at"`
}

// Reminder is a one-shot or recurring reminder delivered via the agent's channel.
type Reminder struct {
	ID        string     `db:"id"`
	AgentID   string     `db:"agent_id"`
	Title     string     `db:"title"`
	Body      string     `db:"body"`
	DueAt     DBTime     `db:"due_at"`
	SentAt    NullDBTime `db:"sent_at"`
	Recurring string     `db:"recurring"`
	CreatedAt DBTime     `db:"created_at"`
}

// JobRun records the last execution of a named background job.
// Stored with job_name as primary key — one row per job, upserted on each run.
type JobRun struct {
	JobName    string `db:"job_name"`
	LastRun    DBTime `db:"last_run"`
	LastStatus string `db:"last_status"` // "ok" | "error"
	LastError  string `db:"last_error"`  // "" when ok
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
	ID          string         `db:"id"`
	Name        string         `db:"name"`
	Description string         `db:"description"`
	CreatedBy   string         `db:"created_by"`
	Status      WorkflowStatus `db:"status"`
	Metadata    string         `db:"metadata"` // JSON
	CreatedAt   DBTime         `db:"created_at"`
	UpdatedAt   DBTime         `db:"updated_at"`
}

type TaskType string

const (
	TaskTypeAgentExecution TaskType = "agent_execution"
	TaskTypeSkillExecution TaskType = "skill_execution"
	TaskTypeWebhook        TaskType = "webhook"
	TaskTypeDelay          TaskType = "delay"
	TaskTypeCondition      TaskType = "condition"
)

type TaskStatus string

const (
	TaskStatusPending   TaskStatus = "pending"
	TaskStatusQueued    TaskStatus = "queued"
	TaskStatusRunning   TaskStatus = "running"
	TaskStatusCompleted TaskStatus = "completed"
	TaskStatusFailed    TaskStatus = "failed"
	TaskStatusCancelled TaskStatus = "cancelled"
	TaskStatusBlocked   TaskStatus = "blocked"
)

type Task struct {
	ID                   string     `db:"id"`
	WorkflowID           string     `db:"workflow_id"`
	Name                 string     `db:"name"`
	Description          string     `db:"description"`
	Type                 TaskType   `db:"type"`
	DependsOn            string     `db:"depends_on"`            // JSON array
	RequiredCapabilities string     `db:"required_capabilities"` // JSON array
	Payload              string     `db:"payload"`               // JSON
	Status               TaskStatus `db:"status"`
	Result               string     `db:"result"` // JSON
	Error                string     `db:"error"`
	AssignedAgentID      string     `db:"assigned_agent_id"`
	CreatedAt            DBTime     `db:"created_at"`
	StartedAt            NullDBTime `db:"started_at"`
	CompletedAt          NullDBTime `db:"completed_at"`
	RetryCount           int        `db:"retry_count"`
	MaxRetries           int        `db:"max_retries"`
	TimeoutSeconds       int        `db:"timeout_seconds"`
	Metadata             string     `db:"metadata"` // JSON
	UpdatedAt            DBTime     `db:"updated_at"`
}
