-- Reminders
CREATE TABLE IF NOT EXISTS reminders (
    id         TEXT PRIMARY KEY,
    title      TEXT NOT NULL,
    body       TEXT NOT NULL DEFAULT '',
    due_at     TEXT NOT NULL,
    sent_at    TEXT,
    recurring  TEXT NOT NULL DEFAULT '',
    created_at TEXT NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_reminders_due ON reminders (due_at ASC, sent_at);
