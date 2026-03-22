package scheduler

import "context"

// Job is the base interface every scheduled job must implement.
type Job interface {
	// Name is the unique identifier for this job — used for logging and
	// as the primary key in cron_jobs if persisted.
	Name() string

	// Schedule returns a standard 5-field cron spec, e.g. "59 23 * * *".
	Schedule() string

	// Run executes the job. Returning an error marks the run as failed
	// but does not stop the scheduler.
	Run(ctx context.Context) error
}

// CatchUpJob extends Job with catch-up logic for jobs that must run even
// if the process was down at the scheduled time.
// On scheduler Start, CatchUp is called before the cron loop begins.
// Each implementation decides its own "did I miss a run" threshold.
type CatchUpJob interface {
	Job
	CatchUp(ctx context.Context) error
}
