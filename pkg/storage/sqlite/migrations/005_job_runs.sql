-- Tracks the last successful run time of each scheduled background job.
-- Used by the memory scheduler for catch-up logic on startup.
-- Reusable for any future cron jobs beyond memory summarisation.

CREATE TABLE IF NOT EXISTS job_runs (
    job_name    TEXT PRIMARY KEY,
    last_run    TEXT NOT NULL,
    last_status TEXT NOT NULL DEFAULT 'ok', -- ok | error
    last_error  TEXT NOT NULL DEFAULT ''
);
