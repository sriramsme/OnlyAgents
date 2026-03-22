CREATE TABLE cron_jobs (
    id          TEXT PRIMARY KEY,
    name        TEXT NOT NULL,
    description TEXT,
    schedule    TEXT NOT NULL,
    enabled     INTEGER NOT NULL DEFAULT 1,
    event_type  TEXT NOT NULL,     -- e.g. "task_assigned", "workflow_submitted"
    event_payload TEXT NOT NULL,   -- raw JSON, kernel deserializes and handles it
    last_run    TEXT,
    last_status TEXT,
    last_error  TEXT,
    created_at  TEXT NOT NULL,
    updated_at  TEXT NOT NULL
);
