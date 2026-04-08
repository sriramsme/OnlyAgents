package sqlite

import (
	"context"
	"database/sql"
	"errors"

	"github.com/sriramsme/OnlyAgents/pkg/scheduler"
)

func (d *DB) GetCronJob(ctx context.Context, id string) (*scheduler.CronJob, error) {
	var job scheduler.CronJob
	err := d.db.GetContext(ctx, &job, `SELECT * FROM cron_jobs WHERE id = ?`, id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, wrap(err, "get cron job")
	}
	return &job, nil
}

func (d *DB) ListCronJobs(ctx context.Context) ([]*scheduler.CronJob, error) {
	var jobs []*scheduler.CronJob
	err := d.db.SelectContext(ctx, &jobs, `
		SELECT * FROM cron_jobs ORDER BY created_at ASC
	`)
	return jobs, wrap(err, "list cron jobs")
}

func (d *DB) SaveCronJob(ctx context.Context, job *scheduler.CronJob) error {
	_, err := d.db.NamedExecContext(ctx, `
		INSERT INTO cron_jobs
			(id, name, description, schedule, enabled, event_type,
			 event_payload, last_run, last_status, last_error,
			 created_at, updated_at)
		VALUES
			(:id, :name, :description, :schedule, :enabled, :event_type,
			 :event_payload, :last_run, :last_status, :last_error,
			 :created_at, :updated_at)
		ON CONFLICT(id) DO UPDATE SET
			name                 = excluded.name,
			description          = excluded.description,
			schedule             = excluded.schedule,
			enabled              = excluded.enabled,
			event_type           = excluded.event_type,
			event_payload        = excluded.event_payload,
			last_run             = excluded.last_run,
			last_status          = excluded.last_status,
			last_error           = excluded.last_error,
			updated_at           = excluded.updated_at
	`, job)
	return wrap(err, "save cron job")
}

func (d *DB) DeleteCronJob(ctx context.Context, id string) error {
	_, err := d.db.ExecContext(ctx, `DELETE FROM cron_jobs WHERE id = ?`, id)
	return wrap(err, "delete cron job")
}

func (d *DB) UpdateCronJobRun(ctx context.Context, id, status, lastError string) error {
	_, err := d.db.ExecContext(ctx, `
		UPDATE cron_jobs
		SET last_run    = datetime('now'),
		    last_status = ?,
		    last_error  = ?,
		    updated_at  = datetime('now')
		WHERE id = ?
	`, status, lastError, id)
	return wrap(err, "update cron job run")
}
