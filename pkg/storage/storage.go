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
	CronJobStore
	TaskStore
	Close() error
}

type ConversationStore interface {
	CreateConversation(ctx context.Context, conv *Conversation) error
	GetConversation(ctx context.Context, id string) (*Conversation, error)
	GetConversationByChannel(ctx context.Context, channel, agentID string) (*Conversation, error)
	GetConversationsByDay(ctx context.Context, from, to time.Time) ([]*Conversation, error)
	UpdateConversation(ctx context.Context, conv *Conversation) error
	ListConversations(ctx context.Context, limit int) ([]*Conversation, error)
	ListConversationsByChannel(ctx context.Context, channel string, limit int) ([]*Conversation, error)
	EndConversation(ctx context.Context, id string, summary string) error
}

type MessageStore interface {
	SaveMessage(ctx context.Context, msg *Message) error
	GetMessages(ctx context.Context, conversationID string) ([]*Message, error)
	GetMessagesByAgent(ctx context.Context, conversationID, agentID string) ([]*Message, error)
	GetMessagesBetween(ctx context.Context, roles []string, from, to time.Time) ([]*Message, error)
	DeleteOldMessages(ctx context.Context, olderThan time.Time) error
	// After send, update the existing message record with the platform ID
	UpdateMessagePlatformID(ctx context.Context, messageID, platformMessageID string) error

	// Lookup by platform ID — caller reads .AgentID from the result
	GetMessageByPlatformID(ctx context.Context, platformMessageID string) (*Message, error)
}

type MemoryStore interface {
	SaveDailySummary(ctx context.Context, s *DailySummary) error
	GetDailySummary(ctx context.Context, date time.Time) (*DailySummary, error)
	GetDailySummaries(ctx context.Context, from, to time.Time) ([]*DailySummary, error)

	SaveWeeklySummary(ctx context.Context, s *WeeklySummary) error
	GetWeeklySummaries(ctx context.Context, from, to time.Time) ([]*WeeklySummary, error)

	SaveMonthlySummary(ctx context.Context, s *MonthlySummary) error
	GetMonthlySummaries(ctx context.Context, year int) ([]*MonthlySummary, error)

	SaveYearlyArchive(ctx context.Context, a *YearlyArchive) error
	GetYearlyArchive(ctx context.Context, year int) (*YearlyArchive, error)
}

type FactStore interface {
	InsertFact(ctx context.Context, fact *Fact) error
	GetFacts(ctx context.Context, entity string) ([]*Fact, error)
	SearchFacts(ctx context.Context, query string) ([]*Fact, error) // FTS5
	DeleteFact(ctx context.Context, id string) error

	// for conflict detection and reinforcement in saveFacts.
	GetFactByKey(ctx context.Context, entity, fact string) (*Fact, error)
	GetActiveFactsByEntity(ctx context.Context, entity string) ([]*Fact, error)
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
	ListEvents(ctx context.Context, from, to time.Time) ([]*CalendarEvent, error)
	GetUpcomingEvents(ctx context.Context, limit int) ([]*CalendarEvent, error)
}

type NoteStore interface {
	CreateNote(ctx context.Context, note *Note) error
	GetNote(ctx context.Context, id string) (*Note, error)
	UpdateNote(ctx context.Context, note *Note) error
	DeleteNote(ctx context.Context, id string) error
	ListNotes(ctx context.Context) ([]*Note, error)
	SearchNotes(ctx context.Context, query string) ([]*Note, error)
}

type ReminderStore interface {
	CreateReminder(ctx context.Context, r *Reminder) error
	GetReminder(ctx context.Context, id string) (*Reminder, error)
	UpdateReminder(ctx context.Context, r *Reminder) error
	DeleteReminder(ctx context.Context, id string) error
	ListReminders(ctx context.Context) ([]*Reminder, error)
	GetDueReminders(ctx context.Context, before time.Time) ([]*Reminder, error)
	MarkReminderSent(ctx context.Context, id string, sentAt time.Time) error
}

// WorkflowStore manages workflow orchestration
type WorkflowStore interface {
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

// TaskStore manages tasks with optional project grouping.
// TaskFilter fields are all optional — nil means no filter on that field.
type TaskStore interface {
	CreateTask(ctx context.Context, task *Task) error
	GetTask(ctx context.Context, id string) (*Task, error)
	UpdateTask(ctx context.Context, task *Task) error
	DeleteTask(ctx context.Context, id string) error
	CompleteTask(ctx context.Context, id string) error
	ListTasks(ctx context.Context, filter TaskFilter) ([]*Task, error)
	SearchTasks(ctx context.Context, query string) ([]*Task, error)
	GetTasksDueOn(ctx context.Context, date time.Time) ([]*Task, error)

	CreateProject(ctx context.Context, project *Project) error
	GetProject(ctx context.Context, id string) (*Project, error)
	UpdateProject(ctx context.Context, project *Project) error
	DeleteProject(ctx context.Context, id string) error
	ListProjects(ctx context.Context) ([]*Project, error)
}

// JobRunStore tracks the last successful execution of each scheduled background job.
// Used by the memory scheduler for catch-up on startup. Reusable for any cron job.
type CronJobStore interface {
	GetCronJob(ctx context.Context, id string) (*CronJob, error)
	SaveCronJob(ctx context.Context, job *CronJob) error
	DeleteCronJob(ctx context.Context, id string) error
	ListCronJobs(ctx context.Context) ([]*CronJob, error)
	UpdateCronJobRun(ctx context.Context, id, status, lastError string) error
}
