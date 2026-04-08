package storage

import (
	"context"
	"time"

	"github.com/sriramsme/OnlyAgents/pkg/conversation"
	"github.com/sriramsme/OnlyAgents/pkg/message"
	"github.com/sriramsme/OnlyAgents/pkg/productivity/calendar"
	"github.com/sriramsme/OnlyAgents/pkg/productivity/notes"
	"github.com/sriramsme/OnlyAgents/pkg/productivity/reminder"
	"github.com/sriramsme/OnlyAgents/pkg/productivity/task"
	"github.com/sriramsme/OnlyAgents/pkg/scheduler"
	"github.com/sriramsme/OnlyAgents/pkg/workflow"
)

type Storage interface {
	conversation.Store
	message.Store
	task.Store
	reminder.Store
	calendar.Store
	notes.Store
	workflow.Store
	scheduler.Store
	MemoryStore
	FactStore
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
