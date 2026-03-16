-- Calendar
CREATE TABLE IF NOT EXISTS calendar_events (
    id          TEXT PRIMARY KEY,
    title       TEXT NOT NULL,
    description TEXT NOT NULL DEFAULT '',
    start_time  TEXT NOT NULL,
    end_time    TEXT NOT NULL,
    all_day     INTEGER NOT NULL DEFAULT 0,
    location    TEXT NOT NULL DEFAULT '',
    recurrence  TEXT NOT NULL DEFAULT '',
    tags        TEXT NOT NULL DEFAULT '[]',
    created_at  TEXT NOT NULL,
    updated_at  TEXT NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_calendar_time ON calendar_events (start_time ASC);
