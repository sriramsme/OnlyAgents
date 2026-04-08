package storage

import (
	"context"
	"encoding/json"
	"time"

	"github.com/sriramsme/OnlyAgents/pkg/conversation"
	"github.com/sriramsme/OnlyAgents/pkg/message"
	"github.com/sriramsme/OnlyAgents/pkg/productivity/calendar"
	"github.com/sriramsme/OnlyAgents/pkg/productivity/notes"
	"github.com/sriramsme/OnlyAgents/pkg/productivity/reminder"
	"github.com/sriramsme/OnlyAgents/pkg/productivity/task"
)

type Storage interface {
	conversation.Store
	message.Store
	task.Store
	reminder.Store
	calendar.Store
	notes.Store
	MemoryStore
	FactStore
	AgentStateStore
	WorkflowStore
	CronJobStore
	Close() error
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

// JobRunStore tracks the last successful execution of each scheduled background job.
// Used by the memory scheduler for catch-up on startup. Reusable for any cron job.
type CronJobStore interface {
	GetCronJob(ctx context.Context, id string) (*CronJob, error)
	SaveCronJob(ctx context.Context, job *CronJob) error
	DeleteCronJob(ctx context.Context, id string) error
	ListCronJobs(ctx context.Context) ([]*CronJob, error)
	UpdateCronJobRun(ctx context.Context, id, status, lastError string) error
}
