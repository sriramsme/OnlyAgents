package storage

import (
	"context"
	"encoding/json"
	"time"
)

type Storage interface {
	ConversationStore
	MessageStore
	MemoryStore
	FactStore
	AgentStateStore
	CalendarStore
	NoteStore
	ReminderStore
	WorkflowStore
	JobRunStore
	Close() error
}

type ConversationStore interface {
	CreateConversation(ctx context.Context, conv *Conversation) error
	GetConversation(ctx context.Context, id string) (*Conversation, error)
	UpdateConversation(ctx context.Context, conv *Conversation) error
	ListConversations(ctx context.Context, agentID string, limit int) ([]*Conversation, error)
	EndConversation(ctx context.Context, id string, summary string) error
}

type MessageStore interface {
	SaveMessage(ctx context.Context, msg *Message) error
	GetMessages(ctx context.Context, conversationID string) ([]*Message, error)
	GetRecentMessages(ctx context.Context, agentID string, since time.Time) ([]*Message, error)
	DeleteOldMessages(ctx context.Context, olderThan time.Time) error
}

type MemoryStore interface {
	SaveDailySummary(ctx context.Context, s *DailySummary) error
	GetDailySummary(ctx context.Context, agentID string, date time.Time) (*DailySummary, error)
	GetDailySummaries(ctx context.Context, agentID string, from, to time.Time) ([]*DailySummary, error)

	SaveWeeklySummary(ctx context.Context, s *WeeklySummary) error
	GetWeeklySummaries(ctx context.Context, agentID string, from, to time.Time) ([]*WeeklySummary, error)

	SaveMonthlySummary(ctx context.Context, s *MonthlySummary) error
	GetMonthlySummaries(ctx context.Context, agentID string, year int) ([]*MonthlySummary, error)

	SaveYearlyArchive(ctx context.Context, a *YearlyArchive) error
	GetYearlyArchive(ctx context.Context, agentID string, year int) (*YearlyArchive, error)
}

type FactStore interface {
	UpsertFact(ctx context.Context, fact *Fact) error
	GetFacts(ctx context.Context, agentID string, entity string) ([]*Fact, error)
	SearchFacts(ctx context.Context, agentID string, query string) ([]*Fact, error) // FTS5
	DeleteFact(ctx context.Context, id string) error
}

type AgentStateStore interface {
	GetAgentState(ctx context.Context, agentID string) (*AgentState, error)
	SaveAgentState(ctx context.Context, state *AgentState) error
}

type CalendarStore interface {
	CreateEvent(ctx context.Context, event *CalendarEvent) error
	GetEvent(ctx context.Context, id string) (*CalendarEvent, error)
	UpdateEvent(ctx context.Context, event *CalendarEvent) error
	DeleteEvent(ctx context.Context, id string) error
	ListEvents(ctx context.Context, agentID string, from, to time.Time) ([]*CalendarEvent, error)
	GetUpcomingEvents(ctx context.Context, agentID string, limit int) ([]*CalendarEvent, error)
}

type NoteStore interface {
	CreateNote(ctx context.Context, note *Note) error
	GetNote(ctx context.Context, id string) (*Note, error)
	UpdateNote(ctx context.Context, note *Note) error
	DeleteNote(ctx context.Context, id string) error
	ListNotes(ctx context.Context, agentID string) ([]*Note, error)
	SearchNotes(ctx context.Context, agentID string, query string) ([]*Note, error) // FTS5
}

type ReminderStore interface {
	CreateReminder(ctx context.Context, r *Reminder) error
	GetReminder(ctx context.Context, id string) (*Reminder, error)
	UpdateReminder(ctx context.Context, r *Reminder) error
	DeleteReminder(ctx context.Context, id string) error
	ListReminders(ctx context.Context, agentID string) ([]*Reminder, error)
	GetDueReminders(ctx context.Context, before time.Time) ([]*Reminder, error)
	MarkReminderSent(ctx context.Context, id string) error
}

// WorkflowStore manages workflow orchestration
type WorkflowStore interface {
	// Workflows
	CreateWorkflow(ctx context.Context, workflow *Workflow) error
	GetWorkflow(ctx context.Context, id string) (*Workflow, error)
	UpdateWorkflowStatus(ctx context.Context, id string, status WorkflowStatus) error

	// Tasks
	CreateTask(ctx context.Context, task *Task) error
	GetTask(ctx context.Context, id string) (*Task, error)
	UpdateTaskStatus(ctx context.Context, id string, status TaskStatus, errorMsg string) error
	UpdateTaskResult(ctx context.Context, id string, result json.RawMessage) error
	GetWorkflowTasks(ctx context.Context, workflowID string) ([]*Task, error)
	GetReadyTasks(ctx context.Context, limit int) ([]*Task, error)
	GetDependentTasks(ctx context.Context, taskID string) ([]*Task, error)
	AllDependenciesSatisfied(ctx context.Context, taskID string) (bool, error)
}

// JobRunStore tracks the last successful execution of each scheduled background job.
// Used by the memory scheduler for catch-up on startup. Reusable for any cron job.
type JobRunStore interface {
	GetJobRun(ctx context.Context, jobName string) (*JobRun, error)
	SaveJobRun(ctx context.Context, run *JobRun) error
}
