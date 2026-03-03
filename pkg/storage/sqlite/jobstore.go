package sqlite

import (
	"context"
	"database/sql"
	"errors"

	"github.com/sriramsme/OnlyAgents/pkg/storage"
)

func (d *DB) GetJobRun(ctx context.Context, jobName string) (*storage.JobRun, error) {
	var run storage.JobRun
	err := d.db.GetContext(ctx, &run, `SELECT * FROM job_runs WHERE job_name = ?`, jobName)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil // no prior run — caller treats nil as "never ran"
		}
		return nil, wrap(err, "get job run")
	}
	return &run, nil
}

func (d *DB) SaveJobRun(ctx context.Context, run *storage.JobRun) error {
	_, err := d.db.NamedExecContext(ctx, `
		INSERT INTO job_runs (job_name, last_run, last_status, last_error)
		VALUES (:job_name, :last_run, :last_status, :last_error)
		ON CONFLICT(job_name) DO UPDATE SET
			last_run    = excluded.last_run,
			last_status = excluded.last_status,
			last_error  = excluded.last_error
	`, run)
	return wrap(err, "save job run")
}
