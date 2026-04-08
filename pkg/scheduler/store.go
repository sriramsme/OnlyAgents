package scheduler

import (
	"context"

	"github.com/sriramsme/OnlyAgents/pkg/dbtypes"
)

// JobRunStore tracks the last successful execution of each scheduled background job.
// Used by the memory scheduler for catch-up on startup. Reusable for any cron job.
type Store interface {
	GetCronJob(ctx context.Context, id string) (*CronJob, error)
	SaveCronJob(ctx context.Context, job *CronJob) error
	DeleteCronJob(ctx context.Context, id string) error
	ListCronJobs(ctx context.Context) ([]*CronJob, error)
	UpdateCronJobRun(ctx context.Context, id, status, lastError string) error
}

type CronJob struct {
	ID           string          `db:"id" json:"id"`
	Name         string          `db:"name" json:"name"`
	Description  string          `db:"description" json:"description,omitempty"`
	Schedule     string          `db:"schedule" json:"schedule"`
	Enabled      bool            `db:"enabled" json:"enabled,omitempty"`
	EventType    string          `db:"event_type" json:"event_type"`
	EventPayload string          `db:"event_payload" json:"event_payload,omitempty"`
	LastRun      *dbtypes.DBTime `db:"last_run" json:"last_run,omitempty"` // pointer — nil means never ran
	LastStatus   string          `db:"last_status" json:"last_status,omitempty"`
	LastError    string          `db:"last_error" json:"last_error,omitempty"`
	CreatedAt    dbtypes.DBTime  `db:"created_at" json:"created_at"`
	UpdatedAt    dbtypes.DBTime  `db:"updated_at" json:"updated_at"`
}
